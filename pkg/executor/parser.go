package executor

import (
	"fmt"

	"cee.io/pkg/languages"
	"cee.io/pkg/utils"
)

const MaxOutputLength = 65536 // 64KB truncate limit

type ResultParser struct{}

func NewResultParser() *ResultParser {
	return &ResultParser{}
}

// Parse processes raw execution telemetry into a Judge0-compliant ExecutionResult.
func (p *ResultParser) Parse(raw *RawExecution, sub *ExecutionSubmission) *ExecutionResult {
	stdout := p.truncate(raw.Stdout)
	stderr := p.truncate(raw.Stderr)

	if sub.RedirectStderrToStdout {
		if stderr != "" {
			if stdout != "" {
				stdout += "\n" + stderr
			} else {
				stdout = stderr
			}
			stderr = ""
		}
	}

	var status languages.Status
	var exitSignal *int

	if raw.TimedOut {
		status = languages.GetStatusByID(languages.StatusTimeLimitExceeded)
	} else if raw.ExitCode == 0 {
		if sub.ExpectedOutput != "" && !utils.CompareOutput(stdout, sub.ExpectedOutput) {
			status = languages.GetStatusByID(languages.StatusWrongAnswer)
		} else {
			status = languages.GetStatusByID(languages.StatusAccepted)
		}
	} else {
		switch raw.ExitCode {
		case 137, 9:
			status = languages.GetStatusByID(languages.StatusTimeLimitExceeded)
			sig := 9
			exitSignal = &sig
		case 139, 11:
			status = languages.GetStatusByID(languages.StatusRuntimeErrorSIGSEGV)
			sig := 11
			exitSignal = &sig
		case 136, 8:
			status = languages.GetStatusByID(languages.StatusRuntimeErrorSIGFPE)
			sig := 8
			exitSignal = &sig
		case 134, 6:
			status = languages.GetStatusByID(languages.StatusRuntimeErrorSIGABRT)
			sig := 6
			exitSignal = &sig
		case 153, 25:
			status = languages.GetStatusByID(languages.StatusRuntimeErrorSIGXFSZ)
			sig := 25
			exitSignal = &sig
		default:
			status = languages.GetStatusByID(languages.StatusRuntimeErrorNZEC)
			if raw.ExitCode > 128 {
				sig := raw.ExitCode - 128
				exitSignal = &sig
			}
		}
	}

	var pStdout *string
	if stdout != "" {
		pStdout = &stdout
	}

	var pStderr *string
	if stderr != "" {
		pStderr = &stderr
	}

	wallTimeStr := fmt.Sprintf("%.3f", raw.WallTime)
	cpuTimeStr := fmt.Sprintf("%.3f", raw.CPUTime)
	mem := raw.MemoryKB
	exitCode := raw.ExitCode

	return &ExecutionResult{
		Status:     status,
		Stdout:     pStdout,
		Stderr:     pStderr,
		Time:       &cpuTimeStr,
		WallTime:   &wallTimeStr,
		Memory:     &mem,
		ExitCode:   &exitCode,
		ExitSignal: exitSignal,
	}
}

func (p *ResultParser) truncate(str string) string {
	if len(str) <= MaxOutputLength {
		return str
	}
	return str[:MaxOutputLength] + "\n[truncated]"
}
