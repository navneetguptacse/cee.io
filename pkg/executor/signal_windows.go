//go:build windows

package executor

import "os/exec"

func extractExitCode(exitErr *exec.ExitError) int {
	return exitErr.ExitCode()
}

