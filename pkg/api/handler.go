package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"cee.io/pkg/config"
	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
	"cee.io/pkg/queue"
	"cee.io/pkg/security"
	"cee.io/pkg/utils"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var appStartTime = time.Now()

type Handler struct {
	cfg      *config.Config
	queue    queue.Queue
	executor executor.Executor
}

func NewHandler(cfg *config.Config, q queue.Queue, exec executor.Executor) *Handler {
	return &Handler{
		cfg:      cfg,
		queue:    q,
		executor: exec,
	}
}

// GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"uptime":    time.Since(appStartTime).Seconds(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// GET /about
func (h *Handler) About(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"version":     "1.0.0",
		"engine":      "CEE (Code Execution Engine in Go)",
		"homepage":    "https://github.com/navneetguptacse/cee",
		"maintainer":  "Navneet Gupta <navneetguptacse@gmail.com>",
		"compatible":  "Judge0 API v1.13.0",
		"performance": "Ultra-low latency compiled Go runtime",
	})
}

// GET /system_info
func (h *Handler) SystemInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := os.Hostname()

	respondJSON(w, http.StatusOK, map[string]any{
		"system": map[string]any{
			"platform": runtime.GOOS,
			"arch":     runtime.GOARCH,
			"hostname": hostname,
			"num_cpu":  runtime.NumCPU(),
		},
		"memory": map[string]any{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
		"uptime": map[string]any{
			"process": time.Since(appStartTime).Seconds(),
		},
	})
}

// GET /config_info
func (h *Handler) ConfigInfo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"max_cpu_time_limit":               h.cfg.Execution.MaxCPUTimeLimit,
		"max_cpu_extra_time":               5.0,
		"max_wall_time_limit":              h.cfg.Execution.MaxWallTimeLimit,
		"max_memory_limit":                 h.cfg.Execution.MaxMemoryLimit,
		"max_stack_limit":                  h.cfg.Execution.DefaultStackLimit,
		"max_max_processes_and_or_threads": h.cfg.Execution.MaxProcesses,
		"max_max_file_size":                4096,
		"cpu_time_limit":                   h.cfg.Execution.DefaultCPUTimeLimit,
		"cpu_extra_time":                   1.0,
		"wall_time_limit":                  h.cfg.Execution.DefaultWallTimeLimit,
		"memory_limit":                     h.cfg.Execution.DefaultMemoryLimit,
		"stack_limit":                      h.cfg.Execution.DefaultStackLimit,
		"max_processes_and_or_threads":     h.cfg.Execution.MaxProcesses,
		"max_file_size":                    1024,
		"enable_network":                   false,
		"allow_enable_network":             false,
		"enable_additional_files":          true,
	})
}

// GET /executor
func (h *Handler) ExecutorInfo(w http.ResponseWriter, r *http.Request) {
	caps := executor.GetSystemCapabilities(h.cfg.Docker.SocketPath, h.executor.Type())
	respondJSON(w, http.StatusOK, map[string]any{
		"current_executor": h.executor.Type(),
		"configured":       h.cfg.Executor.Type,
		"recommended":      caps.Recommended,
		"capabilities": map[string]any{
			"docker":  caps.Docker,
			"isolate": caps.Isolate,
			"process": caps.Process,
		},
		"platform": caps.Platform,
		"arch":     caps.Arch,
	})
}

// GET /workers
func (h *Handler) Workers(w http.ResponseWriter, r *http.Request) {
	workers := make([]map[string]any, h.cfg.Worker.Concurrency)
	for i := 0; i < h.cfg.Worker.Concurrency; i++ {
		workers[i] = map[string]any{
			"id":     fmt.Sprintf("worker-%d", i+1),
			"name":   fmt.Sprintf("cee-worker-%d", i+1),
			"active": true,
		}
	}
	respondJSON(w, http.StatusOK, workers)
}

// GET /statistics
func (h *Handler) Statistics(w http.ResponseWriter, r *http.Request) {
	stats, err := h.queue.GetStats(r.Context())
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"submissions": map[string]int{"total": 0, "in_queue": 0, "processing": 0, "completed": 0},
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"submissions": stats,
	})
}

// GET /statuses
func (h *Handler) Statuses(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, languages.GetAllStatuses())
}

// GET /languages
func (h *Handler) Languages(w http.ResponseWriter, r *http.Request) {
	langs := languages.GetActiveLanguages()
	res := make([]map[string]any, len(langs))
	for i, l := range langs {
		res[i] = map[string]any{
			"id":   l.ID,
			"name": l.Name,
		}
	}
	respondJSON(w, http.StatusOK, res)
}

// GET /languages/all
func (h *Handler) LanguagesAll(w http.ResponseWriter, r *http.Request) {
	langs := languages.GetAllLanguages()
	res := make([]map[string]any, len(langs))
	for i, l := range langs {
		res[i] = map[string]any{
			"id":          l.ID,
			"name":        l.Name,
			"is_archived": l.IsArchived,
		}
	}
	respondJSON(w, http.StatusOK, res)
}

// GET /languages/{id}
func (h *Handler) LanguageByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Bad Request", "Invalid language ID")
		return
	}

	lang := languages.GetLanguageByID(id)
	if lang == nil {
		respondError(w, http.StatusNotFound, "Not Found", fmt.Sprintf("Language with ID %d not found", id))
		return
	}

	respondJSON(w, http.StatusOK, lang)
}

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

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, code int, errType string, message string) {
	respondJSON(w, code, ErrorResponse{Error: errType, Message: message})
}
