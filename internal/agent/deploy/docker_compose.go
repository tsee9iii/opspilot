package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// DockerComposeStrategy deploys via the docker compose CLI: it pulls the
// configured images and recreates the containers with `up -d` against the
// project's compose file.
type DockerComposeStrategy struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

// NewDockerComposeStrategy returns a strategy that runs commands through the
// agent's command runner.
func NewDockerComposeStrategy() *DockerComposeStrategy {
	return &DockerComposeStrategy{run: agent.RunCommand}
}

// NewDockerComposeStrategyWithRun returns a strategy that runs commands through
// the given runner, for testing.
func NewDockerComposeStrategyWithRun(run func(context.Context, string, ...string) ([]byte, error)) *DockerComposeStrategy {
	return &DockerComposeStrategy{run: run}
}

func (s *DockerComposeStrategy) Type() string { return project.StrategyDockerCompose }

// composeFile resolves the configured compose file against the working
// directory.
func (s *DockerComposeStrategy) composeFile(dc DeployContext) (string, error) {
	composeFile := dc.DeployConfig.ComposeFile
	if composeFile == "" {
		return "", &agent.ToolError{
			Code:       "compose_file_not_configured",
			Message:    "compose_file is not configured for the project.",
			Suggestion: "Set deploy.compose_file in the project's agent config.",
		}
	}
	if !filepath.IsAbs(composeFile) {
		composeFile = filepath.Join(dc.WorkingDir, composeFile)
	}
	return composeFile, nil
}

// Validate checks the docker-compose configuration before any command runs.
func (s *DockerComposeStrategy) Validate(dc DeployContext) error {
	path, err := s.composeFile(dc)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return wrapError(&agent.ToolError{
			Code:       "compose_file_not_found",
			Message:    fmt.Sprintf("compose file not found: %s", path),
			Suggestion: "Ensure the compose file exists at the configured path.",
		}, err)
	}
	return nil
}

func (s *DockerComposeStrategy) Deploy(ctx context.Context, dc DeployContext) error {
	composeFile, err := s.composeFile(dc)
	if err != nil {
		return err
	}

	actions := []struct {
		name string
		args []string
	}{
		{name: "docker compose pull", args: []string{"compose", "-f", composeFile, "pull"}},
		{name: "docker compose up -d", args: []string{"compose", "-f", composeFile, "up", "-d"}},
	}
	for _, action := range actions {
		res, err := runCommand(ctx, s.run, "docker", action.args...)
		if err != nil {
			return wrapError(&agent.ToolError{
				Code:       "docker_compose_failed",
				Message:    fmt.Sprintf("%s failed.", action.name),
				Suggestion: "Check the docker compose configuration and daemon status.",
			}, err)
		}
		if res.ExitCode != 0 {
			return &agent.ToolError{
				Code:       "docker_compose_failed",
				Message:    fmt.Sprintf("%s failed: %s", action.name, res.Stderr),
				Suggestion: "Check the docker compose configuration and daemon status.",
			}
		}
	}
	return nil
}

// TODO(strategy-primitives): when reusable deployment primitives are needed,
// expose docker.compose.pull and docker.compose.up without changing
// workflow.deploy.
