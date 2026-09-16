//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

func extractExitCode(exitErr *exec.ExitError) int {
	if ws, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ws.ExitStatus()
	}
	return exitErr.ExitCode()
}
