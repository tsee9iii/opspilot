package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/gen/postgresql"
	appcapability "github.com/tsee9iii/opspilot/internal/application/capability"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
)

var _ appcapability.CapabilityRepository = (*CapabilityRepository)(nil)

type CapabilityRepository struct {
	q *postgresql.Queries
}

func NewCapabilityRepository(pool *pgxpool.Pool) *CapabilityRepository {
	return &CapabilityRepository{q: postgresql.New(pool)}
}

func (r *CapabilityRepository) Upsert(ctx context.Context, agentID uuid.UUID, cap appcapability.Capability) error {
	return r.q.UpsertCapability(ctx, postgresql.UpsertCapabilityParams{
		AgentID:           agentID,
		ToolName:          cap.ToolName,
		Version:           cap.Version,
		Description:       cap.Description,
		ParameterSchema:   cap.ParameterSchema,
		Confirmation:      cap.Confirmation,
		Available:         cap.Available,
		UnavailableReason: cap.UnavailableReason,
	})
}

// ConfirmationLevel resolves a tool's confirmation level for an agent. It
// fails closed: a missing capability returns ErrCapabilityNotFound (never an
// empty level) and an advertised-but-unavailable tool returns
// ErrCapabilityUnavailable. Command creation must not proceed without a
// known, available capability.
func (r *CapabilityRepository) ConfirmationLevel(ctx context.Context, agentID uuid.UUID, toolName string) (string, error) {
	row, err := r.q.GetCapabilityByAgentTool(ctx, postgresql.GetCapabilityByAgentToolParams{
		AgentID:  agentID,
		ToolName: toolName,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", appcommand.ErrCapabilityNotFound
	}
	if err != nil {
		return "", err
	}
	if !row.Available {
		return "", appcommand.ErrCapabilityUnavailable
	}
	return row.ConfirmationLevel, nil
}
