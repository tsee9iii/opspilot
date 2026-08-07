// Package health contains the use cases for agent health reporting and
// retrieval. Health is a distinct signal from the heartbeat: the heartbeat is a
// lightweight liveness poll, while a health report is a full snapshot of
// runtime metrics (CPU, memory, disk) plus project-level health checks.
package health

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidAgentID is returned when an agent id cannot be parsed.
	ErrInvalidAgentID = errors.New("invalid agent id")
	// ErrHealthNotFound is returned when no health report exists for an agent.
	ErrHealthNotFound = errors.New("agent health not found")
	// ErrStatusRequired is returned when a health report omits its status.
	ErrStatusRequired = errors.New("status is required")
)

// ProjectHealth is the result of a project-level health check (HTTP probe).
type ProjectHealth struct {
	// Project is the name of the checked project.
	Project string `json:"project"`
	// Healthy reports whether the probe succeeded.
	Healthy bool `json:"healthy"`
	// URL is the probed endpoint.
	URL string `json:"url"`
	// StatusCode is the HTTP status observed by the probe, when available.
	StatusCode int `json:"status_code,omitempty"`
	// Error is a short description of a failed probe.
	Error string `json:"error,omitempty"`
}

// ReportRequest is a single health report submitted by an agent. The Snapshot
// payload is the opaque full report (already validated by the agent); the
// typed fields are the normalized projection used for alerting.
type ReportRequest struct {
	AgentID      string
	ReportedAt   time.Time
	AgentVersion string
	Hostname     string
	Environment  string
	Status       string

	CPUUserPercent    float64
	CPUSystemPercent  float64
	CPUIdlePercent    float64
	MemoryUsedPercent float64
	DiskUsedPercent   float64

	// Snapshot is the opaque raw report as received, stored for later MCP and
	// operator inspection.
	Snapshot []byte
}

// ReportResponse confirms a stored report.
type ReportResponse struct {
	Status string
}

// Summary is the latest health snapshot for one agent joined with its server
// and registration context.
type Summary struct {
	AgentID    string
	ServerID   string
	ReportedAt time.Time

	AgentVersion string
	Hostname     string
	Environment  string
	Status       string

	CPUUserPercent    float64
	CPUSystemPercent  float64
	CPUIdlePercent    float64
	MemoryUsedPercent float64
	DiskUsedPercent   float64

	Snapshot []byte

	// AgentStatus is the current registration status of the agent.
	AgentStatus string
	// LastHeartbeat is the agent's last heartbeat, nil when never seen.
	LastHeartbeat *time.Time
	// AgentVersionRegistered is the version recorded at registration.
	AgentVersionRegistered string
	// ServerName and ServerHostname are the owning server's context.
	ServerName     string
	ServerHostname string
}

// ReportRepository persists health reports.
type ReportRepository interface {
	// UpsertHealth writes the latest report for an agent, replacing any
	// previous report for the same agent.
	UpsertHealth(ctx context.Context, req ReportRequest) (ReportResponse, error)
}

// ReadRepository reads health snapshots.
type ReadRepository interface {
	// ListHealth returns the latest report for every agent that has reported,
	// newest first.
	ListHealth(ctx context.Context) ([]Summary, error)
	// GetHealthByAgentID returns the latest report for one agent.
	GetHealthByAgentID(ctx context.Context, agentID string) (Summary, error)
	// ListHealthSignals returns every active agent with the signal needed to
	// reason about health (registration status, latest report status and age),
	// including agents that have never reported.
	ListHealthSignals(ctx context.Context) ([]Signal, error)
}

// Signal is the per-agent health-reasoning input: registration status plus the
// latest report's status and age. It is the read-only counterpart to the
// alert evaluator's signal and never contacts agents.
type Signal struct {
	AgentID         string
	ServerID        string
	AgentStatus     string
	LastHeartbeat   *time.Time
	LastHealthAt    *time.Time
	HealthStatus    *string
	DiskUsedPercent *float64
}

// ReportUseCase ingests an agent's health report.
type ReportUseCase struct {
	repo ReportRepository
}

func NewReportUseCase(repo ReportRepository) *ReportUseCase {
	return &ReportUseCase{repo: repo}
}

func (uc *ReportUseCase) Report(ctx context.Context, req ReportRequest) (ReportResponse, error) {
	if req.AgentID == "" {
		return ReportResponse{}, ErrInvalidAgentID
	}
	if req.Status == "" {
		return ReportResponse{}, ErrStatusRequired
	}
	return uc.repo.UpsertHealth(ctx, req)
}

// GetUseCase reads health snapshots.
type GetUseCase struct {
	repo ReadRepository
}

func NewGetUseCase(repo ReadRepository) *GetUseCase {
	return &GetUseCase{repo: repo}
}

// List returns the latest health snapshot for every agent that has reported.
func (uc *GetUseCase) List(ctx context.Context) ([]Summary, error) {
	return uc.repo.ListHealth(ctx)
}

// Get returns the latest health snapshot for one agent.
func (uc *GetUseCase) Get(ctx context.Context, agentID string) (Summary, error) {
	if agentID == "" {
		return Summary{}, ErrInvalidAgentID
	}
	return uc.repo.GetHealthByAgentID(ctx, agentID)
}

// Unhealthy returns the agents currently considered unhealthy: offline, never
// reported, or with a report whose status is not "ok". It is a pure read over
// central state and never contacts agents.
func (uc *GetUseCase) Unhealthy(ctx context.Context) ([]Signal, error) {
	signals, err := uc.repo.ListHealthSignals(ctx)
	if err != nil {
		return nil, err
	}
	unhealthy := make([]Signal, 0, len(signals))
	for _, s := range signals {
		if s.AgentStatus != "online" || s.HealthStatus == nil || *s.HealthStatus != "ok" {
			unhealthy = append(unhealthy, s)
		}
	}
	return unhealthy, nil
}
