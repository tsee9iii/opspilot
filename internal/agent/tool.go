package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// CommandResult is the JSON shape produced by RunCommand.
type CommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// ToolError is a structured error a tool may return for a common, actionable
// failure. It carries a machine-readable code and remediation guidance so a
// workflow report can surface them alongside the failure.
type ToolError struct {
	Code       string `json:"error_code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

func (e *ToolError) Error() string { return e.Message }

// EmptyParameterSchema is the parameter schema of tools that accept no
// payload fields.
const EmptyParameterSchema = `{"type":"object","properties":{}}`

// BinaryAvailable verifies a binary a tool depends on is installed and
// responds to `binary --version`. It returns false with a reason otherwise.
// The check is cheap and only exercises the binary the tool itself needs.
func BinaryAvailable(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), binary string) (bool, string) {
	out, err := run(ctx, binary, "--version")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, binary + " is not installed"
		}
		return false, binary + " is not runnable"
	}
	var res CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return false, binary + " is not runnable"
	}
	if res.ExitCode != 0 {
		return false, binary + " is not runnable"
	}
	return true, ""
}

// RunCommand runs a single binary and returns stdout, stderr and the exit
// code as JSON. Context expiry (e.g. a policy timeout) surfaces as a
// "tool timed out" error.
func RunCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr) && ctx.Err() == nil:
			exitCode = exitErr.ExitCode()
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return nil, errors.New("tool timed out")
		default:
			return nil, fmt.Errorf("run %s: %w", path, err)
		}
	}

	return json.Marshal(CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	})
}
