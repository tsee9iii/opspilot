package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

type commandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// runCommand runs a single binary and returns stdout, stderr and the exit
// code as JSON. Context expiry (e.g. a policy timeout) surfaces as a
// "tool timed out" error.
func runCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
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

	return json.Marshal(commandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	})
}
