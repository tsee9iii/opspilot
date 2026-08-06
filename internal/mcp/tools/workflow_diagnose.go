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
	workflowDiagnoseName        = "workflow_diagnose"
	workflowDiagnoseDescription = "Run the host diagnostic workflow on an agent and wait for its report."
)

const workflowDiagnoseInputSchema = `{
  "type": "object",
  "required": ["agent_id"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to run the diagnostic workflow"},
    "service": {"type": "string", "description": "Optional systemd service to include a systemctl.status step"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const workflowOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "report": {"type": "object"},
    "error": {"type": "string"}
  }
}`

// WorkflowDiagnoseTool dispatches the agent's workflow.diagnose tool through the
// existing command pipeline. Approval is never bypassed: commands requiring
// confirmation return immediately as awaiting_approval.
type WorkflowDiagnoseTool struct {
	dispatch              *dispatch.DispatchUseCase
	defaultTimeoutSeconds int
}

func NewWorkflowDiagnoseTool(dispatch *dispatch.DispatchUseCase) *WorkflowDiagnoseTool {
	return &WorkflowDiagnoseTool{dispatch: dispatch, defaultTimeoutSeconds: 300}
}

// SetDefaultTimeoutSeconds overrides the default timeout used when a call
// omits timeout_seconds.
func (t *WorkflowDiagnoseTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeoutSeconds = seconds
	}
}

func (t *WorkflowDiagnoseTool) Name() string        { return workflowDiagnoseName }
func (t *WorkflowDiagnoseTool) Description() string { return workflowDiagnoseDescription }
func (t *WorkflowDiagnoseTool) InputSchema() json.RawMessage {
	return json.RawMessage(workflowDiagnoseInputSchema)
}
func (t *WorkflowDiagnoseTool) OutputSchema() json.RawMessage {
	return json.RawMessage(workflowOutputSchema)
}

func (t *WorkflowDiagnoseTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	service, err := optionalString(args, "service")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := optionalTimeoutSeconds(args, "timeout_seconds", t.defaultTimeoutSeconds, 600)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]string{"service": service})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.WorkflowDiagnoseTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow_diagnose: %w", mapDispatchError(err))
	}
	return buildWorkflowResult(resp), nil
}

var _ mcp.Tool = (*WorkflowDiagnoseTool)(nil)
