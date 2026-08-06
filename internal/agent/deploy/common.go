package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// runCommand executes a command through the injected runner and decodes the
// agent.CommandResult shape every strategy produces.
func runCommand(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), name string, args ...string) (agent.CommandResult, error) {
	out, err := run(ctx, name, args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return agent.CommandResult{}, fmt.Errorf("%s is not installed", name)
		}
		return agent.CommandResult{}, fmt.Errorf("run %s: %w", name, err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return agent.CommandResult{}, fmt.Errorf("decode command result: %w", err)
	}
	return res, nil
}

// wrapError wraps an underlying error while keeping the structured ToolError
// reachable through errors.As.
func wrapError(te *agent.ToolError, err error) error {
	return fmt.Errorf("%w: %v", te, err)
}
