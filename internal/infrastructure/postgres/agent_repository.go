package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	appagent "github.com/opspilot/opspilot/internal/application/agent"
	"github.com/opspilot/opspilot/gen/postgresql"
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

	// TODO: the secret must be hashed in the Application layer before it is
	// persisted. Hashing is not implemented yet.
	row, err := qtx.CreateAgent(ctx, postgresql.CreateAgentParams{
		ServerID: serverID,
		Secret:   req.Secret,
		Version:  req.Version,
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
