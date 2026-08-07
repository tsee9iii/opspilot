package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tsee9iii/opspilot/internal/application/health"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	getAgentHealthName        = "get_agent_health"
	getAgentHealthDescription = "Get the latest health report for one agent (CPU, memory, disk, project health). Pure read of central state; never contacts the agent."

	listAgentHealthName        = "list_agent_health"
	listAgentHealthDescription = "List the latest health report for every agent that has reported. Pure read of central state; never contacts the agent."

	listUnhealthyAgentsName        = "list_unhealthy_agents"
	listUnhealthyAgentsDescription = "List agents currently considered unhealthy: offline, never reported, or reporting a non-ok status. Pure read of central state; never contacts the agent."
)

const getAgentHealthInputSchema = `{
  "type": "object",
  "required": ["agent_id"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID"}
  }
}`

const listAgentHealthInputSchema = `{
  "type": "object"
}`

const listUnhealthyAgentsInputSchema = `{
  "type": "object"
}`

const healthSummaryObject = `{
  "type": "object",
  "required": ["agent_id", "server_id", "reported_at", "status"],
  "properties": {
    "agent_id": {"type": "string"},
    "server_id": {"type": "string"},
    "reported_at": {"type": "string"},
    "status": {"type": "string"},
    "agent_version": {"type": "string"},
    "hostname": {"type": "string"},
    "environment": {"type": "string"},
    "cpu_user_percent": {"type": "number"},
    "cpu_system_percent": {"type": "number"},
    "cpu_idle_percent": {"type": "number"},
    "memory_used_percent": {"type": "number"},
    "disk_used_percent": {"type": "number"}
  }
}`

const getAgentHealthOutputSchema = `{
  "type": "object",
  "required": ["health"],
  "properties": {"health": ` + healthSummaryObject + `}
}`

const listAgentHealthOutputSchema = `{
  "type": "object",
  "required": ["agents"],
  "properties": {"agents": {"type": "array", "items": ` + healthSummaryObject + `}}
}`

// GetAgentHealthTool reads the latest health report for one agent.
type GetAgentHealthTool struct {
	uc *health.GetUseCase
}

func NewGetAgentHealthTool(uc *health.GetUseCase) *GetAgentHealthTool {
	return &GetAgentHealthTool{uc: uc}
}

func (t *GetAgentHealthTool) Name() string        { return getAgentHealthName }
func (t *GetAgentHealthTool) Description() string { return getAgentHealthDescription }
func (t *GetAgentHealthTool) Category() string    { return CategoryInventory }
func (t *GetAgentHealthTool) InputSchema() json.RawMessage {
	return json.RawMessage(getAgentHealthInputSchema)
}
func (t *GetAgentHealthTool) OutputSchema() json.RawMessage {
	return json.RawMessage(getAgentHealthOutputSchema)
}

func (t *GetAgentHealthTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	summary, err := t.uc.Get(ctx, agentID)
	if err != nil {
		switch {
		case errors.Is(err, health.ErrHealthNotFound):
			return nil, &mcp.ToolError{
				Code:       "agent_health_not_found",
				Message:    "No health report exists for this agent.",
				Suggestion: "The agent may never have reported. Check the agent is online with list_agents.",
			}
		case errors.Is(err, health.ErrInvalidAgentID):
			return nil, &mcp.ToolError{
				Code:       "invalid_agent_id",
				Message:    "agent_id is not a valid UUID.",
				Suggestion: "Use list_agents to find a valid agent id.",
			}
		}
		return nil, fmt.Errorf("get_agent_health: %w", err)
	}
	return json.Marshal(map[string]any{"health": healthSummaryResult(summary)})
}

// ListAgentHealthTool lists the latest health report for every agent that has
// reported.
type ListAgentHealthTool struct {
	uc *health.GetUseCase
}

func NewListAgentHealthTool(uc *health.GetUseCase) *ListAgentHealthTool {
	return &ListAgentHealthTool{uc: uc}
}

