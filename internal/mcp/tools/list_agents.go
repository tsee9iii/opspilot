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
	listAgentsName        = "list_agents"
	listAgentsDescription = "List agents and their server context, optionally filtered by server_id or status."
)

const listAgentsInputSchema = `{
  "type": "object",
  "properties": {
    "server_id": {"type": "string", "description": "Filter by server UUID"},
    "status": {"type": "string", "description": "Filter by agent status (online, offline, unregistered)"}
  }
}`

const listAgentsOutputSchema = `{
  "type": "object",
  "required": ["agents"],
  "properties": {
    "agents": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "server", "hostname", "environment", "version", "status", "last_heartbeat"],
        "properties": {
          "id": {"type": "string"},
          "server": {"type": "string"},
          "hostname": {"type": "string"},
          "environment": {"type": "string"},
          "version": {"type": "string"},
          "status": {"type": "string"},
          "last_heartbeat": {"type": ["string", "null"]}
        }
      }
    }
  }
}`

// ListAgentsTool adapts the inventory agent projection into MCP. The stored
// secret hash is never projected.
type ListAgentsTool struct {
	uc *inventory.ListAgentsUseCase
}

func NewListAgentsTool(uc *inventory.ListAgentsUseCase) *ListAgentsTool {
	return &ListAgentsTool{uc: uc}
}

func (t *ListAgentsTool) Name() string                 { return listAgentsName }
func (t *ListAgentsTool) Description() string          { return listAgentsDescription }
func (t *ListAgentsTool) InputSchema() json.RawMessage { return json.RawMessage(listAgentsInputSchema) }
func (t *ListAgentsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(listAgentsOutputSchema)
}

type agentResult struct {
	ID            string  `json:"id"`
	Server        string  `json:"server"`
	Hostname      string  `json:"hostname"`
	Environment   string  `json:"environment"`
	Version       string  `json:"version"`
	Status        string  `json:"status"`
	LastHeartbeat *string `json:"last_heartbeat"`
}

func (t *ListAgentsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	serverID, err := optionalUUID(args, "server_id")
	if err != nil {
		return nil, err
	}
	status, err := optionalString(args, "status")
	if err != nil {
		return nil, err
	}

	agents, err := t.uc.List(ctx, inventory.ListAgentsRequest{ServerID: serverID, Status: status})
	if err != nil {
		if errors.Is(err, inventory.ErrInvalidServerID) {
			return nil, &mcp.ToolError{
				Code:       "invalid_server_id",
				Message:    "The server_id filter is not a valid UUID.",
				Suggestion: "Use list_servers to find a valid server id.",
			}
		}
		return nil, fmt.Errorf("list_agents: %w", err)
	}
	out := make([]agentResult, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentResult{
			ID:            a.ID.String(),
			Server:        a.ServerName,
			Hostname:      a.Hostname,
			Environment:   a.Environment,
			Version:       a.Version,
			Status:        a.Status,
			LastHeartbeat: formatTime(a.LastHeartbeat),
		})
	}
	return json.Marshal(map[string]any{"agents": out})
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.UTC().Format(time.RFC3339)
	return &v
}

var _ mcp.Tool = (*ListAgentsTool)(nil)
