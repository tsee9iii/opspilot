package http

import (
	"errors"
	"net/http"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
)

// AlertHandler exposes alert listing and acknowledgment to operators. Alert
// opening and resolution is owned exclusively by the in-process evaluator.
type AlertHandler struct {
	list *appalert.ListUseCase
	ack  *appalert.AcknowledgeUseCase
}

func NewAlertHandler(list *appalert.ListUseCase, ack *appalert.AcknowledgeUseCase) *AlertHandler {
	return &AlertHandler{list: list, ack: ack}
}

// List returns alerts, optionally filtered by status, severity, agent and
// server. Unfiltered listings are capped by the application layer.
func (h *AlertHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := h.list.List(r.Context(), appalert.ListRequest{
		Status:   q.Get("status"),
		Severity: q.Get("severity"),
		AgentID:  q.Get("agent_id"),
		ServerID: q.Get("server_id"),
	})
	if err != nil {
		switch {
		case errors.Is(err, appalert.ErrInvalidAgentID):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list alerts")
		}
		return
	}
	out := make([]AlertResponse, 0, len(items))
	for _, it := range items {
		out = append(out, toAlertResponse(it))
	}
	writeJSON(w, http.StatusOK, AlertListResponse{Alerts: out})
}

// Acknowledge acknowledges an open alert. The acknowledging actor is taken
// from the X-Operator-Actor header captured by ActorIdentity.
func (h *AlertHandler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	alertRow, err := h.ack.Acknowledge(r.Context(), r.PathValue("id"), OperatorActor(r))
	if err != nil {
		switch {
		case errors.Is(err, appalert.ErrInvalidAlertID):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		case errors.Is(err, appalert.ErrAlertNotFound):
			writeError(w, http.StatusNotFound, "alert_not_found", "alert not found")
		case errors.Is(err, appalert.ErrAcknowledgedByRequired):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to acknowledge alert")
		}
		return
	}
	writeJSON(w, http.StatusOK, toAlertResponse(alertRow))
}
