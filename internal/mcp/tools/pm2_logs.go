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
	pm2LogsName        = "pm2_logs"
	pm2LogsDescription = "Inspect recent logs for one PM2 process on an agent. Investigation only: it runs the agent's bounded, read-only `pm2 logs` command remotely and never modifies process state."
)

const pm2LogsInputSchema = `{
  "type": "object",
  "required": ["agent_id", "process"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to read the PM2 logs"},
    "process": {"type": "string", "description": "PM2 process name"},
    "lines": {"type": "integer", "minimum": 1, "maximum": 1000, "default": 100, "description": "Number of log lines to retrieve per stream"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const pm2LogsOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "result": {
      "type": "object",
      "properties": {
        "process": {"type": "string"},
        "stdout": {"type": "string"},
        "stderr": {"type": "string"},
        "lines": {"type": "integer"}
      }
    },
    "error": {"type": "string"}
  }
}`

// PM2LogsTool dispatches the agent's pm2.logs tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
type PM2LogsTool struct {
	investigationTool
}

func NewPM2LogsTool(dispatch *dispatch.DispatchUseCase) *PM2LogsTool {
	return &PM2LogsTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *PM2LogsTool) Name() string        { return pm2LogsName }
func (t *PM2LogsTool) Description() string { return pm2LogsDescription }
func (t *PM2LogsTool) Category() string    { return CategoryInvestigation }
func (t *PM2LogsTool) InputSchema() json.RawMessage {
	return json.RawMessage(pm2LogsInputSchema)
}
func (t *PM2LogsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(pm2LogsOutputSchema)
}

func (t *PM2LogsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	process, err := requireString(args, "process")
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

	payload, _ := json.Marshal(map[string]any{"process": process, "lines": lines})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.PM2LogsTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("pm2_logs: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent reads PM2 logs."), nil
}

var _ mcp.Tool = (*PM2LogsTool)(nil)
