package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	dockerLogsName        = "docker_logs"
	dockerLogsDescription = "Inspect recent logs for one Docker container on an agent. Investigation only: it runs the agent's bounded, read-only `docker logs` command remotely and never modifies container state."
)

const dockerLogsInputSchema = `{
  "type": "object",
  "required": ["agent_id", "container"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to read the Docker logs"},
    "container": {"type": "string", "description": "Docker container name or ID"},
    "lines": {"type": "integer", "minimum": 1, "maximum": 1000, "default": 100, "description": "Number of log lines to retrieve"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const dockerLogsOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "result": {
      "type": "object",
      "properties": {
        "container": {"type": "string"},
        "stdout": {"type": "string"},
        "stderr": {"type": "string"},
        "lines": {"type": "integer"}
      }
    },
    "error": {"type": "string"}
  }
}`

// DockerLogsTool dispatches the agent's docker.logs tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
type DockerLogsTool struct {
	investigationTool
}

func NewDockerLogsTool(dispatch *dispatch.DispatchUseCase) *DockerLogsTool {
	return &DockerLogsTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *DockerLogsTool) Name() string        { return dockerLogsName }
func (t *DockerLogsTool) Description() string { return dockerLogsDescription }
func (t *DockerLogsTool) Category() string    { return CategoryInvestigation }
func (t *DockerLogsTool) InputSchema() json.RawMessage {
	return json.RawMessage(dockerLogsInputSchema)
}
func (t *DockerLogsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(dockerLogsOutputSchema)
}

func (t *DockerLogsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	container, err := requireString(args, "container")
	if err != nil {
		return nil, err
	}
	lines, err := optionalLines(args, "lines")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := t.timeout(args)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]any{"container": container, "lines": lines})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.DockerLogsTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("docker_logs: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent reads Docker logs."), nil
}

var _ mcp.Tool = (*DockerLogsTool)(nil)
