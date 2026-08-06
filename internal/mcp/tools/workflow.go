package tools

import (
	"encoding/json"
	"errors"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

// workflowResult is the stable output of a workflow execution tool.
type workflowResult struct {
	CommandID string          `json:"command_id"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	Report    json.RawMessage `json:"report,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// buildWorkflowResult shapes a dispatch outcome into the stable tool output.
func buildWorkflowResult(resp dispatch.DispatchResponse) json.RawMessage {
	out := workflowResult{CommandID: resp.CommandID, Status: resp.Status}
	switch resp.Status {
	case "awaiting_approval":
		out.Message = "Awaiting operator approval before the agent executes this workflow."
	case "completed":
		out.Report = resp.Result
	case "failed":
		out.Error = resp.Error
	}
	b, _ := json.Marshal(out)
	return b
}

// mapDispatchError converts dispatch/command errors into machine-readable
// tool errors; anything else is left for the generic internal_error mapping.
func mapDispatchError(err error) error {
	switch {
	case errors.Is(err, dispatch.ErrInvalidAgentID),
		errors.Is(err, appcommand.ErrInvalidAgentID):
		return &mcp.ToolError{
			Code:       "invalid_agent_id",
			Message:    "The agent id is not valid.",
			Suggestion: "Use list_agents to find a valid agent id.",
		}
	case errors.Is(err, dispatch.ErrTimeout):
		return &mcp.ToolError{
			Code:       "command_timeout",
			Message:    "The dispatched command did not reach a terminal state before the timeout.",
			Suggestion: "Retry the call or inspect the command with get_command.",
		}
	default:
		return err
	}
}
