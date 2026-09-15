package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"cee.io/pkg/languages"
	"cee.io/pkg/utils"
)

var globalBoxCounter uint64

type IsolateExecutor struct {
	parser *ResultParser
}

func NewIsolateExecutor() *IsolateExecutor {
	return &IsolateExecutor{
		parser: NewResultParser(),
	}
}

func (e *IsolateExecutor) Type() string {
	return "isolate"
}

func (e *IsolateExecutor) allocateBoxID() int {
	val := atomic.AddUint64(&globalBoxCounter, 1)
	return int(val % 1000)
}

func (e *IsolateExecutor) Execute(ctx context.Context, sub *ExecutionSubmission) (*ExecutionResult, error) {
	boxID := e.allocateBoxID()
	lang := sub.Language

	initCmd := exec.CommandContext(ctx, "isolate", "--init", fmt.Sprintf("--box-id=%d", boxID), "--cg")
	initOut, err := initCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("isolate --init failed: %w", err)
	}
	boxDir := strings.TrimSpace(string(initOut))
	workDir := filepath.Join(boxDir, "box")

	defer func() {
		_ = exec.Command("isolate", "--cleanup", fmt.Sprintf("--box-id=%d", boxID), "--cg").Run()
	}()

	if lang.ID == languages.LangMultiFile {
		if sub.AdditionalFiles == "" {
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: strPtr("Multi-file program requires a ZIP archive with a 'run' or 'run.sh' script"),
			}, nil
		}
		if err := utils.ExtractZipFromBase64(sub.AdditionalFiles, workDir); err != nil {
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: strPtr(fmt.Sprintf("Failed to extract additional files: %v", err)),
			}, nil
		}
		return e.executeMultiFile(ctx, boxID, workDir, sub)
	}

	if sub.SourceCode != "" && lang.SourceFile != "" {
		if err := os.WriteFile(filepath.Join(workDir, lang.SourceFile), []byte(sub.SourceCode), 0644); err != nil {
			return nil, err
		}
	}

	if sub.AdditionalFiles != "" {
		if err := utils.ExtractZipFromBase64(sub.AdditionalFiles, workDir); err != nil {
			return nil, err
		}
	}

	stdinPath := filepath.Join(workDir, "_stdin.txt")
	if err := os.WriteFile(stdinPath, []byte(sub.Stdin), 0644); err != nil {
		return nil, err
	}

	if lang.CompileCmd != "" {
		compileCmd := lang.CompileCmd
		if sub.CompilerOptions != "" {
			compileCmd += " " + sub.CompilerOptions
		}

		compileTimeout := sub.CPUTimeLimit + sub.CPUExtraTime
		if compileTimeout <= 0 {
			compileTimeout = 15.0
		}

		rawCompile, err := e.runIsolate(ctx, boxID, workDir, compileCmd, "", int(compileTimeout), 30, sub.MemoryLimit, sub.MaxProcesses)
		if err != nil {
			return nil, err
		}
		if rawCompile.ExitCode != 0 {
			out := rawCompile.Stderr
			if out == "" {
				out = rawCompile.Stdout
			}
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: &out,
				ExitCode:      &rawCompile.ExitCode,
			}, nil
		}
	}

	runCmd := lang.RunCmd
	if sub.CommandLineArguments != "" {
		runCmd += " " + sub.CommandLineArguments
	}

	wallTimeout := int(sub.WallTimeLimit)
	if wallTimeout <= 0 {
		wallTimeout = 10
	}
	cpuTimeout := int(sub.CPUTimeLimit)
	if cpuTimeout <= 0 {
		cpuTimeout = 5
	}

	raw, err := e.runIsolate(ctx, boxID, workDir, runCmd, "/box/_stdin.txt", cpuTimeout, wallTimeout, sub.MemoryLimit, sub.MaxProcesses)
	if err != nil {
		return nil, err
	}

	return e.parser.Parse(raw, sub), nil
}

