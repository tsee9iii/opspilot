package deploy

import (
	"context"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// PM2Strategy deploys by reloading a pm2 process. When reload is not supported
// (older pm2 or a process that does not support it), it falls back to a
// restart of the same process.
type PM2Strategy struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

// NewPM2Strategy returns a strategy that runs commands through the agent's
// command runner.
func NewPM2Strategy() *PM2Strategy {
	return &PM2Strategy{run: agent.RunCommand}
}

// NewPM2StrategyWithRun returns a strategy that runs commands through the given
// runner, for testing.
func NewPM2StrategyWithRun(run func(context.Context, string, ...string) ([]byte, error)) *PM2Strategy {
	return &PM2Strategy{run: run}
}

func (s *PM2Strategy) Type() string { return project.StrategyPM2 }

func (s *PM2Strategy) process(dc DeployContext) (string, error) {
	process := dc.DeployConfig.Process
	if process == "" {
		return "", &agent.ToolError{
			Code:       "pm2_process_not_configured",
			Message:    "deploy.process is not configured for the project.",
			Suggestion: "Set deploy.process to the pm2 process name in the project's agent config.",
		}
	}
	return process, nil
}

// Validate checks the pm2 configuration before any command runs.
func (s *PM2Strategy) Validate(dc DeployContext) error {
	_, err := s.process(dc)
	return err
}

func (s *PM2Strategy) Deploy(ctx context.Context, dc DeployContext) error {
	process, err := s.process(dc)
	if err != nil {
		return err
	}

	reload, err := runCommand(ctx, s.run, "pm2", "reload", process)
	if err != nil {
		return wrapError(&agent.ToolError{
			Code:       "pm2_failed",
			Message:    "pm2 reload failed.",
			Suggestion: "Ensure pm2 is installed and the process is managed by pm2.",
		}, err)
	}
	if reload.ExitCode == 0 {
		return nil
	}

	restart, err := runCommand(ctx, s.run, "pm2", "restart", process)
	if err != nil {
		return wrapError(&agent.ToolError{
			Code:       "pm2_failed",
			Message:    "pm2 restart failed.",
			Suggestion: "Ensure pm2 is installed and the process is managed by pm2.",
		}, err)
	}
	if restart.ExitCode != 0 {
		return &agent.ToolError{
			Code:       "pm2_failed",
			Message:    fmt.Sprintf("pm2 reload failed (%s); restart failed: %s", reload.Stderr, restart.Stderr),
			Suggestion: "Check the pm2 process logs.",
		}
	}
	return nil
}

// TODO(strategy-primitives): when reusable deployment primitives are needed,
// expose pm2.reload without changing workflow.deploy.
