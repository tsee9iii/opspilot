package command

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type GetCommandRequest struct {
	CommandID string
}

// GetCommandResponse is the current state and final result of a command. The
// Result payload is opaque JSON exactly as stored — it is never transformed or
// deserialized into tool-specific structures.
type GetCommandResponse struct {
	ID                 uuid.UUID
	AgentID            uuid.UUID
	Status             string
	ConfirmationStatus string
	Tool               string
	Parameters         []byte
	Result             []byte
	Error              string
	CreatedAt          time.Time
	LeasedAt           *time.Time
	CompletedAt        *time.Time
	// Source is the immutable command origin ('api', 'mcp' or 'system').
	Source string
	// RequestedBy is the actor that requested the command.
	RequestedBy string
	RequestedAt time.Time
	// ApprovedBy is the authenticated operator that approved the command, set
	// exactly once at the pending -> approved transition.
	ApprovedBy   *string
	ApprovedAt   *time.Time
	ApprovalNote *string
}

type GetCommandUseCase struct {
	repo Repository
}

func NewGetCommandUseCase(repo Repository) *GetCommandUseCase {
	return &GetCommandUseCase{repo: repo}
}

// Get returns the stored state of a command without executing anything or
// recalculating its result.
func (uc *GetCommandUseCase) Get(ctx context.Context, req GetCommandRequest) (GetCommandResponse, error) {
	if _, err := uuid.Parse(req.CommandID); err != nil {
		return GetCommandResponse{}, ErrInvalidCommandID
	}
	return uc.repo.GetCommand(ctx, req)
}
