package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	listAlertsName        = "list_alerts"
	listAlertsDescription = "List alert lifecycle state, optionally filtered by status, severity, agent or server. Pure read of central state; alert changes are never made from here."

	getAlertName        = "get_alert"
	getAlertDescription = "Get one alert's full lifecycle state by id. Pure read of central state."
)

const listAlertsInputSchema = `{
  "type": "object",
  "properties": {
    "status": {"type": "string", "description": "Filter by status: open, acknowledged, resolved"},
    "severity": {"type": "string", "description": "Filter by severity: warning, critical"},
    "agent_id": {"type": "string", "description": "Filter by agent UUID"},
    "server_id": {"type": "string", "description": "Filter by server UUID"}
  }
}`

const getAlertInputSchema = `{
  "type": "object",
  "required": ["alert_id"],
  "properties": {
    "alert_id": {"type": "string", "description": "Alert UUID"}
  }
}`

const alertObject = `{
  "type": "object",
  "required": ["id", "agent_id", "rule_type", "severity", "status", "message", "first_seen_at", "last_seen_at"],
  "properties": {
    "id": {"type": "string"},
    "agent_id": {"type": "string"},
    "server_id": {"type": ["string", "null"]},
    "rule_type": {"type": "string"},
    "severity": {"type": "string"},
    "status": {"type": "string"},
    "message": {"type": "string"},
    "first_seen_at": {"type": "string"},
    "last_seen_at": {"type": "string"},
    "resolved_at": {"type": ["string", "null"]},
    "acknowledged_at": {"type": ["string", "null"]},
    "acknowledged_by": {"type": ["string", "null"]}
  }
}`

const listAlertsOutputSchema = `{
  "type": "object",
  "required": ["alerts"],
  "properties": {"alerts": {"type": "array", "items": ` + alertObject + `}}
}`

const getAlertOutputSchema = `{
  "type": "object",
  "required": ["alert"],
  "properties": {"alert": ` + alertObject + `}
}`

// ListAlertsTool reads alert lifecycle state. Acknowledgment is never exposed
// through the MCP; it is an authenticated operator action only.
type ListAlertsTool struct {
	uc *appalert.ListUseCase
}

func NewListAlertsTool(uc *appalert.ListUseCase) *ListAlertsTool {
	return &ListAlertsTool{uc: uc}
}

func (t *ListAlertsTool) Name() string                 { return listAlertsName }
func (t *ListAlertsTool) Description() string          { return listAlertsDescription }
func (t *ListAlertsTool) Category() string             { return CategoryInventory }
func (t *ListAlertsTool) InputSchema() json.RawMessage { return json.RawMessage(listAlertsInputSchema) }
func (t *ListAlertsTool) OutputSchema() json.RawMessage {
	return json.RawMessage(listAlertsOutputSchema)
}

func (t *ListAlertsTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	status, err := optionalString(args, "status")
	if err != nil {
		return nil, err
	}
	severity, err := optionalString(args, "severity")
	if err != nil {
		return nil, err
	}
	agentID, err := optionalString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	serverID, err := optionalString(args, "server_id")
	if err != nil {
		return nil, err
	}

	alerts, err := t.uc.List(ctx, appalert.ListRequest{
		Status:   status,
		Severity: severity,
		AgentID:  agentID,
		ServerID: serverID,
	})
	if err != nil {
		switch {
		case errors.Is(err, appalert.ErrInvalidAgentID), errors.Is(err, appalert.ErrInvalidServerID):
			return nil, &mcp.ToolError{
				Code:       "invalid_id",
				Message:    "agent_id and server_id must be valid UUIDs.",
				Suggestion: "Use list_agents and list_servers to find valid ids.",
			}
		}
		return nil, fmt.Errorf("list_alerts: %w", err)
	}
	out := make([]map[string]any, 0, len(alerts))
	for _, a := range alerts {
		out = append(out, alertResult(a))
	}
	return json.Marshal(map[string]any{"alerts": out})
}

// GetAlertTool reads one alert's lifecycle state by id.
type GetAlertTool struct {
	uc *appalert.GetUseCase
}

func NewGetAlertTool(uc *appalert.GetUseCase) *GetAlertTool {
	return &GetAlertTool{uc: uc}
}

func (t *GetAlertTool) Name() string                 { return getAlertName }
func (t *GetAlertTool) Description() string          { return getAlertDescription }
func (t *GetAlertTool) Category() string             { return CategoryInventory }
func (t *GetAlertTool) InputSchema() json.RawMessage { return json.RawMessage(getAlertInputSchema) }
func (t *GetAlertTool) OutputSchema() json.RawMessage {
	return json.RawMessage(getAlertOutputSchema)
}

func (t *GetAlertTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	alertID, err := requireString(args, "alert_id")
	if err != nil {
		return nil, err
	}
	alertRow, err := t.uc.Get(ctx, alertID)
	if err != nil {
		switch {
		case errors.Is(err, appalert.ErrAlertNotFound):
			return nil, &mcp.ToolError{
				Code:       "alert_not_found",
				Message:    "No alert exists with this id.",
				Suggestion: "Use list_alerts to find a valid alert id.",
			}
		case errors.Is(err, appalert.ErrInvalidAlertID):
			return nil, &mcp.ToolError{
				Code:       "invalid_alert_id",
				Message:    "alert_id is not a valid UUID.",
				Suggestion: "Use list_alerts to find a valid alert id.",
			}
		}
		return nil, fmt.Errorf("get_alert: %w", err)
	}
	return json.Marshal(map[string]any{"alert": alertResult(alertRow)})
}

func alertResult(a appalert.Alert) map[string]any {
	out := map[string]any{
		"id":              a.ID.String(),
		"agent_id":        a.AgentID.String(),
		"server_id":       nil,
		"rule_type":       a.RuleType,
		"severity":        a.Severity,
		"status":          a.Status,
		"message":         a.Message,
		"first_seen_at":   a.FirstSeenAt.UTC().Format(time.RFC3339),
		"last_seen_at":    a.LastSeenAt.UTC().Format(time.RFC3339),
		"resolved_at":     formatTime(a.ResolvedAt),
		"acknowledged_at": formatTime(a.AcknowledgedAt),
		"acknowledged_by": a.AcknowledgedBy,
	}
	if a.ServerID != uuid.Nil {
		out["server_id"] = a.ServerID.String()
	}
	return out
}

var _ mcp.Tool = (*ListAlertsTool)(nil)
var _ mcp.Tool = (*GetAlertTool)(nil)
