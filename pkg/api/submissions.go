package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cee.io/pkg/languages"
	"cee.io/pkg/queue"
	"cee.io/pkg/security"
	"cee.io/pkg/utils"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// POST /submissions
func (h *Handler) CreateSubmission(w http.ResponseWriter, r *http.Request) {
	var req SubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Validation Error", "Invalid JSON body")
		return
	}

	job, err := h.buildJobFromRequest(&req, r.URL.Query().Get("base64_encoded") == "true")
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error())
		return
	}

	if err := h.queue.Enqueue(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal Error", "Failed to enqueue submission: "+err.Error())
		return
	}

	wait := r.URL.Query().Get("wait") == "true"
	base64Encoded := r.URL.Query().Get("base64_encoded") == "true"
	fields := parseFields(r.URL.Query().Get("fields"))

	if wait {
		completedJob, err := h.queue.WaitForResult(r.Context(), job.Token, 30*time.Second)
		if err != nil || completedJob == nil {
			completedJob = job
		}
		respondJSON(w, http.StatusCreated, formatJobResponse(completedJob, base64Encoded, fields))
	} else {
		respondJSON(w, http.StatusCreated, CreateTokenResponse{Token: job.Token})
	}
}

// GET /submissions/{token}
func (h *Handler) GetSubmission(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	base64Encoded := r.URL.Query().Get("base64_encoded") == "true"
	fields := parseFields(r.URL.Query().Get("fields"))

	job, err := h.queue.GetSubmission(r.Context(), token)
	if err != nil || job == nil {
		respondError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("Submission with token %s not found", token))
		return
	}

	respondJSON(w, http.StatusOK, formatJobResponse(job, base64Encoded, fields))
}

// DELETE /submissions/{token}
func (h *Handler) DeleteSubmission(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if err := h.queue.DeleteSubmission(r.Context(), token); err != nil {
		respondError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("Submission with token %s not found", token))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Submission deleted"})
}

// POST /submissions/batch
func (h *Handler) CreateBatchSubmission(w http.ResponseWriter, r *http.Request) {
	var req BatchSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Validation Error", "Invalid JSON body")
		return
	}

	if len(req.Submissions) == 0 || len(req.Submissions) > 20 {
		respondError(w, http.StatusUnprocessableEntity, "Validation Error", "Batch submissions must contain between 1 and 20 items")
		return
	}

	base64Encoded := r.URL.Query().Get("base64_encoded") == "true"
	results := make([]CreateTokenResponse, 0, len(req.Submissions))

	jobs := make([]*queue.SubmissionJob, 0, len(req.Submissions))
	for _, subReq := range req.Submissions {
		job, err := h.buildJobFromRequest(&subReq, base64Encoded)
		if err != nil {
			respondError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error())
			return
		}
		jobs = append(jobs, job)
	}

	for _, job := range jobs {
		if err := h.queue.Enqueue(r.Context(), job); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal Error", err.Error())
			return
		}
		results = append(results, CreateTokenResponse{Token: job.Token})
	}

	respondJSON(w, http.StatusCreated, results)
}

// GET /submissions/batch
func (h *Handler) GetBatchSubmission(w http.ResponseWriter, r *http.Request) {
	tokensParam := r.URL.Query().Get("tokens")
	if tokensParam == "" {
		respondError(w, http.StatusBadRequest, "Bad Request", "Missing tokens query parameter")
		return
	}

	base64Encoded := r.URL.Query().Get("base64_encoded") == "true"
	fields := parseFields(r.URL.Query().Get("fields"))

	rawTokens := strings.Split(tokensParam, ",")
	tokens := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" {
			tokens = append(tokens, trimmed)
		}
	}

	if len(tokens) > 20 {
		respondError(w, http.StatusBadRequest, "Bad Request", "Maximum 20 tokens allowed")
		return
	}

	submissions := make([]any, len(tokens))
	for i, tok := range tokens {
		job, err := h.queue.GetSubmission(r.Context(), tok)
		if err != nil || job == nil {
			submissions[i] = map[string]any{"token": tok, "error": "Not found"}
		} else {
			submissions[i] = formatJobResponse(job, base64Encoded, fields)
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"submissions": submissions})
}

