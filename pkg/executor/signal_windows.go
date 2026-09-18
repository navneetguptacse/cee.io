//go:build windows

package executor

import "os/exec"

func extractExitCode(exitErr *exec.ExitError) int {
	return exitErr.ExitCode()
}

func prepareProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
