package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/opspilot/opspilot/gen/postgresql"
	appcapability "github.com/opspilot/opspilot/internal/application/capability"
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
		AgentID:         agentID,
		ToolName:        cap.ToolName,
		Version:         cap.Version,
		Description:     cap.Description,
		ParameterSchema: cap.ParameterSchema,
		Confirmation:    cap.Confirmation,
	})
}