func (e *IsolateExecutor) executeMultiFile(ctx context.Context, boxID int, workDir string, sub *ExecutionSubmission) (*ExecutionResult, error) {
	runScript := ""
	for _, name := range []string{"run", "run.sh"} {
		if _, err := os.Stat(filepath.Join(workDir, name)); err == nil {
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
		if _, err := os.Stat(filepath.Join(workDir, name)); err == nil {
			cRaw, err := e.runIsolate(ctx, boxID, workDir, "bash /box/"+name, "", 15, 30, sub.MemoryLimit, sub.MaxProcesses)
			if err != nil {
				return nil, err
			}
			if cRaw.ExitCode != 0 {
				out := cRaw.Stderr
				if out == "" {
					out = cRaw.Stdout
				}
				return &ExecutionResult{
					Status:        languages.GetStatusByID(languages.StatusCompilationError),
					CompileOutput: &out,
					ExitCode:      &cRaw.ExitCode,
				}, nil
			}
			break
		}
	}

	wallTimeout := int(sub.WallTimeLimit)
	if wallTimeout <= 0 {
		wallTimeout = 10
	}
	cpuTimeout := int(sub.CPUTimeLimit)
	if cpuTimeout <= 0 {
		cpuTimeout = 5
	}

	stdinPath := filepath.Join(workDir, "_stdin.txt")
	_ = os.WriteFile(stdinPath, []byte(sub.Stdin), 0644)

	raw, err := e.runIsolate(ctx, boxID, workDir, "bash /box/"+runScript, "/box/_stdin.txt", cpuTimeout, wallTimeout, sub.MemoryLimit, sub.MaxProcesses)
	if err != nil {
		return nil, err
	}

	return e.parser.Parse(raw, sub), nil
}

func (e *IsolateExecutor) runIsolate(ctx context.Context, boxID int, workDir string, cmdStr string, sandboxStdin string, timeLimit int, wallLimit int, memLimit int, processes int) (*RawExecution, error) {
	metaFile := filepath.Join(os.TempDir(), fmt.Sprintf("isolate-meta-%d.txt", boxID))
	defer os.Remove(metaFile)

	args := []string{
		fmt.Sprintf("--box-id=%d", boxID),
		"--cg",
		fmt.Sprintf("--cg-mem=%d", memLimit),
		fmt.Sprintf("--meta=%s", metaFile),
		fmt.Sprintf("--time=%d", timeLimit),
		fmt.Sprintf("--wall-time=%d", wallLimit),
		fmt.Sprintf("--processes=%d", processes),
		"--fsize=10240",
		"--stdout=/box/_stdout.txt",
		"--stderr=/box/_stderr.txt",
		"--dir=/etc:noexec",
		"--dir=/tmp=",
		"--env=PATH=/usr/local/bin:/usr/bin:/bin",
		"--env=HOME=/box",
		"--env=LANG=C.UTF-8",
	}

	if sandboxStdin != "" {
		args = append(args, fmt.Sprintf("--stdin=%s", sandboxStdin))
	}

	args = append(args, "--run", "--", "/bin/sh", "-c", "cd /box && "+cmdStr)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(wallLimit+5)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "isolate", args...)
	_ = cmd.Run()

	stdoutB, _ := os.ReadFile(filepath.Join(workDir, "_stdout.txt"))
	stderrB, _ := os.ReadFile(filepath.Join(workDir, "_stderr.txt"))

	meta := parseIsolateMeta(metaFile)

	return &RawExecution{
		Stdout:   string(stdoutB),
		Stderr:   string(stderrB),
		ExitCode: meta.exitCode,
		TimedOut: meta.status == "TO",
		WallTime: meta.wallTime,
		CPUTime:  meta.time,
		MemoryKB: meta.memKB,
	}, nil
}

type isolateMeta struct {
	time     float64
	wallTime float64
	memKB    int
	exitCode int
	status   string
}

func parseIsolateMeta(metaPath string) isolateMeta {
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return isolateMeta{}
	}

	res := isolateMeta{}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		switch k {
		case "time":
			res.time, _ = strconv.ParseFloat(v, 64)
		case "time-wall":
			res.wallTime, _ = strconv.ParseFloat(v, 64)
		case "cg-mem", "max-rss":
			if res.memKB == 0 {
				res.memKB, _ = strconv.Atoi(v)
			}
		case "exitcode":
			res.exitCode, _ = strconv.Atoi(v)
		case "status":
			res.status = v
		}
	}
	return res
}