func (h *Handler) buildJobFromRequest(req *SubmissionRequest, base64Encoded bool) (*queue.SubmissionJob, error) {
	lang := languages.GetLanguageByID(req.LanguageID)
	if lang == nil {
		return nil, fmt.Errorf("invalid language_id: %d", req.LanguageID)
	}

	var sourceCode string
	if req.SourceCode != nil {
		sourceCode = utils.DecodeIfNeeded(*req.SourceCode, base64Encoded)
	}
	if len(sourceCode) > h.cfg.Execution.MaxSourceSize {
		return nil, fmt.Errorf("source_code exceeds limit of %d bytes", h.cfg.Execution.MaxSourceSize)
	}

	var stdin string
	if req.Stdin != nil {
		stdin = utils.DecodeIfNeeded(*req.Stdin, base64Encoded)
	}

	var expectedOutput string
	if req.ExpectedOutput != nil {
		expectedOutput = utils.DecodeIfNeeded(*req.ExpectedOutput, base64Encoded)
	}

	var additionalFiles string
	if req.AdditionalFiles != nil {
		additionalFiles = *req.AdditionalFiles
		if len(additionalFiles) > h.cfg.Execution.MaxAdditionalFilesSize {
			return nil, fmt.Errorf("additional_files exceeds size limit")
		}
	}

	if lang.ID == languages.LangMultiFile && additionalFiles == "" {
		return nil, fmt.Errorf("language_id 89 (multi-file) requires additional_files ZIP")
	}

	var callbackURL string
	if req.CallbackURL != nil && *req.CallbackURL != "" {
		if err := security.ValidateCallbackURL(*req.CallbackURL); err != nil {
			return nil, fmt.Errorf("invalid callback_url: %w", err)
		}
		callbackURL = *req.CallbackURL
	}

	cpuLimit := h.cfg.Execution.DefaultCPUTimeLimit
	if req.CPUTimeLimit != nil && *req.CPUTimeLimit > 0 {
		cpuLimit = *req.CPUTimeLimit
		if cpuLimit > h.cfg.Execution.MaxCPUTimeLimit {
			cpuLimit = h.cfg.Execution.MaxCPUTimeLimit
		}
	}

	cpuExtra := 1.0
	if req.CPUExtraTime != nil && *req.CPUExtraTime >= 0 {
		cpuExtra = *req.CPUExtraTime
		if cpuExtra > 5.0 {
			cpuExtra = 5.0
		}
	}

	wallLimit := h.cfg.Execution.DefaultWallTimeLimit
	if req.WallTimeLimit != nil && *req.WallTimeLimit > 0 {
		wallLimit = *req.WallTimeLimit
		if wallLimit > h.cfg.Execution.MaxWallTimeLimit {
			wallLimit = h.cfg.Execution.MaxWallTimeLimit
		}
	}

	memLimit := h.cfg.Execution.DefaultMemoryLimit
	if req.MemoryLimit != nil && *req.MemoryLimit > 0 {
		memLimit = *req.MemoryLimit
		if memLimit > h.cfg.Execution.MaxMemoryLimit {
			memLimit = h.cfg.Execution.MaxMemoryLimit
		}
	}

	processes := h.cfg.Execution.MaxProcesses
	if req.MaxProcessesAndOrThreads != nil && *req.MaxProcessesAndOrThreads > 0 {
		processes = *req.MaxProcessesAndOrThreads
		if processes > h.cfg.Execution.MaxProcesses {
			processes = h.cfg.Execution.MaxProcesses
		}
	}

	var compilerOptions string
	if req.CompilerOptions != nil {
		compilerOptions = security.SanitizeOptions(*req.CompilerOptions)
	}

	var cmdArgs string
	if req.CommandLineArguments != nil {
		cmdArgs = security.SanitizeOptions(*req.CommandLineArguments)
	}

	redirectStderr := false
	if req.RedirectStderrToStdout != nil {
		redirectStderr = *req.RedirectStderrToStdout
	}

	token := uuid.New().String()

	return &queue.SubmissionJob{
		Token:                  token,
		SourceCode:             sourceCode,
		LanguageID:             req.LanguageID,
		Language:               lang,
		Stdin:                  stdin,
		ExpectedOutput:         expectedOutput,
		CPUTimeLimit:           cpuLimit,
		CPUExtraTime:           cpuExtra,
		WallTimeLimit:          wallLimit,
		MemoryLimit:            memLimit,
		StackLimit:             h.cfg.Execution.DefaultStackLimit,
		MaxProcesses:           processes,
		MaxFileSize:            1024,
		CompilerOptions:        compilerOptions,
		CommandLineArguments:   cmdArgs,
		RedirectStderrToStdout: redirectStderr,
		EnableNetwork:          false,
		CallbackURL:            callbackURL,
		AdditionalFiles:        additionalFiles,
		Status:                 languages.GetStatusByID(languages.StatusInQueue),
		CreatedAt:              time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func formatJobResponse(job *queue.SubmissionJob, base64Encode bool, fields map[string]bool) any {
	resp := SubmissionResponse{
		Token:          job.Token,
		SourceCode:     utils.EncodeIfNeeded(&job.SourceCode, base64Encode),
		LanguageID:     job.LanguageID,
		Stdin:          utils.EncodeIfNeeded(&job.Stdin, base64Encode),
		ExpectedOutput: utils.EncodeIfNeeded(&job.ExpectedOutput, base64Encode),
		Stdout:         utils.EncodeIfNeeded(job.Stdout, base64Encode),
		Stderr:         utils.EncodeIfNeeded(job.Stderr, base64Encode),
		CompileOutput:  utils.EncodeIfNeeded(job.CompileOutput, base64Encode),
		Message:        utils.EncodeIfNeeded(job.Message, base64Encode),
		Status:         job.Status,
		CreatedAt:      job.CreatedAt,
		FinishedAt:     job.FinishedAt,
		Time:           job.Time,
		WallTime:       job.WallTime,
		Memory:         job.Memory,
		ExitCode:       job.ExitCode,
		ExitSignal:     job.ExitSignal,
	}

	if len(fields) == 0 {
		return resp
	}

	rawMap, _ := json.Marshal(resp)
	var fullMap map[string]any
	_ = json.Unmarshal(rawMap, &fullMap)

	filtered := make(map[string]any)
	for k := range fields {
		if val, exists := fullMap[k]; exists {
			filtered[k] = val
		}
	}
	return filtered
}

func parseFields(fieldsStr string) map[string]bool {
	if fieldsStr == "" {
		return nil
	}
	m := make(map[string]bool)
	for _, f := range strings.Split(fieldsStr, ",") {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			m[trimmed] = true
		}
	}
	return m
}

