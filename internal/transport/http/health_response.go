package http

import (
	"encoding/json"
	"time"

	apphealth "github.com/tsee9iii/opspilot/internal/application/health"
)

// ReportHealthRequest is the health report an agent submits. ReportedAt is the
// agent's wall-clock time at collection; central stores it as authoritative so
// the alert evaluator works on report time, not ingestion time.
type ReportHealthRequest struct {
	AgentID           string          `json:"agent_id"`
	ReportedAt        time.Time       `json:"reported_at"`
	AgentVersion      string          `json:"agent_version"`
	Hostname          string          `json:"hostname"`
	Environment       string          `json:"environment"`
	Status            string          `json:"status"`
	CPUUserPercent    float64         `json:"cpu_user_percent"`
	CPUSystemPercent  float64         `json:"cpu_system_percent"`
	CPUIdlePercent    float64         `json:"cpu_idle_percent"`
	MemoryUsedPercent float64         `json:"memory_used_percent"`
	DiskUsedPercent   float64         `json:"disk_used_percent"`
	Snapshot          json.RawMessage `json:"snapshot"`
}

// HealthReportResponse confirms a stored report.
type HealthReportResponse struct {
	Status string `json:"status"`
}

// HealthListResponse lists the latest health snapshot per agent.
type HealthListResponse struct {
	Agents []HealthSummaryResponse `json:"agents"`
}

// HealthSummaryResponse is the latest health snapshot for one agent, joined
// with its server context. Snapshot is opaque JSON returned exactly as stored.
type HealthSummaryResponse struct {
	AgentID    string `json:"agent_id"`
	ServerID   string `json:"server_id"`
	ReportedAt string `json:"reported_at"`
	Status     string `json:"status"`

	AgentVersion string `json:"agent_version"`
	Hostname     string `json:"hostname"`
	Environment  string `json:"environment"`

	CPUUserPercent    float64 `json:"cpu_user_percent"`
	CPUSystemPercent  float64 `json:"cpu_system_percent"`
	CPUIdlePercent    float64 `json:"cpu_idle_percent"`
	MemoryUsedPercent float64 `json:"memory_used_percent"`
	DiskUsedPercent   float64 `json:"disk_used_percent"`

	Snapshot json.RawMessage `json:"snapshot,omitempty"`

	AgentStatus            string  `json:"agent_status"`
	LastHeartbeat          *string `json:"last_heartbeat,omitempty"`
	AgentVersionRegistered string  `json:"agent_version_registered"`
	ServerName             string  `json:"server_name"`
	ServerHostname         string  `json:"server_hostname"`
}

func toHealthSummary(it apphealth.Summary) HealthSummaryResponse {
	return HealthSummaryResponse{
		AgentID:                it.AgentID,
		ServerID:               it.ServerID,
		ReportedAt:             it.ReportedAt.Format(time.RFC3339),
		Status:                 it.Status,
		AgentVersion:           it.AgentVersion,
		Hostname:               it.Hostname,
		Environment:            it.Environment,
		CPUUserPercent:         it.CPUUserPercent,
		CPUSystemPercent:       it.CPUSystemPercent,
		CPUIdlePercent:         it.CPUIdlePercent,
		MemoryUsedPercent:      it.MemoryUsedPercent,
		DiskUsedPercent:        it.DiskUsedPercent,
		Snapshot:               it.Snapshot,
		AgentStatus:            it.AgentStatus,
		LastHeartbeat:          formatTimePtr(it.LastHeartbeat),
		AgentVersionRegistered: it.AgentVersionRegistered,
		ServerName:             it.ServerName,
		ServerHostname:         it.ServerHostname,
	}
}
