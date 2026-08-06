package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	listCommandsName        = "list_commands"
	listCommandsDescription = "List recent commands as lightweight summaries, optionally filtered by status or agent_id."
)

const listCommandsInputSchema = `{
  "type": "object",
  "properties": {
    "status": {"type": "string", "description": "Filter by command status (pending, leased, running, completed, failed)"},
    "agent_id": {"type": "string", "description": "Filter by agent UUID"},
    "limit": {"type": "integer", "description": "Maximum number of commands to return (default 50, max 500)"}
  }
}`

const listCommandsOutputSchema = `{
  "type": "object",
  "required": ["commands"],
  "properties": {
    "commands": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "agent_id", "tool", "status", "created_at"],
        "properties": {
          "id": {"type": "string"},
          "agent_id": {"type": "string"},
          "tool": {"type": "string"},
          "status": {"type": "string"},
          "created_at": {"type": "string"}
        }
      }
    }
  }
}`

// ListCommandsTool adapts the inventory command projection into MCP. Payloads
// and result blobs are never part of the summary.
type ListCommandsTool struct {
	uc *inventory.ListCommandsUseCase
}

func NewListCommandsTool(uc *inventory.ListCommandsUseCase) *ListCommandsTool {
	return &ListCommandsTool{uc: uc}
}

func (t *ListCommandsTool) Name() string        { return listCommandsName }
func (t *ListCommandsTool) Description() string { return listCommandsDescription }
func (t *ListCommandsTool) Category() string    { return CategoryInventory }
func (t *ListCommandsTool) InputSchema() json.RawMessage {
	return json.RawMessage(listCommandsInputSchema)
}
func (t *ListCommandsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(listCommandsOutputSchema)
}

type commandSummaryResult struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (t *ListCommandsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	status, err := optionalString(args, "status")
	if err != nil {
		return nil, err
	}
	agentID, err := optionalUUID(args, "agent_id")
	if err != nil {
		return nil, err
	}
	limit, err := optionalInt(args, "limit", inventory.DefaultLimit)
	if err != nil {
		return nil, err
	}

	commands, err := t.uc.List(ctx, inventory.ListCommandsRequest{
		Status:  status,
		AgentID: agentID,
		Limit:   int32(limit),
	})
	if err != nil {
		switch {
		case errors.Is(err, inventory.ErrInvalidAgentID):
			return nil, &mcp.ToolError{
				Code:       "invalid_agent_id",
				Message:    "The agent_id filter is not a valid UUID.",
				Suggestion: "Use list_agents to find a valid agent id.",
			}
		case errors.Is(err, inventory.ErrInvalidLimit):
			return nil, &mcp.ToolError{
				Code:       "invalid_limit",
				Message:    "The limit is outside the allowed range.",
				Suggestion: "Provide a limit between 1 and 500.",
			}
		default:
			return nil, fmt.Errorf("list_commands: %w", err)
		}
	}
	out := make([]commandSummaryResult, 0, len(commands))
	for _, c := range commands {
		out = append(out, commandSummaryResult{
			ID:        c.ID.String(),
			AgentID:   c.AgentID.String(),
			Tool:      c.Tool,
			Status:    c.Status,
			CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return json.Marshal(map[string]any{"commands": out})
}

var _ mcp.Tool = (*ListCommandsTool)(nil)
