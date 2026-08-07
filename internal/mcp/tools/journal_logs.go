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
	journalLogsName        = "journal_logs"
	journalLogsDescription = "Inspect recent journal logs for one systemd service on an agent. Investigation only: it runs the agent's bounded, read-only `journalctl` command remotely and never modifies service state."
)

const journalLogsInputSchema = `{
  "type": "object",
  "required": ["agent_id", "service"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to read the journal logs"},
    "service": {"type": "string", "description": "Systemd service name"},
    "lines": {"type": "integer", "minimum": 1, "maximum": 1000, "default": 100, "description": "Number of log lines to retrieve"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const journalLogsOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "result": {
      "type": "object",
      "properties": {
        "service": {"type": "string"},
        "stdout": {"type": "string"},
        "stderr": {"type": "string"},
        "lines": {"type": "integer"}
      }
    },
    "error": {"type": "string"}
  }
}`

// JournalLogsTool dispatches the agent's journal.logs tool through the
// existing command pipeline. The tool is strictly read-only (confirmation
// level none), so dispatched commands complete or fail without awaiting
// operator approval.
type JournalLogsTool struct {
	investigationTool
}

func NewJournalLogsTool(dispatch *dispatch.DispatchUseCase) *JournalLogsTool {
	return &JournalLogsTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *JournalLogsTool) Name() string        { return journalLogsName }
func (t *JournalLogsTool) Description() string { return journalLogsDescription }
func (t *JournalLogsTool) Category() string    { return CategoryInvestigation }
func (t *JournalLogsTool) InputSchema() json.RawMessage {
	return json.RawMessage(journalLogsInputSchema)
}
func (t *JournalLogsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(journalLogsOutputSchema)
}

func (t *JournalLogsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	service, err := requireString(args, "service")
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

	payload, _ := json.Marshal(map[string]any{"service": service, "lines": lines})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.JournalLogsTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("journal_logs: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent reads journal logs."), nil
}

var _ mcp.Tool = (*JournalLogsTool)(nil)
