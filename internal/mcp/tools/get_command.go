package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	getCommandName        = "get_command"
	getCommandDescription = "Get a command's current state, parameters and final result."
)

const getCommandInputSchema = `{
  "type": "object",
  "required": ["command_id"],
  "properties": {
    "command_id": {"type": "string", "description": "Command UUID"}
  }
}`

const getCommandOutputSchema = `{
  "type": "object",
  "required": ["command"],
  "properties": {
    "command": {
      "type": "object",
      "required": ["id", "agent_id", "status", "confirmation_status", "tool", "created_at"],
      "properties": {
        "id": {"type": "string"},
        "agent_id": {"type": "string"},
        "status": {"type": "string"},
        "confirmation_status": {"type": "string"},
        "tool": {"type": "string"},
        "parameters": {"type": "object"},
        "result": {"type": ["object", "null"]},
        "error": {"type": "string"},
        "created_at": {"type": "string"},
        "leased_at": {"type": ["string", "null"]},
        "completed_at": {"type": ["string", "null"]}
      }
    }
  }
}`

// GetCommandTool adapts the existing command query use case into MCP. The
// command's stored parameters and result are passed through unchanged.
type GetCommandTool struct {
	uc *command.GetCommandUseCase
}

func NewGetCommandTool(uc *command.GetCommandUseCase) *GetCommandTool {
	return &GetCommandTool{uc: uc}
}

func (t *GetCommandTool) Name() string                 { return getCommandName }
func (t *GetCommandTool) Description() string          { return getCommandDescription }
func (t *GetCommandTool) InputSchema() json.RawMessage { return json.RawMessage(getCommandInputSchema) }
func (t *GetCommandTool) OutputSchema() json.RawMessage {
	return json.RawMessage(getCommandOutputSchema)
}

type commandResult struct {
	ID                 string          `json:"id"`
	AgentID            string          `json:"agent_id"`
	Status             string          `json:"status"`
	ConfirmationStatus string          `json:"confirmation_status"`
	Tool               string          `json:"tool"`
	Parameters         json.RawMessage `json:"parameters"`
	Result             json.RawMessage `json:"result"`
	Error              string          `json:"error"`
	CreatedAt          string          `json:"created_at"`
	LeasedAt           *string         `json:"leased_at"`
	CompletedAt        *string         `json:"completed_at"`
}

func (t *GetCommandTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	commandID, err := requireString(args, "command_id")
	if err != nil {
		return nil, err
	}

	resp, err := t.uc.Get(ctx, command.GetCommandRequest{CommandID: commandID})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrInvalidCommandID):
			return nil, &mcp.ToolError{
				Code:       "invalid_command_id",
				Message:    "The command id is not a valid UUID.",
				Suggestion: "Provide a valid command UUID from list_commands.",
			}
		case errors.Is(err, command.ErrCommandNotFound):
			return nil, &mcp.ToolError{
				Code:       "command_not_found",
				Message:    "The command does not exist.",
				Suggestion: "Use list_commands to find a valid command id.",
			}
		default:
			return nil, fmt.Errorf("get_command: %w", err)
		}
	}

	out := commandResult{
		ID:                 resp.ID.String(),
		AgentID:            resp.AgentID.String(),
		Status:             resp.Status,
		ConfirmationStatus: resp.ConfirmationStatus,
		Tool:               resp.Tool,
		Parameters:         json.RawMessage(resp.Parameters),
		Result:             json.RawMessage(resp.Result),
		Error:              resp.Error,
		CreatedAt:          resp.CreatedAt.UTC().Format(time.RFC3339),
		LeasedAt:           formatTime(resp.LeasedAt),
		CompletedAt:        formatTime(resp.CompletedAt),
	}
	return json.Marshal(map[string]any{"command": out})
}

var _ mcp.Tool = (*GetCommandTool)(nil)
