package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	apphealth "github.com/tsee9iii/opspilot/internal/application/health"
)

// HealthHandler exposes agent health reporting (signed agent requests) and
// reading (operator requests).
type HealthHandler struct {
	report *apphealth.ReportUseCase
	get    *apphealth.GetUseCase
}

func NewHealthHandler(report *apphealth.ReportUseCase, get *apphealth.GetUseCase) *HealthHandler {
	return &HealthHandler{report: report, get: get}
}

// Report ingests an agent's health report. The agent is already authenticated
// by AgentAuth; the agent id in the body must match the signing identity. The
// raw request body is stored verbatim as the snapshot so nothing collected by
// the agent is lost in the typed projection.
func (h *HealthHandler) Report(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	var reqDTO ReportHealthRequest
	if err := json.Unmarshal(raw, &reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if reqDTO.AgentID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "agent_id is required")
		return
	}
	if reqDTO.Status == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}
	reportedAt := reqDTO.ReportedAt
	if reportedAt.IsZero() {
		reportedAt = time.Now()
	}

	resp, err := h.report.Report(r.Context(), apphealth.ReportRequest{
		AgentID:           reqDTO.AgentID,
		ReportedAt:        reportedAt,
		AgentVersion:      reqDTO.AgentVersion,
		Hostname:          reqDTO.Hostname,
		Environment:       reqDTO.Environment,
		Status:            reqDTO.Status,
		CPUUserPercent:    reqDTO.CPUUserPercent,
		CPUSystemPercent:  reqDTO.CPUSystemPercent,
		CPUIdlePercent:    reqDTO.CPUIdlePercent,
		MemoryUsedPercent: reqDTO.MemoryUsedPercent,
		DiskUsedPercent:   reqDTO.DiskUsedPercent,
		Snapshot:          raw,
	})
	if err != nil {
		switch {
		case errors.Is(err, apphealth.ErrInvalidAgentID):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to store health report")
		}
		return
	}
	writeJSON(w, http.StatusOK, HealthReportResponse{Status: resp.Status})
}

// List returns the latest health snapshot for every agent that has reported.
func (h *HealthHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.get.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list health")
		return
	}
	out := make([]HealthSummaryResponse, 0, len(items))
	for _, it := range items {
		out = append(out, toHealthSummary(it))
	}
	writeJSON(w, http.StatusOK, HealthListResponse{Agents: out})
}
