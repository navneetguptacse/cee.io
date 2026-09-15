package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"cee.io/pkg/languages"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type DockerExecutor struct {
	cli    *client.Client
	parser *ResultParser
}

func NewDockerExecutor(socketPath string) (*DockerExecutor, error) {
	host := "unix://" + socketPath
	if !strings.HasPrefix(socketPath, "/") && !strings.HasPrefix(socketPath, "unix://") {
		host = socketPath
	}
	cli, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerExecutor{
		cli:    cli,
		parser: NewResultParser(),
	}, nil
}

func (d *DockerExecutor) Type() string {
	return "docker"
}

func (d *DockerExecutor) Execute(ctx context.Context, sub *ExecutionSubmission) (*ExecutionResult, error) {
	lang := sub.Language

	// Calculate memory and box size
	effectiveMem := sub.MemoryLimit
	if lang.MinMemory > effectiveMem {
		effectiveMem = lang.MinMemory
	}
	boxSizeMB := effectiveMem / 1024
	if boxSizeMB < 128 {
		boxSizeMB = 128
	}

	// 1. Create secure sandbox container
	containerConfig := &container.Config{
		Image:        lang.Image,
		Cmd:          []string{"/bin/sh", "-c", "sleep 3600"},
		WorkingDir:   "/box",
		Tty:          false,
		OpenStdin:    true,
		StdinOnce:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	netMode := container.NetworkMode("none")
	if sub.EnableNetwork {
		netMode = container.NetworkMode("bridge")
	}

	hostConfig := &container.HostConfig{
		NetworkMode: netMode,
		Resources: container.Resources{
			Memory:     int64(effectiveMem) * 1024,
			MemorySwap: int64(effectiveMem) * 1024, // disable swap
			CPUPeriod:  100000,
			CPUQuota:   100000, // 1 CPU
			PidsLimit:  func() *int64 { v := int64(sub.MaxProcesses); return &v }(),
		},
		ReadonlyRootfs: true,
		SecurityOpt:    []string{"no-new-privileges"},
		CapDrop:        []string{"ALL"},
		MaskedPaths: []string{
			"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/gshadow",
			"/etc/hostname", "/etc/hosts", "/etc/resolv.conf",
			"/proc/kcore", "/proc/keys", "/proc/latency_stats",
			"/proc/timer_list", "/proc/timer_stats", "/proc/sched_debug",
			"/proc/scsi", "/proc/acpi", "/proc/bus",
			"/proc/1/environ", "/proc/1/cmdline", "/proc/1/maps",
			"/sys/firmware", "/sys/devices",
		},
		ReadonlyPaths: []string{"/proc", "/sys"},
		Tmpfs: map[string]string{
			"/tmp":  "rw,noexec,nosuid,size=64m,mode=1777",
			"/box":  fmt.Sprintf("rw,exec,nosuid,size=%dm,mode=1777", boxSizeMB),
			"/home": "rw,noexec,nosuid,size=16m,mode=1777",
		},
	}

	resp, err := d.cli.ContainerCreate(ctx, containerConfig, hostConfig, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox container: %w", err)
	}
	containerID := resp.ID

	defer func() {
		stopTimeout := 1
		_ = d.cli.ContainerStop(context.Background(), containerID, container.StopOptions{Timeout: &stopTimeout})
		_ = d.cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
	}()

	// 2. Start container
	if err := d.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// 3. Inject source code or multi-file archives
	if lang.ID == languages.LangMultiFile {
		if sub.AdditionalFiles == "" {
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: strPtr("Multi-file program requires a ZIP archive in additional_files"),
			}, nil
		}
		if err := d.copyAdditionalFiles(ctx, containerID, sub.AdditionalFiles); err != nil {
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: strPtr(err.Error()),
			}, nil
		}
		return d.executeMultiFile(ctx, containerID, sub)
	}

	if sub.SourceCode != "" && lang.SourceFile != "" {
		if err := d.writeFileToBox(ctx, containerID, lang.SourceFile, []byte(sub.SourceCode)); err != nil {
			return nil, err
		}
	}

	if sub.AdditionalFiles != "" {
		if err := d.copyAdditionalFiles(ctx, containerID, sub.AdditionalFiles); err != nil {
			return nil, err
		}
	}

	// 4. Compile step
	if lang.CompileCmd != "" {
		compileCmd := lang.CompileCmd
		if sub.CompilerOptions != "" {
			compileCmd += " " + sub.CompilerOptions
		}

		compileTimeout := sub.CPUTimeLimit + sub.CPUExtraTime
		if compileTimeout <= 0 {
			compileTimeout = 15.0
		}

		cRes, err := d.runCommand(ctx, containerID, compileCmd, int(compileTimeout), "")
		if err != nil {
			return nil, err
		}
		if cRes.ExitCode != 0 {
			output := cRes.Stderr
			if output == "" {
				output = cRes.Stdout
			}
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: &output,
				ExitCode:      &cRes.ExitCode,
			}, nil
		}
	}

	// 5. Run step
	runCmd := lang.RunCmd
	if sub.CommandLineArguments != "" {
		runCmd += " " + sub.CommandLineArguments
	}

	timeoutSecs := int(sub.WallTimeLimit)
	if timeoutSecs <= 0 {
		timeoutSecs = 10
	}

	raw, err := d.runCommand(ctx, containerID, runCmd, timeoutSecs, sub.Stdin)
	if err != nil {
		return nil, err
	}

	return d.parser.Parse(raw, sub), nil
}

