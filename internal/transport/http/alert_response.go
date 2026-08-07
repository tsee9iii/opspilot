package http

import (
	"time"

	"github.com/google/uuid"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
)

// AlertListResponse is the alert listing envelope.
type AlertListResponse struct {
	Alerts []AlertResponse `json:"alerts"`
}

// AlertResponse is a single alert's lifecycle state.
type AlertResponse struct {
	ID             string  `json:"id"`
	AgentID        string  `json:"agent_id"`
	ServerID       string  `json:"server_id"`
	RuleType       string  `json:"rule_type"`
	Severity       string  `json:"severity"`
	Status         string  `json:"status"`
	Message        string  `json:"message"`
	FirstSeenAt    string  `json:"first_seen_at"`
	LastSeenAt     string  `json:"last_seen_at"`
	ResolvedAt     *string `json:"resolved_at,omitempty"`
	AcknowledgedAt *string `json:"acknowledged_at,omitempty"`
	AcknowledgedBy *string `json:"acknowledged_by,omitempty"`
}

func toAlertResponse(a appalert.Alert) AlertResponse {
	resp := AlertResponse{
		ID:             a.ID.String(),
		AgentID:        a.AgentID.String(),
		RuleType:       a.RuleType,
		Severity:       a.Severity,
		Status:         a.Status,
		Message:        a.Message,
		FirstSeenAt:    a.FirstSeenAt.Format(time.RFC3339),
		LastSeenAt:     a.LastSeenAt.Format(time.RFC3339),
		ResolvedAt:     formatTimePtr(a.ResolvedAt),
		AcknowledgedAt: formatTimePtr(a.AcknowledgedAt),
		AcknowledgedBy: a.AcknowledgedBy,
	}
	if a.ServerID != uuid.Nil {
		resp.ServerID = a.ServerID.String()
	}
	return resp
}
