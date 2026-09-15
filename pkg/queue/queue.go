package queue

import (
	"context"
	"sync"
	"time"

	"cee.io/pkg/executor"
	"cee.io/pkg/languages"
)

// SubmissionJob represents a unit of execution work in the queue.
type SubmissionJob struct {
	mu                     sync.RWMutex
	Token                  string
	SourceCode             string
	LanguageID             int
	Language               *languages.Language
	Stdin                  string
	ExpectedOutput         string
	CPUTimeLimit           float64
	CPUExtraTime           float64
	WallTimeLimit          float64
	MemoryLimit            int
	StackLimit             int
	MaxProcesses           int
	MaxFileSize            int
	CompilerOptions        string
	CommandLineArguments   string
	RedirectStderrToStdout bool
	EnableNetwork          bool
	CallbackURL            string
	AdditionalFiles        string // base64 encoded zip
	Status                 languages.Status
	CreatedAt              string
	FinishedAt             *string
	Time                   *string
	WallTime               *string
	Memory                 *int
	Stdout                 *string
	Stderr                 *string
	CompileOutput          *string
	Message                *string
	ExitCode               *int
	ExitSignal             *int
}

// SetStatus updates status thread-safely.
func (j *SubmissionJob) SetStatus(s languages.Status) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = s
}

// GetStatus returns status thread-safely.
func (j *SubmissionJob) GetStatus() languages.Status {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status
}

// IsFinished checks whether execution has ended.
func (j *SubmissionJob) IsFinished() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status.ID > languages.StatusProcessing
}

// Snapshot returns a thread-safe shallow clone of the job state.
func (j *SubmissionJob) Snapshot() *SubmissionJob {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return &SubmissionJob{
		Token:                  j.Token,
		SourceCode:             j.SourceCode,
		LanguageID:             j.LanguageID,
		Language:               j.Language,
		Stdin:                  j.Stdin,
		ExpectedOutput:         j.ExpectedOutput,
		CPUTimeLimit:           j.CPUTimeLimit,
		CPUExtraTime:           j.CPUExtraTime,
		WallTimeLimit:          j.WallTimeLimit,
		MemoryLimit:            j.MemoryLimit,
		StackLimit:             j.StackLimit,
		MaxProcesses:           j.MaxProcesses,
		MaxFileSize:            j.MaxFileSize,
		CompilerOptions:        j.CompilerOptions,
		CommandLineArguments:   j.CommandLineArguments,
		RedirectStderrToStdout: j.RedirectStderrToStdout,
		EnableNetwork:          j.EnableNetwork,
		CallbackURL:            j.CallbackURL,
		AdditionalFiles:        j.AdditionalFiles,
		Status:                 j.Status,
		CreatedAt:              j.CreatedAt,
		FinishedAt:             j.FinishedAt,
		Time:                   j.Time,
		WallTime:               j.WallTime,
		Memory:                 j.Memory,
		Stdout:                 j.Stdout,
		Stderr:                 j.Stderr,
		CompileOutput:          j.CompileOutput,
		Message:                j.Message,
		ExitCode:               j.ExitCode,
		ExitSignal:             j.ExitSignal,
	}
}

// QueueStats provides metrics on queue state.
type QueueStats struct {
	InQueue    int64 `json:"in_queue"`
	Processing int64 `json:"processing"`
	Completed  int64 `json:"completed"`
	Failed     int64 `json:"failed"`
	Total      int64 `json:"total"`
}

// Queue defines the common abstraction for in-memory and distributed queues.
type Queue interface {
	Enqueue(ctx context.Context, job *SubmissionJob) error
	Dequeue(ctx context.Context) (*SubmissionJob, error)
	GetSubmission(ctx context.Context, token string) (*SubmissionJob, error)
	UpdateSubmission(ctx context.Context, job *SubmissionJob) error
	DeleteSubmission(ctx context.Context, token string) error
	GetStats(ctx context.Context) (QueueStats, error)
	WaitForResult(ctx context.Context, token string, timeout time.Duration) (*SubmissionJob, error)
	SignalCompleted(token string, job *SubmissionJob)
	Close() error
}

func JobToExecutionSubmission(job *SubmissionJob) *executor.ExecutionSubmission {
	job.mu.RLock()
	defer job.mu.RUnlock()

	return &executor.ExecutionSubmission{
		Token:                  job.Token,
		SourceCode:             job.SourceCode,
		Language:               job.Language,
		Stdin:                  job.Stdin,
		ExpectedOutput:         job.ExpectedOutput,
		CPUTimeLimit:           job.CPUTimeLimit,
		CPUExtraTime:           job.CPUExtraTime,
		WallTimeLimit:          job.WallTimeLimit,
		MemoryLimit:            job.MemoryLimit,
		StackLimit:             job.StackLimit,
		MaxProcesses:           job.MaxProcesses,
		MaxFileSize:            job.MaxFileSize,
		CompilerOptions:        job.CompilerOptions,
		CommandLineArguments:   job.CommandLineArguments,
		RedirectStderrToStdout: job.RedirectStderrToStdout,
		EnableNetwork:          job.EnableNetwork,
		AdditionalFiles:        job.AdditionalFiles,
	}
}