func (t *ListAgentHealthTool) Name() string        { return listAgentHealthName }
func (t *ListAgentHealthTool) Description() string { return listAgentHealthDescription }
func (t *ListAgentHealthTool) Category() string    { return CategoryInventory }
func (t *ListAgentHealthTool) InputSchema() json.RawMessage {
	return json.RawMessage(listAgentHealthInputSchema)
}
func (t *ListAgentHealthTool) OutputSchema() json.RawMessage {
	return json.RawMessage(listAgentHealthOutputSchema)
}

func (t *ListAgentHealthTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	items, err := t.uc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_agent_health: %w", err)
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, healthSummaryResult(it))
	}
	return json.Marshal(map[string]any{"agents": out})
}

// ListUnhealthyAgentsTool lists agents currently considered unhealthy. It is a
// pure read over central state.
type ListUnhealthyAgentsTool struct {
	uc *health.GetUseCase
}

func NewListUnhealthyAgentsTool(uc *health.GetUseCase) *ListUnhealthyAgentsTool {
	return &ListUnhealthyAgentsTool{uc: uc}
}

func (t *ListUnhealthyAgentsTool) Name() string        { return listUnhealthyAgentsName }
func (t *ListUnhealthyAgentsTool) Description() string { return listUnhealthyAgentsDescription }
func (t *ListUnhealthyAgentsTool) Category() string    { return CategoryInventory }
func (t *ListUnhealthyAgentsTool) InputSchema() json.RawMessage {
	return json.RawMessage(listUnhealthyAgentsInputSchema)
}
func (t *ListUnhealthyAgentsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(`{
	  "type": "object",
	  "required": ["agents"],
	  "properties": {
	    "agents": {
	      "type": "array",
	      "items": {
	        "type": "object",
	        "required": ["agent_id", "server_id", "agent_status"],
	        "properties": {
	          "agent_id": {"type": "string"},
	          "server_id": {"type": "string"},
	          "agent_status": {"type": "string"},
	          "health_status": {"type": ["string", "null"]},
	          "last_health_at": {"type": ["string", "null"]},
	          "last_heartbeat": {"type": ["string", "null"]}
	        }
	      }
	    }
	  }
	}`)
}

func (t *ListUnhealthyAgentsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	signals, err := t.uc.Unhealthy(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_unhealthy_agents: %w", err)
	}
	out := make([]map[string]any, 0, len(signals))
	for _, s := range signals {
		item := map[string]any{
			"agent_id":       s.AgentID,
			"server_id":      s.ServerID,
			"agent_status":   s.AgentStatus,
			"health_status":  nil,
			"last_health_at": nil,
			"last_heartbeat": nil,
		}
		if s.HealthStatus != nil {
			item["health_status"] = *s.HealthStatus
		}
		if s.LastHealthAt != nil {
			item["last_health_at"] = s.LastHealthAt.UTC().Format(time.RFC3339)
		}
		if s.LastHeartbeat != nil {
			item["last_heartbeat"] = s.LastHeartbeat.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return json.Marshal(map[string]any{"agents": out})
}

func healthSummaryResult(s health.Summary) map[string]any {
	return map[string]any{
		"agent_id":            s.AgentID,
		"server_id":           s.ServerID,
		"reported_at":         s.ReportedAt.UTC().Format(time.RFC3339),
		"status":              s.Status,
		"agent_version":       s.AgentVersion,
		"hostname":            s.Hostname,
		"environment":         s.Environment,
		"cpu_user_percent":    s.CPUUserPercent,
		"cpu_system_percent":  s.CPUSystemPercent,
		"cpu_idle_percent":    s.CPUIdlePercent,
		"memory_used_percent": s.MemoryUsedPercent,
		"disk_used_percent":   s.DiskUsedPercent,
	}
}

var _ mcp.Tool = (*GetAgentHealthTool)(nil)
var _ mcp.Tool = (*ListAgentHealthTool)(nil)
var _ mcp.Tool = (*ListUnhealthyAgentsTool)(nil)
