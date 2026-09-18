package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cee.io/pkg/languages"
	"cee.io/pkg/utils"
	"github.com/google/uuid"
)

// ProcessExecutor executes code directly as an isolated host subprocess.
type ProcessExecutor struct {
	parser *ResultParser
}

func NewProcessExecutor() *ProcessExecutor {
	return &ProcessExecutor{
		parser: NewResultParser(),
	}
}

func (e *ProcessExecutor) Type() string {
	return "process"
}

func (e *ProcessExecutor) Execute(ctx context.Context, sub *ExecutionSubmission) (*ExecutionResult, error) {
	boxID := uuid.New().String()[:8]
	boxDir := filepath.Join(os.TempDir(), fmt.Sprintf("cee-box-%s", boxID))
	if err := os.MkdirAll(boxDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create sandbox dir: %w", err)
	}
	defer os.RemoveAll(boxDir)

	lang := sub.Language

	if lang.ID == languages.LangMultiFile {
		if sub.AdditionalFiles == "" {
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: strPtr("Multi-file program requires a ZIP archive with a 'run' or 'run.sh' script"),
			}, nil
		}
		if err := utils.ExtractZipFromBase64(sub.AdditionalFiles, boxDir); err != nil {
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: strPtr(fmt.Sprintf("Failed to extract additional files: %v", err)),
			}, nil
		}
		return e.executeMultiFile(ctx, boxDir, sub)
	}

	if sub.SourceCode != "" && lang.SourceFile != "" {
		sourcePath := filepath.Join(boxDir, lang.SourceFile)
		if err := os.WriteFile(sourcePath, []byte(sub.SourceCode), 0600); err != nil {
			return nil, fmt.Errorf("failed to write source code: %w", err)
		}
	}

	if sub.AdditionalFiles != "" {
		if err := utils.ExtractZipFromBase64(sub.AdditionalFiles, boxDir); err != nil {
			return nil, fmt.Errorf("failed to extract additional files: %w", err)
		}
	}

	if lang.CompileCmd != "" {
		compileCmdStr := lang.CompileCmd
		if sub.CompilerOptions != "" {
			compileCmdStr += " " + sub.CompilerOptions
		}

		compileTimeout := time.Duration((sub.CPUTimeLimit + sub.CPUExtraTime) * float64(time.Second))
		if compileTimeout <= 0 {
			compileTimeout = 15 * time.Second
		}
		cCtx, cancel := context.WithTimeout(ctx, compileTimeout)
		defer cancel()

		cmd := exec.CommandContext(cCtx, "sh", "-c", compileCmdStr)
		cmd.Dir = boxDir
		prepareProcessGroup(cmd)
		cOut := newBoundedBuffer(MaxOutputLength * 2)
		cErr := newBoundedBuffer(MaxOutputLength * 2)
		cmd.Stdout = cOut
		cmd.Stderr = cErr

		err := cmd.Run()
		if err != nil || (cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0) {
			output := cErr.String()
			if output == "" {
				output = cOut.String()
			}
			if output == "" {
				output = err.Error()
			}
			exitCode := 1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: &output,
				ExitCode:      &exitCode,
			}, nil
		}
	}

	runCmdStr := lang.RunCmd
	if sub.CommandLineArguments != "" {
		runCmdStr += " " + sub.CommandLineArguments
	}

	timeoutSecs := sub.WallTimeLimit
	if timeoutSecs <= 0 {
		timeoutSecs = 10.0
	}
	rCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs*float64(time.Second)))
	defer cancel()

	startTime := time.Now()
	cmd := exec.CommandContext(rCtx, "sh", "-c", runCmdStr)
	cmd.Dir = boxDir
	prepareProcessGroup(cmd)
	if sub.Stdin != "" {
		cmd.Stdin = strings.NewReader(sub.Stdin)
	}

	stdoutBuf := newBoundedBuffer(MaxOutputLength * 2)
	stderrBuf := newBoundedBuffer(MaxOutputLength * 2)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	wallTime := time.Since(startTime).Seconds()

	timedOut := rCtx.Err() == context.DeadlineExceeded
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = extractExitCode(exitErr)
		} else if timedOut {
			exitCode = 124
		} else {
			exitCode = 1
		}
	}

	raw := &RawExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		TimedOut: timedOut,
		WallTime: wallTime,
		CPUTime:  wallTime,
		MemoryKB: 0,
	}

	return e.parser.Parse(raw, sub), nil
}

func (e *ProcessExecutor) executeMultiFile(ctx context.Context, boxDir string, sub *ExecutionSubmission) (*ExecutionResult, error) {
	runScript := ""
	for _, name := range []string{"run", "run.sh"} {
		if _, err := os.Stat(filepath.Join(boxDir, name)); err == nil {
			runScript = name
			break
		}
	}

	if runScript == "" {
		return &ExecutionResult{
			Status:        languages.GetStatusByID(languages.StatusCompilationError),
			CompileOutput: strPtr("Multi-file program requires a 'run' or 'run.sh' script in the ZIP archive"),
		}, nil
	}

	for _, name := range []string{"compile", "compile.sh"} {
		if _, err := os.Stat(filepath.Join(boxDir, name)); err == nil {
			cCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			cmd := exec.CommandContext(cCtx, "sh", "-c", "bash "+name)
			cmd.Dir = boxDir
			prepareProcessGroup(cmd)
			cOut := newBoundedBuffer(MaxOutputLength * 2)
			cErr := newBoundedBuffer(MaxOutputLength * 2)
			cmd.Stdout = cOut
			cmd.Stderr = cErr

			if err := cmd.Run(); err != nil {
				output := cErr.String()
				if output == "" {
					output = cOut.String()
				}
				exitCode := 1
				if cmd.ProcessState != nil {
					exitCode = cmd.ProcessState.ExitCode()
				}
				return &ExecutionResult{
					Status:        languages.GetStatusByID(languages.StatusCompilationError),
					CompileOutput: &output,
					ExitCode:      &exitCode,
				}, nil
			}
			break
		}
	}

	timeoutSecs := sub.WallTimeLimit
	if timeoutSecs <= 0 {
		timeoutSecs = 10.0
	}
	rCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs*float64(time.Second)))
	defer cancel()

	startTime := time.Now()
	cmd := exec.CommandContext(rCtx, "sh", "-c", "bash "+runScript)
	cmd.Dir = boxDir
	prepareProcessGroup(cmd)
	if sub.Stdin != "" {
		cmd.Stdin = strings.NewReader(sub.Stdin)
	}

	stdoutBuf := newBoundedBuffer(MaxOutputLength * 2)
	stderrBuf := newBoundedBuffer(MaxOutputLength * 2)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
	wallTime := time.Since(startTime).Seconds()
	timedOut := rCtx.Err() == context.DeadlineExceeded

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = extractExitCode(exitErr)
		} else if timedOut {
			exitCode = 124
		} else {
			exitCode = 1
		}
	}

	raw := &RawExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		TimedOut: timedOut,
		WallTime: wallTime,
		CPUTime:  wallTime,
	}

	return e.parser.Parse(raw, sub), nil
}

// boundedBuffer limits the maximum bytes written to prevent out-of-memory errors on runaway outputs.
type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	if b.buf.Len() >= b.limit {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		_, err := b.buf.Write(p[:remaining])
		return len(p), err
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) String() string {
	return b.buf.String()
}

func strPtr(s string) *string {
	return &s
}
