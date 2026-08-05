package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	appcommand "github.com/opspilot/opspilot/internal/application/command"
	"github.com/opspilot/opspilot/gen/postgresql"
)

var _ appcommand.Repository = (*CommandRepository)(nil)

type CommandRepository struct {
	q *postgresql.Queries
}

func NewCommandRepository(pool *pgxpool.Pool) *CommandRepository {
	return &CommandRepository{q: postgresql.New(pool)}
}

func (r *CommandRepository) CreateCommand(ctx context.Context, req appcommand.CreateCommandRequest) (appcommand.CreateCommandResponse, error) {
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return appcommand.CreateCommandResponse{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	row, err := r.q.CreateCommand(ctx, postgresql.CreateCommandParams{
		AgentID:  agentID,
		ToolName: req.Tool,
		Payload:  req.Payload,
	})
	if err != nil {
		return appcommand.CreateCommandResponse{}, fmt.Errorf("postgres: create command: %w", err)
	}

	return appcommand.CreateCommandResponse{
		CommandID: row.ID.String(),
		Status:    row.Status,
	}, nil
}

func (r *CommandRepository) LeaseNextCommand(ctx context.Context, req appcommand.LeaseCommandRequest) (appcommand.LeaseCommandResponse, error) {
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return appcommand.LeaseCommandResponse{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	row, err := r.q.LeaseNextCommand(ctx, postgresql.LeaseNextCommandParams{
		AgentID:    agentID,
		LeaseOwner: pgtype.Text{String: agentID.String(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return appcommand.LeaseCommandResponse{}, appcommand.ErrNoPendingCommands
	}
	if err != nil {
		return appcommand.LeaseCommandResponse{}, fmt.Errorf("postgres: lease next command: %w", err)
	}

	return appcommand.LeaseCommandResponse{
		CommandID: row.ID.String(),
		Tool:      row.ToolName,
		Payload:   row.Payload,
	}, nil
}
