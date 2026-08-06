package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// ScriptStrategy deploys by executing the project's configured deploy script.
// The script path is resolved against the project path when relative, so no
// script name is ever hardcoded.
type ScriptStrategy struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

// NewScriptStrategy returns a strategy that runs commands through the agent's
// command runner.
func NewScriptStrategy() *ScriptStrategy {
	return &ScriptStrategy{run: agent.RunCommand}
}

// NewScriptStrategyWithRun returns a strategy that runs commands through the
// given runner, for testing.
func NewScriptStrategyWithRun(run func(context.Context, string, ...string) ([]byte, error)) *ScriptStrategy {
	return &ScriptStrategy{run: run}
}

func (s *ScriptStrategy) Type() string { return project.StrategyScript }

func (s *ScriptStrategy) script(dc DeployContext) (string, error) {
	script := dc.DeployConfig.Script
	if script == "" {
		return "", &agent.ToolError{
			Code:       "deploy_script_not_configured",
			Message:    "deploy.script is not configured for the project.",
			Suggestion: "Set deploy.script in the project's agent config.",
		}
	}
	if !filepath.IsAbs(script) {
		script = filepath.Join(dc.WorkingDir, script)
	}
	return script, nil
}

// Validate checks that the deploy script exists and is executable before any
// command runs.
func (s *ScriptStrategy) Validate(dc DeployContext) error {
	path, err := s.script(dc)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return wrapError(&agent.ToolError{
			Code:       "deploy_script_not_found",
			Message:    fmt.Sprintf("deploy script not found: %s", path),
			Suggestion: "Ensure the deploy script exists at the configured path.",
		}, err)
	}
	if info.Mode()&0o111 == 0 {
		return &agent.ToolError{
			Code:       "deploy_script_not_executable",
			Message:    fmt.Sprintf("deploy script is not executable: %s", path),
			Suggestion: "Run chmod +x on the deploy script.",
		}
	}
	return nil
}

func (s *ScriptStrategy) Deploy(ctx context.Context, dc DeployContext) error {
	script, err := s.script(dc)
	if err != nil {
		return err
	}

	res, err := runCommand(ctx, s.run, script)
	if err != nil {
		return wrapError(&agent.ToolError{
			Code:       "deploy_script_failed",
			Message:    "deploy script failed.",
			Suggestion: "Check the deploy script output.",
		}, err)
	}
	if res.ExitCode != 0 {
		return &agent.ToolError{
			Code:       "deploy_script_failed",
			Message:    fmt.Sprintf("deploy script failed: %s", res.Stderr),
			Suggestion: "Check the deploy script output.",
		}
	}
	return nil
}

// TODO(strategy-primitives): when reusable deployment primitives are needed,
// expose script.execute without changing workflow.deploy.