func (d *DockerExecutor) executeMultiFile(ctx context.Context, containerID string, sub *ExecutionSubmission) (*ExecutionResult, error) {
	// Verify /box/run or /box/run.sh exists
	checkRun, err := d.runCommand(ctx, containerID, "test -f /box/run || test -f /box/run.sh", 5, "")
	if err != nil || checkRun.ExitCode != 0 {
		return &ExecutionResult{
			Status:        languages.GetStatusByID(languages.StatusCompilationError),
			CompileOutput: strPtr("Multi-file program requires a 'run' or 'run.sh' script in the archive"),
		}, nil
	}

	// Optional compile script
	checkCompile, _ := d.runCommand(ctx, containerID, "test -f /box/compile || test -f /box/compile.sh", 5, "")
	if checkCompile != nil && checkCompile.ExitCode == 0 {
		compRes, err := d.runCommand(ctx, containerID, "[ -f /box/compile ] && bash /box/compile || bash /box/compile.sh", 15, "")
		if err != nil {
			return nil, err
		}
		if compRes.ExitCode != 0 {
			output := compRes.Stderr
			if output == "" {
				output = compRes.Stdout
			}
			return &ExecutionResult{
				Status:        languages.GetStatusByID(languages.StatusCompilationError),
				CompileOutput: &output,
				ExitCode:      &compRes.ExitCode,
			}, nil
		}
	}

	// Run script
	timeoutSecs := int(sub.WallTimeLimit)
	if timeoutSecs <= 0 {
		timeoutSecs = 10
	}
	raw, err := d.runCommand(ctx, containerID, "[ -f /box/run ] && bash /box/run || bash /box/run.sh", timeoutSecs, sub.Stdin)
	if err != nil {
		return nil, err
	}

	return d.parser.Parse(raw, sub), nil
}

func (d *DockerExecutor) writeFileToBox(ctx context.Context, containerID, fileName string, content []byte) error {
	b64Content := base64.StdEncoding.EncodeToString(content)
	cmd := fmt.Sprintf("echo '%s' | base64 -d > '/box/%s'", b64Content, fileName)

	res, err := d.runCommand(ctx, containerID, cmd, 10, "")
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", fileName, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to write file %s: %s", fileName, res.Stderr)
	}
	return nil
}

func (d *DockerExecutor) copyAdditionalFiles(ctx context.Context, containerID string, b64Zip string) error {
	if err := d.writeFileToBox(ctx, containerID, "_additional.zip", []byte(b64Zip)); err != nil {
		return err
	}

	// Check path traversal inside container
	check, err := d.runCommand(ctx, containerID, `unzip -l /box/_additional.zip | grep -E '\.\./|/\.\.' && echo TRAVERSAL || echo OK`, 10, "")
	if err != nil || strings.Contains(check.Stdout, "TRAVERSAL") {
		_, _ = d.runCommand(ctx, containerID, "rm -f /box/_additional.zip", 5, "")
		return fmt.Errorf("ZIP archive contains dangerous path traversal entries")
	}

	// Extract
	extract, err := d.runCommand(ctx, containerID, "unzip -n -qq /box/_additional.zip -d /box && rm -f /box/_additional.zip", 15, "")
	if err != nil || extract.ExitCode != 0 {
		return fmt.Errorf("failed to extract ZIP archive: %s", extract.Stderr)
	}
	return nil
}

func (d *DockerExecutor) runCommand(ctx context.Context, containerID string, cmdStr string, timeoutSecs int, stdin string) (*RawExecution, error) {
	startTime := time.Now()
	timeoutDur := time.Duration(timeoutSecs) * time.Second
	cCtx, cancel := context.WithTimeout(ctx, timeoutDur)
	defer cancel()

	execConfig := container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", cmdStr},
		WorkingDir:   "/box",
		AttachStdin:  stdin != "",
		AttachStdout: true,
		AttachStderr: true,
	}

	execCreate, err := d.cli.ContainerExecCreate(cCtx, containerID, execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create container exec: %w", err)
	}

	attachResp, err := d.cli.ContainerExecAttach(cCtx, execCreate.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to container exec: %w", err)
	}
	defer attachResp.Close()

	if stdin != "" {
		go func() {
			_, _ = io.WriteString(attachResp.Conn, stdin)
			_ = attachResp.CloseWrite()
		}()
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	doneCh := make(chan error, 1)

	go func() {
		_, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)
		doneCh <- err
	}()

	var timedOut bool
	select {
	case <-cCtx.Done():
		timedOut = true
	case <-doneCh:
	}

	wallTime := time.Since(startTime).Seconds()

	inspect, err := d.cli.ContainerExecInspect(context.Background(), execCreate.ID)
	exitCode := 0
	if err == nil {
		exitCode = inspect.ExitCode
	}
	if timedOut {
		exitCode = 124
	}

	return &RawExecution{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		TimedOut: timedOut,
		WallTime: wallTime,
		CPUTime:  wallTime,
	}, nil
}
