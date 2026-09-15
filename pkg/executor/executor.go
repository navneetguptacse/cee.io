package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	"cee.io/pkg/config"
	"cee.io/pkg/languages"
)

// ExecutionSubmission contains all parameters required by an executor.
type ExecutionSubmission struct {
	Token                  string
	SourceCode             string
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
	AdditionalFiles        string // base64 encoded zip
}

// ExecutionResult encapsulates the outcome of running user code.
type ExecutionResult struct {
	Status        languages.Status
	Stdout        *string
	Stderr        *string
	CompileOutput *string
	Message       *string
	Time          *string
	WallTime      *string
	Memory        *int
	ExitCode      *int
	ExitSignal    *int
}

// RawExecution represents direct process output before parsing.
type RawExecution struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	WallTime float64
	CPUTime  float64
	MemoryKB int
}

// Executor defines the contract for code sandboxing engines.
type Executor interface {
	Execute(ctx context.Context, sub *ExecutionSubmission) (*ExecutionResult, error)
	Type() string
}

// SystemCapabilities provides discovery details on sandbox engines.
type SystemCapabilities struct {
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	Current     string `json:"current"`
	Recommended string `json:"recommended"`
	Docker      struct {
		Available bool   `json:"available"`
		Socket    string `json:"socket"`
	} `json:"docker"`
	Isolate struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason,omitempty"`
	} `json:"isolate"`
	Process struct {
		Available bool `json:"available"`
	} `json:"process"`
}

// IsIsolateAvailable checks if the isolate binary exists on the host.
func IsIsolateAvailable() (bool, string) {
	if runtime.GOOS != "linux" {
		return false, "isolate requires Linux OS"
	}
	_, err := exec.LookPath("isolate")
	if err != nil {
		return false, "isolate binary not found in PATH"
	}
	return true, ""
}

// IsDockerAvailable checks if Docker socket exists.
func IsDockerAvailable(socketPath string) bool {
	if fi, err := os.Stat(socketPath); err == nil {
		return fi.Mode()&os.ModeSocket != 0 || fi.Mode().IsRegular()
	}
	return false
}

// GetSystemCapabilities inspects host capabilities.
func GetSystemCapabilities(socketPath string, currentType string) SystemCapabilities {
	isoOk, isoReason := IsIsolateAvailable()
	dockOk := IsDockerAvailable(socketPath)

	recommended := "process"
	if isoOk {
		recommended = "isolate"
	} else if dockOk {
		recommended = "docker"
	}

	caps := SystemCapabilities{
		Platform:    runtime.GOOS,
		Arch:        runtime.GOARCH,
		Current:     currentType,
		Recommended: recommended,
	}
	caps.Isolate.Available = isoOk
	caps.Isolate.Reason = isoReason
	caps.Docker.Available = dockOk
	caps.Docker.Socket = socketPath
	caps.Process.Available = true

	return caps
}

// NewExecutor creates the appropriate executor based on configuration and system discovery.
func NewExecutor(cfg *config.Config) (Executor, error) {
	switch cfg.Executor.Type {
	case "isolate":
		if ok, reason := IsIsolateAvailable(); !ok {
			return nil, fmt.Errorf("isolate requested but not available: %s", reason)
		}
		slog.Info("executor_initialized", "type", "isolate")
		return NewIsolateExecutor(), nil

	case "docker":
		if !IsDockerAvailable(cfg.Docker.SocketPath) {
			return nil, fmt.Errorf("docker requested but socket not accessible at %s", cfg.Docker.SocketPath)
		}
		dex, err := NewDockerExecutor(cfg.Docker.SocketPath)
		if err != nil {
			return nil, err
		}
		slog.Info("executor_initialized", "type", "docker")
		return dex, nil

	case "process":
		slog.Info("executor_initialized", "type", "process")
		return NewProcessExecutor(), nil

	case "auto":
		fallthrough
	default:
		// Priority: isolate > docker > process
		if ok, _ := IsIsolateAvailable(); ok {
			slog.Info("executor_auto_selected", "type", "isolate")
			return NewIsolateExecutor(), nil
		}
		if IsDockerAvailable(cfg.Docker.SocketPath) {
			dex, err := NewDockerExecutor(cfg.Docker.SocketPath)
			if err == nil {
				slog.Info("executor_auto_selected", "type", "docker")
				return dex, nil
			}
		}
		slog.Info("executor_auto_selected", "type", "process (fast native sandbox)")
		return NewProcessExecutor(), nil
	}
}
