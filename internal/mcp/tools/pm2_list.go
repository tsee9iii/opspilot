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
	pm2ListName        = "pm2_list"
	pm2ListDescription = "List and inspect PM2-managed processes on an agent. Investigation only: it runs the agent's bounded, read-only `pm2 jlist` command remotely and never modifies process state."
)

const pm2ListInputSchema = `{
  "type": "object",
  "required": ["agent_id"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to list PM2 processes"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const pm2ListOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "result": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "status": {"type": "string"},
          "pid": {"type": "integer"},
          "cpu_percent": {"type": "number"},
          "memory_bytes": {"type": "integer"},
          "uptime": {"type": "integer"}
        }
      }
    },
    "error": {"type": "string"}
  }
}`

// PM2ListTool dispatches the agent's pm2.list tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
type PM2ListTool struct {
	investigationTool
}

func NewPM2ListTool(dispatch *dispatch.DispatchUseCase) *PM2ListTool {
	return &PM2ListTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *PM2ListTool) Name() string        { return pm2ListName }
func (t *PM2ListTool) Description() string { return pm2ListDescription }
func (t *PM2ListTool) Category() string    { return CategoryInvestigation }
func (t *PM2ListTool) InputSchema() json.RawMessage {
	return json.RawMessage(pm2ListInputSchema)
}
func (t *PM2ListTool) OutputSchema() json.RawMessage {
	return json.RawMessage(pm2ListOutputSchema)
}

func (t *PM2ListTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := t.timeout(args)
	if err != nil {
		return nil, err
	}

	// pm2.list accepts no parameters; an empty object is still required so the
	// command pipeline rejects a command without a payload.
	payload, _ := json.Marshal(map[string]any{})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.PM2ListTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("pm2_list: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent lists PM2 processes."), nil
}

var _ mcp.Tool = (*PM2ListTool)(nil)
