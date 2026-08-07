package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/gen/postgresql"
	apphealth "github.com/tsee9iii/opspilot/internal/application/health"
)

var (
	_ apphealth.ReportRepository = (*HealthRepository)(nil)
	_ apphealth.ReadRepository   = (*HealthRepository)(nil)
)

// HealthRepository persists and reads agent health reports. There is at most
// one stored report per agent: each report overwrites the previous one, so
// central always holds the latest snapshot for alert evaluation.
type HealthRepository struct {
	q *postgresql.Queries
}

func NewHealthRepository(pool *pgxpool.Pool) *HealthRepository {
	return &HealthRepository{q: postgresql.New(pool)}
}

func (r *HealthRepository) UpsertHealth(ctx context.Context, req apphealth.ReportRequest) (apphealth.ReportResponse, error) {
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return apphealth.ReportResponse{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	if _, err := r.q.UpsertAgentHealth(ctx, postgresql.UpsertAgentHealthParams{
		AgentID:           agentID,
		ReportedAt:        req.ReportedAt,
		AgentVersion:      req.AgentVersion,
		Hostname:          req.Hostname,
		Environment:       req.Environment,
		Status:            req.Status,
		CpuUserPercent:    req.CPUUserPercent,
		CpuSystemPercent:  req.CPUSystemPercent,
		CpuIdlePercent:    req.CPUIdlePercent,
		MemoryUsedPercent: req.MemoryUsedPercent,
		DiskUsedPercent:   req.DiskUsedPercent,
		Snapshot:          req.Snapshot,
	}); err != nil {
		return apphealth.ReportResponse{}, fmt.Errorf("postgres: upsert agent health: %w", err)
	}
	return apphealth.ReportResponse{Status: req.Status}, nil
}

func (r *HealthRepository) ListHealth(ctx context.Context) ([]apphealth.Summary, error) {
	rows, err := r.q.ListAgentHealth(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list agent health: %w", err)
	}
	items := make([]apphealth.Summary, 0, len(rows))
	for _, row := range rows {
		items = append(items, apphealth.Summary{
			AgentID:                row.AgentID.String(),
			ServerID:               row.ServerID.String(),
			ReportedAt:             row.ReportedAt,
			AgentVersion:           row.AgentVersion,
			Hostname:               row.Hostname,
			Environment:            row.Environment,
			Status:                 row.Status,
			CPUUserPercent:         row.CpuUserPercent,
			CPUSystemPercent:       row.CpuSystemPercent,
			CPUIdlePercent:         row.CpuIdlePercent,
			MemoryUsedPercent:      row.MemoryUsedPercent,
			DiskUsedPercent:        row.DiskUsedPercent,
			Snapshot:               row.Snapshot,
			AgentStatus:            row.AgentStatus,
			LastHeartbeat:          pgtypeTimePtr(row.LastHeartbeat),
			AgentVersionRegistered: row.AgentVersionRegistered,
			ServerName:             row.ServerName,
			ServerHostname:         row.ServerHostname,
		})
	}
	return items, nil
}

func (r *HealthRepository) GetHealthByAgentID(ctx context.Context, agentID string) (apphealth.Summary, error) {
	id, err := uuid.Parse(agentID)
	if err != nil {
		return apphealth.Summary{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	row, err := r.q.GetAgentHealthByAgentID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apphealth.Summary{}, apphealth.ErrHealthNotFound
	}
	if err != nil {
		return apphealth.Summary{}, fmt.Errorf("postgres: get agent health: %w", err)
	}
	return apphealth.Summary{
		AgentID:           row.AgentID.String(),
		ReportedAt:        row.ReportedAt,
		AgentVersion:      row.AgentVersion,
		Hostname:          row.Hostname,
		Environment:       row.Environment,
		Status:            row.Status,
		CPUUserPercent:    row.CpuUserPercent,
		CPUSystemPercent:  row.CpuSystemPercent,
		CPUIdlePercent:    row.CpuIdlePercent,
		MemoryUsedPercent: row.MemoryUsedPercent,
		DiskUsedPercent:   row.DiskUsedPercent,
		Snapshot:          row.Snapshot,
	}, nil
}

// ListHealthSignals returns every active agent with the signal needed to reason
// about health, including agents that have never reported. It is a pure DB read
// over central state and never contacts agents.
func (r *HealthRepository) ListHealthSignals(ctx context.Context) ([]apphealth.Signal, error) {
	rows, err := r.q.ListAgentsForEvaluation(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list health signals: %w", err)
	}
	items := make([]apphealth.Signal, 0, len(rows))
	for _, row := range rows {
		sig := apphealth.Signal{
			AgentID:     row.ID.String(),
			ServerID:    row.ServerID.String(),
			AgentStatus: row.Status,
		}
		if row.LastHeartbeat.Valid {
			sig.LastHeartbeat = &row.LastHeartbeat.Time
		}
		if row.LastHealthAt.Valid {
			sig.LastHealthAt = &row.LastHealthAt.Time
		}
		if row.HealthStatus.Valid {
			v := row.HealthStatus.String
			sig.HealthStatus = &v
		}
		if row.DiskUsedPercent.Valid {
			v := row.DiskUsedPercent.Float64
			sig.DiskUsedPercent = &v
		}
		items = append(items, sig)
	}
	return items, nil
}
