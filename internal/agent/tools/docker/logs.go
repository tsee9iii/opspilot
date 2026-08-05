package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolDockerLogs            = "docker.logs"
	toolDockerLogsVersion     = "1.0.0"
	toolDockerLogsDescription = "Retrieve logs for a Docker container"

	defaultDockerLogLines = 100
	maxDockerLogLines     = 1000
)

const toolDockerLogsParameterSchema = `{
  "type": "object",
  "required": ["container"],
  "properties": {
    "container": {
      "type": "string",
      "description": "Docker container name or ID"
    },
    "lines": {
      "type": "integer",
      "minimum": 1,
      "maximum": 1000,
      "default": 100,
      "description": "Number of log lines to retrieve"
    }
  }
}`

type dockerLogsResult struct {
	Container string `json:"container"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Lines     int    `json:"lines"`
}

type dockerLogsRequest struct {
	Container string `json:"container"`
	Lines     *int   `json:"lines"`
}

// DockerLogsTool reports the recent logs of a Docker container via `docker
// logs`. It reuses docker.ps parsing logic to verify the container exists.
type DockerLogsTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewDockerLogsTool() *DockerLogsTool {
	return &DockerLogsTool{run: agent.RunCommand}
}

func (t *DockerLogsTool) Name() string {
	return ToolDockerLogs
}

func (t *DockerLogsTool) Version() string {
	return toolDockerLogsVersion
}

func (t *DockerLogsTool) Description() string {
	return toolDockerLogsDescription
}

func (t *DockerLogsTool) ParameterSchema() string {
	return toolDockerLogsParameterSchema
}

func (t *DockerLogsTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *DockerLogsTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "docker")
}

func (t *DockerLogsTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	container, lines, err := parseDockerLogsRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := dockerInstalled(ctx, t.run, "docker.logs"); err != nil {
		return nil, err
	}
	if err := containerExists(ctx, t.run, "docker.logs", container); err != nil {
		return nil, err
	}

	out, err := t.run(ctx, "docker", "logs", "--tail", strconv.Itoa(lines), container)
	if err != nil {
		return nil, fmt.Errorf("docker.logs: %w", err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("docker.logs: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("docker.logs: docker logs failed: %s", res.Stderr)
	}

	return json.Marshal(dockerLogsResult{
		Container: container,
		Stdout:    res.Stdout,
		Stderr:    res.Stderr,
		Lines:     lines,
	})
}

// parseDockerLogsRequest validates the payload against the tool's parameter
// schema: container is required, lines defaults to 100 and must be 1..1000.
func parseDockerLogsRequest(payload []byte) (string, int, error) {
	if len(payload) == 0 {
		return "", 0, errors.New("docker.logs: payload is required")
	}
	var req dockerLogsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", 0, fmt.Errorf("docker.logs: invalid payload: %w", err)
	}
	if req.Container == "" {
		return "", 0, errors.New("docker.logs: container is required")
	}
	lines := defaultDockerLogLines
	if req.Lines != nil {
		lines = *req.Lines
		if lines < 1 || lines > maxDockerLogLines {
			return "", 0, fmt.Errorf("docker.logs: lines must be between 1 and %d", maxDockerLogLines)
		}
	}
	return req.Container, lines, nil
}

// containerMatches reports whether a container's ID or any of its names equals
// idOrName (names may carry a leading "/").
func containerMatches(c dockerContainer, idOrName string) bool {
	if c.ID == idOrName {
		return true
	}
	idOrName = strings.TrimPrefix(idOrName, "/")
	for _, name := range strings.Split(c.Name, ", ") {
		if strings.TrimPrefix(name, "/") == idOrName {
			return true
		}
	}
	return false
}
