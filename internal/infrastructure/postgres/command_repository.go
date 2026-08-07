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
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
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
		AgentID:            agentID,
		ToolName:           req.Tool,
		Payload:            req.Payload,
		ConfirmationStatus: req.ConfirmationStatus,
		Source:             req.Source,
		RequestedBy:        req.RequestedBy,
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

	// Lazy lease expiry: before claiming a new command, return any of this
	// agent's leases that outlived the TTL back to pending. No scheduler runs
	// in the background; an abandoned lease is reclaimed only when the agent
	// leases again.
	if req.LeaseTTL > 0 {
		if err := r.q.ExpireStaleLeases(ctx, postgresql.ExpireStaleLeasesParams{
			LeaseOwner: pgtype.Text{String: agentID.String(), Valid: true},
			Before:     pgtype.Timestamptz{Time: time.Now().Add(-req.LeaseTTL), Valid: true},
		}); err != nil {
			return appcommand.LeaseCommandResponse{}, fmt.Errorf("postgres: expire stale leases: %w", err)
		}
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

func (r *CommandRepository) StartCommand(ctx context.Context, req appcommand.StartCommandRequest) (appcommand.StartCommandResponse, error) {
	id, err := uuid.Parse(req.CommandID)
	if err != nil {
		return appcommand.StartCommandResponse{}, fmt.Errorf("postgres: parse command id: %w", err)
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return appcommand.StartCommandResponse{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	if err := r.assertTransition(ctx, id, agentID, appcommand.StatusLeased); err != nil {
		return appcommand.StartCommandResponse{}, err
	}

	row, err := r.q.StartCommand(ctx, postgresql.StartCommandParams{ID: id, AgentID: agentID})
	if err != nil {
		return appcommand.StartCommandResponse{}, fmt.Errorf("postgres: start command: %w", err)
	}
	return appcommand.StartCommandResponse{CommandID: row.ID.String(), Status: row.Status}, nil
}

func (r *CommandRepository) CompleteCommand(ctx context.Context, req appcommand.CompleteCommandRequest) (appcommand.CompleteCommandResponse, error) {
	id, err := uuid.Parse(req.CommandID)
	if err != nil {
		return appcommand.CompleteCommandResponse{}, fmt.Errorf("postgres: parse command id: %w", err)
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return appcommand.CompleteCommandResponse{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	if err := r.assertTransition(ctx, id, agentID, appcommand.StatusRunning); err != nil {
		return appcommand.CompleteCommandResponse{}, err
	}

	row, err := r.q.CompleteCommand(ctx, postgresql.CompleteCommandParams{
		ID:      id,
		AgentID: agentID,
		Result:  req.Result,
	})
	if err != nil {
		return appcommand.CompleteCommandResponse{}, fmt.Errorf("postgres: complete command: %w", err)
	}
	return appcommand.CompleteCommandResponse{CommandID: row.ID.String(), Status: row.Status}, nil
}

func (r *CommandRepository) FailCommand(ctx context.Context, req appcommand.FailCommandRequest) (appcommand.FailCommandResponse, error) {
	id, err := uuid.Parse(req.CommandID)
	if err != nil {
		return appcommand.FailCommandResponse{}, fmt.Errorf("postgres: parse command id: %w", err)
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return appcommand.FailCommandResponse{}, fmt.Errorf("postgres: parse agent id: %w", err)
	}

	if err := r.assertTransition(ctx, id, agentID, appcommand.StatusRunning); err != nil {
		return appcommand.FailCommandResponse{}, err
	}

	row, err := r.q.FailCommand(ctx, postgresql.FailCommandParams{
		ID:      id,
		AgentID: agentID,
		Error:   pgtype.Text{String: req.Error, Valid: true},
	})
	if err != nil {
		return appcommand.FailCommandResponse{}, fmt.Errorf("postgres: fail command: %w", err)
	}
	return appcommand.FailCommandResponse{CommandID: row.ID.String(), Status: row.Status}, nil
}

// ApproveCommand transitions a command awaiting confirmation (pending ->
// approved) and stamps the audit fields (approved_at, approved_by,
// approval_note). Approving an already-approved command is an idempotent
// success that never overwrites the original approval; approving a
// non-existent command is not found.
func (r *CommandRepository) ApproveCommand(ctx context.Context, req appcommand.ApproveCommandRequest) (appcommand.ApproveCommandResponse, error) {
	id, err := uuid.Parse(req.CommandID)
	if err != nil {
		return appcommand.ApproveCommandResponse{}, fmt.Errorf("postgres: parse command id: %w", err)
	}

	row, err := r.q.ApproveCommand(ctx, postgresql.ApproveCommandParams{
		ID:           id,
		ApprovedBy:   pgtype.Text{String: req.ApprovedBy, Valid: true},
		ApprovalNote: pgtype.Text{String: req.ApprovalNote, Valid: true},
	})
	if err == nil {
		return appcommand.ApproveCommandResponse{
			CommandID: row.ID.String(),
			Status:    appcommand.ConfirmationApproved,
			AgentID:   row.AgentID,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return appcommand.ApproveCommandResponse{}, fmt.Errorf("postgres: approve command: %w", err)
	}

	// No pending row matched: the command is either missing or already
	// approved. Distinguish the two to keep approval idempotent.
	cmd, err := r.q.GetCommandByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return appcommand.ApproveCommandResponse{}, appcommand.ErrCommandNotFound
	}
	if err != nil {
		return appcommand.ApproveCommandResponse{}, fmt.Errorf("postgres: get command: %w", err)
	}
	return appcommand.ApproveCommandResponse{
		CommandID: cmd.ID.String(),
		Status:    appcommand.ConfirmationApproved,
		AgentID:   cmd.AgentID,
	}, nil
}

// GetCommand returns a command's full current state and result. The result
// payload is passed through exactly as stored (opaque JSON, no transformation).
func (r *CommandRepository) GetCommand(ctx context.Context, req appcommand.GetCommandRequest) (appcommand.GetCommandResponse, error) {
	id, err := uuid.Parse(req.CommandID)
	if err != nil {
		return appcommand.GetCommandResponse{}, fmt.Errorf("postgres: parse command id: %w", err)
	}

	row, err := r.q.GetCommandResult(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return appcommand.GetCommandResponse{}, appcommand.ErrCommandNotFound
	}
	if err != nil {
		return appcommand.GetCommandResponse{}, fmt.Errorf("postgres: get command: %w", err)
	}

	return appcommand.GetCommandResponse{
		ID:                 row.ID,
		AgentID:            row.AgentID,
		Status:             row.Status,
		ConfirmationStatus: row.ConfirmationStatus,
		Tool:               row.ToolName,
		Parameters:         row.Payload,
		Result:             row.Result,
		Error:              row.Error.String,
		CreatedAt:          row.CreatedAt,
		LeasedAt:           pgtypeTimePtr(row.LeasedAt),
		CompletedAt:        pgtypeTimePtr(row.CompletedAt),
		Source:             row.Source,
		RequestedBy:        row.RequestedBy,
		RequestedAt:        row.RequestedAt,
		ApprovedBy:         pgtypeTextPtr(row.ApprovedBy),
		ApprovedAt:         pgtypeTimePtr(row.ApprovedAt),
		ApprovalNote:       pgtypeTextPtr(row.ApprovalNote),
	}, nil
}

// assertTransition verifies the command exists, is owned by the agent, and is
// in the expected state before a transition. The state is re-checked atomically
// by the UPDATE WHERE clause, so a concurrent transition surfaces as a no-op
// here and is reported as an invalid transition.
func (r *CommandRepository) assertTransition(ctx context.Context, id, agentID uuid.UUID, expected string) error {
	cmd, err := r.q.GetCommandByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return appcommand.ErrCommandNotFound
	}
	if err != nil {
		return fmt.Errorf("postgres: get command: %w", err)
	}
	if cmd.AgentID != agentID {
		return appcommand.ErrCommandNotOwned
	}
	if cmd.Status != expected {
		return appcommand.ErrInvalidTransition
	}
	return nil
}
