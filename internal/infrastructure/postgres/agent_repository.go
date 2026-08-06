package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/gen/postgresql"
	appagent "github.com/tsee9iii/opspilot/internal/application/agent"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

var _ appagent.Repository = (*AgentRepository)(nil)

type AgentRepository struct {
	pool *pgxpool.Pool
	q    *postgresql.Queries
}

func NewAgentRepository(pool *pgxpool.Pool) *AgentRepository {
	return &AgentRepository{
		pool: pool,
		q:    postgresql.New(pool),
	}
}

func (r *AgentRepository) RegisterAgent(ctx context.Context, req appagent.RegisterAgentRequest) (appagent.RegisterAgentResponse, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return appagent.RegisterAgentResponse{}, fmt.Errorf("postgres: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)

	serverID, err := qtx.UpsertServer(ctx, postgresql.UpsertServerParams{
		Hostname:    req.Hostname,
		Environment: req.Environment,
	})
	if err != nil {
		return appagent.RegisterAgentResponse{}, fmt.Errorf("postgres: upsert server: %w", err)
	}

	// The caller provides the already-hashed secret and the per-agent signing
	// key issued at registration.
	row, err := qtx.CreateAgent(ctx, postgresql.CreateAgentParams{
		ServerID:   serverID,
		Secret:     req.Secret,
		Version:    req.Version,
		SigningKey: req.SigningKey,
	})
	if err != nil {
		return appagent.RegisterAgentResponse{}, fmt.Errorf("postgres: create agent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return appagent.RegisterAgentResponse{}, fmt.Errorf("postgres: commit transaction: %w", err)
	}

	return appagent.RegisterAgentResponse{
		AgentID: row.ID.String(),
		Status:  row.Status,
	}, nil
}

func (r *AgentRepository) GetAgentByID(ctx context.Context, id uuid.UUID) (*domainagent.Agent, error) {
	row, err := r.q.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appagent.ErrAgentNotFound
		}
		return nil, fmt.Errorf("postgres: get agent: %w", err)
	}
	return mapAgent(row), nil
}

func (r *AgentRepository) UpdateLastHeartbeat(ctx context.Context, id uuid.UUID) error {
	if err := r.q.UpdateAgentLastHeartbeat(ctx, id); err != nil {
		return fmt.Errorf("postgres: update agent last heartbeat: %w", err)
	}
	return nil
}

// UnregisterAgent transitions an agent to unregistered and removes its
// capabilities in a single unit of work. Re-running it on an already
// unregistered agent is a no-op success. Command history is intentionally
// untouched.
func (r *AgentRepository) UnregisterAgent(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)

	row, err := qtx.MarkAgentUnregistered(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return appagent.ErrAgentNotFound
		}
		return fmt.Errorf("postgres: mark agent unregistered: %w", err)
	}
	if row.Status != appagent.StatusUnregistered {
		return fmt.Errorf("postgres: unexpected agent status %q after unregister", row.Status)
	}

	if err := qtx.DeleteAgentCapabilities(ctx, id); err != nil {
		return fmt.Errorf("postgres: delete agent capabilities: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit transaction: %w", err)
	}
	return nil
}

func mapAgent(row postgresql.GetAgentByIDRow) *domainagent.Agent {
	return &domainagent.Agent{
		ID:            row.ID,
		ServerID:      row.ServerID,
		Secret:        row.Secret,
		SigningKey:    row.SigningKey,
		Version:       row.Version,
		Status:        row.Status,
		LastHeartbeat: pgtypeTimePtr(row.LastHeartbeat),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
