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
	workflowDeployName        = "workflow_deploy"
	workflowDeployDescription = "Deploy a project on an agent through its configured strategy and verify health."
)

const workflowDeployInputSchema = `{
  "type": "object",
  "required": ["agent_id", "project"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to run the deploy workflow"},
    "project": {"type": "string", "description": "Project name configured on the agent"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

// WorkflowDeployTool dispatches the agent's workflow.deploy tool through the
// existing command pipeline. Deploying requires confirmation, so the call
// returns awaiting_approval until an operator approves through the existing
// approval flow; approval is never bypassed.
type WorkflowDeployTool struct {
	dispatch              *dispatch.DispatchUseCase
	defaultTimeoutSeconds int
}

func NewWorkflowDeployTool(dispatch *dispatch.DispatchUseCase) *WorkflowDeployTool {
	return &WorkflowDeployTool{dispatch: dispatch, defaultTimeoutSeconds: 300}
}

// SetDefaultTimeoutSeconds overrides the default timeout used when a call
// omits timeout_seconds.
func (t *WorkflowDeployTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeoutSeconds = seconds
	}
}

func (t *WorkflowDeployTool) Name() string        { return workflowDeployName }
func (t *WorkflowDeployTool) Description() string { return workflowDeployDescription }
func (t *WorkflowDeployTool) InputSchema() json.RawMessage {
	return json.RawMessage(workflowDeployInputSchema)
}
func (t *WorkflowDeployTool) OutputSchema() json.RawMessage {
	return json.RawMessage(workflowOutputSchema)
}

func (t *WorkflowDeployTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	projectName, err := requireString(args, "project")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := optionalTimeoutSeconds(args, "timeout_seconds", t.defaultTimeoutSeconds, 600)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]string{"project": projectName})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.WorkflowDeployTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("workflow_deploy: %w", mapDispatchError(err))
	}
	return buildWorkflowResult(resp), nil
}

var _ mcp.Tool = (*WorkflowDeployTool)(nil)
