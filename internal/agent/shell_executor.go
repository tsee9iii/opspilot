package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

const ToolShellExec = "shell.exec"

type shellExecPayload struct {
	Command string `json:"command"`
}

type shellExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// ShellExecutor runs shell.exec commands via exec.CommandContext and captures
// stdout, stderr and the exit code as a JSON result. It consults the
// execution policy before starting any command.
type ShellExecutor struct {
	policy ExecutionPolicy
}

func NewShellExecutor(policy ExecutionPolicy) *ShellExecutor {
	return &ShellExecutor{policy: policy}
}

func (e *ShellExecutor) Execute(ctx context.Context, tool string, payload []byte) ([]byte, error) {
	if tool != ToolShellExec {
		return nil, ErrToolNotImplemented
	}

	var req shellExecPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("shell.exec: parse payload: %w", err)
	}
	if req.Command == "" {
		return nil, errors.New("shell.exec: command is required")
	}

	if err := e.policy.Allow(commandName(req.Command)); err != nil {
		return nil, fmt.Errorf("shell.exec: %w", err)
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if e.policy.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, e.policy.Timeout)
		defer cancel()
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(execCtx, "sh", "-c", req.Command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if e.policy.WorkingDirectory != "" {
		cmd.Dir = e.policy.WorkingDirectory
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr) && execCtx.Err() == nil:
			exitCode = exitErr.ExitCode()
		case errors.Is(execCtx.Err(), context.DeadlineExceeded):
			return nil, errors.New("shell.exec: command timed out")
		case errors.Is(execCtx.Err(), context.Canceled):
			return nil, fmt.Errorf("shell.exec: command canceled: %w", err)
		default:
			return nil, fmt.Errorf("shell.exec: run command: %w", err)
		}
	}

	return json.Marshal(shellExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	})
}
