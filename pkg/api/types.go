package api

import "cee.io/pkg/languages"

// SubmissionRequest matches Judge0 submission creation payload.
type SubmissionRequest struct {
	SourceCode               *string  `json:"source_code"`
	LanguageID               int      `json:"language_id"`
	Stdin                    *string  `json:"stdin"`
	ExpectedOutput           *string  `json:"expected_output"`
	CPUTimeLimit             *float64 `json:"cpu_time_limit"`
	CPUExtraTime             *float64 `json:"cpu_extra_time"`
	WallTimeLimit            *float64 `json:"wall_time_limit"`
	MemoryLimit              *int     `json:"memory_limit"`
	StackLimit               *int     `json:"stack_limit"`
	MaxProcessesAndOrThreads *int     `json:"max_processes_and_or_threads"`
	MaxFileSize              *int     `json:"max_file_size"`
	CompilerOptions          *string  `json:"compiler_options"`
	CommandLineArguments     *string  `json:"command_line_arguments"`
	RedirectStderrToStdout   *bool    `json:"redirect_stderr_to_stdout"`
	EnableNetwork            *bool    `json:"enable_network"`
	CallbackURL              *string  `json:"callback_url"`
	AdditionalFiles          *string  `json:"additional_files"`
}

// BatchSubmissionRequest matches Judge0 batch submission payload.
type BatchSubmissionRequest struct {
	Submissions []SubmissionRequest `json:"submissions"`
}

// SubmissionResponse matches Judge0 response format.
type SubmissionResponse struct {
	Token          string           `json:"token"`
	SourceCode     *string          `json:"source_code,omitempty"`
	LanguageID     int              `json:"language_id,omitempty"`
	Stdin          *string          `json:"stdin,omitempty"`
	ExpectedOutput *string          `json:"expected_output,omitempty"`
	Stdout         *string          `json:"stdout"`
	Stderr         *string          `json:"stderr"`
	CompileOutput  *string          `json:"compile_output"`
	Message        *string          `json:"message"`
	Status         languages.Status `json:"status"`
	CreatedAt      string           `json:"created_at,omitempty"`
	FinishedAt     *string          `json:"finished_at"`
	Time           *string          `json:"time"`
	WallTime       *string          `json:"wall_time"`
	Memory         *int             `json:"memory"`
	ExitCode       *int             `json:"exit_code"`
	ExitSignal     *int             `json:"exit_signal"`
}

// CreateTokenResponse is returned on asynchronous submission creation.
type CreateTokenResponse struct {
	Token string `json:"token"`
}

// ErrorResponse represents standardized API error payload.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
