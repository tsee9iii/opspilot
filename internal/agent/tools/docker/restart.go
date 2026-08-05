package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolDockerRestart            = "docker.restart"
	toolDockerRestartVersion     = "1.0.0"
	toolDockerRestartDescription = "Restart a Docker container"
)

const toolDockerRestartParameterSchema = `{
  "type": "object",
  "required": ["container"],
  "properties": {
    "container": {
      "type": "string",
      "description": "Docker container name or ID"
    }
  }
}`

type dockerRestartResult struct {
	Container string `json:"container"`
	Status    string `json:"status"`
}

type dockerRestartRequest struct {
	Container string `json:"container"`
}

// DockerRestartTool restarts a running Docker container via `docker restart`.
type DockerRestartTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewDockerRestartTool() *DockerRestartTool {
	return &DockerRestartTool{run: agent.RunCommand}
}

func (t *DockerRestartTool) Name() string {
	return ToolDockerRestart
}

func (t *DockerRestartTool) Version() string {
	return toolDockerRestartVersion
}

func (t *DockerRestartTool) Description() string {
	return toolDockerRestartDescription
}

func (t *DockerRestartTool) ParameterSchema() string {
	return toolDockerRestartParameterSchema
}

func (t *DockerRestartTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationRequired
}

func (t *DockerRestartTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	container, err := parseDockerRestartRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := dockerInstalled(ctx, t.run, "docker.restart"); err != nil {
		return nil, err
	}
	if err := containerExists(ctx, t.run, "docker.restart", container); err != nil {
		return nil, err
	}

	out, err := t.run(ctx, "docker", "restart", container)
	if err != nil {
		return nil, fmt.Errorf("docker.restart: %w", err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("docker.restart: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("docker.restart: docker restart failed: %s", res.Stderr)
	}

	return json.Marshal(dockerRestartResult{
		Container: container,
		Status:    "restarted",
	})
}

// parseDockerRestartRequest validates the payload against the tool's
// parameter schema: container is required.
func parseDockerRestartRequest(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("docker.restart: payload is required")
	}
	var req dockerRestartRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("docker.restart: invalid payload: %w", err)
	}
	if req.Container == "" {
		return "", errors.New("docker.restart: container is required")
	}
	return req.Container, nil
}
