package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	listServersName        = "list_servers"
	listServersDescription = "List infrastructure servers and their online/offline agent summary."
)

const listServersInputSchema = `{"type":"object","properties":{}}`

const listServersOutputSchema = `{
  "type": "object",
  "required": ["servers"],
  "properties": {
    "servers": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "name", "hostname", "environment", "status", "agent_count", "online_agent_count"],
        "properties": {
          "id": {"type": "string"},
          "name": {"type": "string"},
          "hostname": {"type": "string"},
          "environment": {"type": "string"},
          "status": {"type": "string"},
          "agent_count": {"type": "integer"},
          "online_agent_count": {"type": "integer"}
        }
      }
    }
  }
}`

// ListServersTool adapts the inventory server projection into MCP.
type ListServersTool struct {
	uc *inventory.ListServersUseCase
}

func NewListServersTool(uc *inventory.ListServersUseCase) *ListServersTool {
	return &ListServersTool{uc: uc}
}

func (t *ListServersTool) Name() string        { return listServersName }
func (t *ListServersTool) Description() string { return listServersDescription }
func (t *ListServersTool) InputSchema() json.RawMessage {
	return json.RawMessage(listServersInputSchema)
}
func (t *ListServersTool) OutputSchema() json.RawMessage {
	return json.RawMessage(listServersOutputSchema)
}

type serverResult struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Hostname         string `json:"hostname"`
	Environment      string `json:"environment"`
	Status           string `json:"status"`
	AgentCount       int64  `json:"agent_count"`
	OnlineAgentCount int64  `json:"online_agent_count"`
}

func (t *ListServersTool) Call(ctx context.Context, _ map[string]any) (json.RawMessage, error) {
	servers, err := t.uc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_servers: %w", err)
	}
	out := make([]serverResult, 0, len(servers))
	for _, s := range servers {
		out = append(out, serverResult{
			ID:               s.ID.String(),
			Name:             s.Name,
			Hostname:         s.Hostname,
			Environment:      s.Environment,
			Status:           s.Status,
			AgentCount:       s.AgentCount,
			OnlineAgentCount: s.OnlineAgentCount,
		})
	}
	return json.Marshal(map[string]any{"servers": out})
}

var _ mcp.Tool = (*ListServersTool)(nil)
