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
	dockerListName        = "docker_list"
	dockerListDescription = "List Docker containers and their runtime state on an agent. Investigation only: it runs the agent's bounded, read-only `docker ps` command remotely and never modifies container state."
)

const dockerListInputSchema = `{
  "type": "object",
  "required": ["agent_id"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to list Docker containers"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const dockerListOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "result": {
      "type": "object",
      "properties": {
        "containers": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "id": {"type": "string"},
              "name": {"type": "string"},
              "image": {"type": "string"},
              "state": {"type": "string"},
              "status": {"type": "string"},
              "ports": {"type": "string"}
            }
          }
        }
      }
    },
    "error": {"type": "string"}
  }
}`

// DockerListTool dispatches the agent's docker.ps tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
type DockerListTool struct {
	investigationTool
}

func NewDockerListTool(dispatch *dispatch.DispatchUseCase) *DockerListTool {
	return &DockerListTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *DockerListTool) Name() string        { return dockerListName }
func (t *DockerListTool) Description() string { return dockerListDescription }
func (t *DockerListTool) Category() string    { return CategoryInvestigation }
func (t *DockerListTool) InputSchema() json.RawMessage {
	return json.RawMessage(dockerListInputSchema)
}
func (t *DockerListTool) OutputSchema() json.RawMessage {
	return json.RawMessage(dockerListOutputSchema)
}

func (t *DockerListTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := t.timeout(args)
	if err != nil {
		return nil, err
	}

	// docker.ps accepts no parameters; an empty object is still required so the
	// command pipeline rejects a command without a payload.
	payload, _ := json.Marshal(map[string]any{})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.DockerPsTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("docker_list: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent lists Docker containers."), nil
}

var _ mcp.Tool = (*DockerListTool)(nil)
