package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/gen/postgresql"
	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
)

var (
	_ appalert.Repository     = (*AlertRepository)(nil)
	_ appalert.ReadRepository = (*AlertRepository)(nil)
)

// AlertRepository persists alert lifecycle state and reads evaluation signals.
type AlertRepository struct {
	q *postgresql.Queries
}

func NewAlertRepository(pool *pgxpool.Pool) *AlertRepository {
	return &AlertRepository{q: postgresql.New(pool)}
}

func (r *AlertRepository) UpsertOpenAlert(ctx context.Context, agentID uuid.UUID, serverID uuid.UUID, ruleType, severity, message string) (bool, error) {
	row, err := r.q.UpsertOpenAlert(ctx, postgresql.UpsertOpenAlertParams{
		AgentID:  agentID,
		ServerID: pgtypeUUID(serverID),
		RuleType: ruleType,
		Severity: severity,
		Message:  message,
	})
	if err != nil {
		return false, fmt.Errorf("postgres: upsert open alert: %w", err)
	}
	return row.Created, nil
}

func (r *AlertRepository) ResolveOpenAlert(ctx context.Context, agentID uuid.UUID, ruleType string) (*appalert.Alert, error) {
	row, err := r.q.ResolveOpenAlert(ctx, postgresql.ResolveOpenAlertParams{
		AgentID:  agentID,
		RuleType: ruleType,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: resolve open alert: %w", err)
	}
	alertRow := toAlert(row.ID, row.AgentID, row.ServerID, row.RuleType, row.Severity, row.Status,
		row.Message, row.FirstSeenAt, row.LastSeenAt, row.ResolvedAt, row.AcknowledgedAt, row.AcknowledgedBy)
	return &alertRow, nil
}

func (r *AlertRepository) ListAgentsForEvaluation(ctx context.Context) ([]appalert.AgentSignal, error) {
	rows, err := r.q.ListAgentsForEvaluation(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list agents for evaluation: %w", err)
	}
	items := make([]appalert.AgentSignal, 0, len(rows))
	for _, row := range rows {
		sig := appalert.AgentSignal{
			AgentID:     row.ID,
			ServerID:    row.ServerID,
			AgentStatus: row.Status,
			Snapshot:    row.Snapshot,
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

func (r *AlertRepository) List(ctx context.Context, status, severity, agentID, serverID string, limit int) ([]appalert.Alert, error) {
	params := postgresql.ListAlertsParams{
		Status:   pgtype.Text{String: status, Valid: status != ""},
		Severity: pgtype.Text{String: severity, Valid: severity != ""},
		Limit:    int32(limit),
	}
	if agentID != "" {
		id, err := uuid.Parse(agentID)
		if err != nil {
			return nil, fmt.Errorf("postgres: parse agent id: %w", err)
		}
		params.AgentID = pgtypeUUID(id)
	}
	if serverID != "" {
		id, err := uuid.Parse(serverID)
		if err != nil {
			return nil, fmt.Errorf("postgres: parse server id: %w", err)
		}
		params.ServerID = pgtypeUUID(id)
	}

	rows, err := r.q.ListAlerts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("postgres: list alerts: %w", err)
	}
	items := make([]appalert.Alert, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAlert(row.ID, row.AgentID, row.ServerID, row.RuleType, row.Severity, row.Status,
			row.Message, row.FirstSeenAt, row.LastSeenAt, row.ResolvedAt, row.AcknowledgedAt, row.AcknowledgedBy))
	}
	return items, nil
}

func (r *AlertRepository) GetByID(ctx context.Context, id string) (appalert.Alert, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return appalert.Alert{}, appalert.ErrInvalidAlertID
	}
	row, err := r.q.GetAlertByID(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return appalert.Alert{}, appalert.ErrAlertNotFound
	}
	if err != nil {
		return appalert.Alert{}, fmt.Errorf("postgres: get alert: %w", err)
	}
	return toAlert(row.ID, row.AgentID, row.ServerID, row.RuleType, row.Severity, row.Status,
		row.Message, row.FirstSeenAt, row.LastSeenAt, row.ResolvedAt, row.AcknowledgedAt, row.AcknowledgedBy), nil
}

func (r *AlertRepository) Acknowledge(ctx context.Context, id, acknowledgedBy string) (appalert.Alert, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return appalert.Alert{}, appalert.ErrInvalidAlertID
	}
	row, err := r.q.AcknowledgeAlert(ctx, postgresql.AcknowledgeAlertParams{
		ID:             parsed,
		AcknowledgedBy: pgtype.Text{String: acknowledgedBy, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// No open alert matched the update. If the alert exists it is already
		// acknowledged or resolved; return its current state so acknowledging
		// is idempotent. A missing alert is not found.
		current, getErr := r.q.GetAlertByID(ctx, parsed)
		if errors.Is(getErr, pgx.ErrNoRows) {
			return appalert.Alert{}, appalert.ErrAlertNotFound
		}
		if getErr != nil {
			return appalert.Alert{}, fmt.Errorf("postgres: get alert: %w", getErr)
		}
		return toAlert(current.ID, current.AgentID, current.ServerID, current.RuleType, current.Severity, current.Status,
			current.Message, current.FirstSeenAt, current.LastSeenAt, current.ResolvedAt, current.AcknowledgedAt, current.AcknowledgedBy), nil
	}
	if err != nil {
		return appalert.Alert{}, fmt.Errorf("postgres: acknowledge alert: %w", err)
	}
	return toAlert(row.ID, row.AgentID, row.ServerID, row.RuleType, row.Severity, row.Status,
		row.Message, row.FirstSeenAt, row.LastSeenAt, row.ResolvedAt, row.AcknowledgedAt, row.AcknowledgedBy), nil
}

func toAlert(id, agentID uuid.UUID, serverID pgtype.UUID, ruleType, severity, status, message string, firstSeen, lastSeen time.Time, resolvedAt, ackedAt pgtype.Timestamptz, ackedBy pgtype.Text) appalert.Alert {
	a := appalert.Alert{
		ID:          id,
		AgentID:     agentID,
		RuleType:    ruleType,
		Severity:    severity,
		Status:      status,
		Message:     message,
		FirstSeenAt: firstSeen,
		LastSeenAt:  lastSeen,
	}
	if serverID.Valid {
		a.ServerID = pgtypeUUIDValue(serverID)
	}
	if resolvedAt.Valid {
		a.ResolvedAt = &resolvedAt.Time
	}
	if ackedAt.Valid {
		a.AcknowledgedAt = &ackedAt.Time
	}
	if ackedBy.Valid {
		a.AcknowledgedBy = &ackedBy.String
	}
	return a
}
