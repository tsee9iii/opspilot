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
	dockerInspectName        = "docker_inspect"
	dockerInspectDescription = "Understand the runtime configuration of a Docker container on an agent."
)

const dockerInspectInputSchema = `{
  "type": "object",
  "required": ["agent_id", "container"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to inspect the container"},
    "container": {"type": "string", "description": "Docker container name or ID"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const dockerInspectOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "container": {
      "type": "object",
      "properties": {
        "id": {"type": "string"},
        "name": {"type": "string"},
        "image": {"type": "string"},
        "state": {"type": "string"},
        "status": {"type": "string"},
        "restart_count": {"type": "integer"},
        "health": {"type": "string"},
        "started_at": {"type": "string"},
        "ports": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "container": {"type": "string"},
              "host": {"type": "string"}
            }
          }
        },
        "mounts": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "source": {"type": "string"},
              "destination": {"type": "string"}
            }
          }
        },
        "networks": {"type": "array", "items": {"type": "string"}}
      }
    },
    "error": {"type": "string"}
  }
}`

// DockerInspectTool dispatches the agent's docker.inspect tool through the
// existing command pipeline. The tool is strictly read-only (confirmation
// level none), so dispatched commands complete or fail without awaiting
// operator approval.
type DockerInspectTool struct {
	dispatch              *dispatch.DispatchUseCase
	defaultTimeoutSeconds int
}

func NewDockerInspectTool(dispatch *dispatch.DispatchUseCase) *DockerInspectTool {
	return &DockerInspectTool{dispatch: dispatch, defaultTimeoutSeconds: 300}
}

// SetDefaultTimeoutSeconds overrides the default timeout used when a call
// omits timeout_seconds.
func (t *DockerInspectTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeoutSeconds = seconds
	}
}

func (t *DockerInspectTool) Name() string        { return dockerInspectName }
func (t *DockerInspectTool) Description() string { return dockerInspectDescription }
func (t *DockerInspectTool) Category() string    { return CategoryInvestigation }
func (t *DockerInspectTool) InputSchema() json.RawMessage {
	return json.RawMessage(dockerInspectInputSchema)
}
func (t *DockerInspectTool) OutputSchema() json.RawMessage {
	return json.RawMessage(dockerInspectOutputSchema)
}

func (t *DockerInspectTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	container, err := requireString(args, "container")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := optionalTimeoutSeconds(args, "timeout_seconds", t.defaultTimeoutSeconds, 600)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]string{"container": container})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.DockerInspectTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("docker_inspect: %w", mapDispatchError(err))
	}
	return buildDockerInspectResult(resp), nil
}

var _ mcp.Tool = (*DockerInspectTool)(nil)

// dockerInspectOutput is the stable output of the docker_inspect tool.
type dockerInspectOutput struct {
	CommandID string          `json:"command_id"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	Container json.RawMessage `json:"container,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// buildDockerInspectResult shapes a dispatch outcome into the stable tool
// output.
func buildDockerInspectResult(resp dispatch.DispatchResponse) json.RawMessage {
	out := dockerInspectOutput{CommandID: resp.CommandID, Status: resp.Status}
	switch resp.Status {
	case "awaiting_approval":
		out.Message = "Awaiting operator approval before the agent inspects the container."
	case "completed":
		out.Container = resp.Result
	case "failed":
		out.Error = resp.Error
	}
	b, _ := json.Marshal(out)
	return b
}
