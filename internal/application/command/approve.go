package command

import (
	"context"

	"github.com/google/uuid"
)

const (
	// ConfirmationApproved marks a command as operator-approved and leasable.
	ConfirmationApproved = "approved"
	// ConfirmationPending marks a command awaiting operator confirmation.
	// Pending commands are never leased.
	ConfirmationPending = "pending"
	// ConfirmationRequiredLevel is the capability confirmation level that
	// produces pending commands at creation.
	ConfirmationRequiredLevel = "required"
)

type ApproveCommandRequest struct {
	CommandID string
	// ApprovedBy is the authenticated operator actor. It is recorded exactly
	// once at the pending -> approved transition and never overwritten.
	ApprovedBy string
	// ApprovalNote is an optional human note recorded with the approval.
	ApprovalNote string
}

type ApproveCommandResponse struct {
	CommandID string
	Status    string
	// AgentID is the target agent the approved command belongs to. It is
	// populated so the caller can wake the correct agent once the command is
	// leasable.
	AgentID uuid.UUID
}

type ApprovalUseCase struct {
	repo     Repository
	notifier Notifier
}

// NewApprovalUseCase builds the approval use case. A notifier may be passed to
// wake the target agent after a pending command is released; it is optional.
func NewApprovalUseCase(repo Repository, notifiers ...Notifier) *ApprovalUseCase {
	uc := &ApprovalUseCase{repo: repo}
	if len(notifiers) > 0 {
		uc.notifier = notifiers[0]
	}
	return uc
}

func (uc *ApprovalUseCase) Approve(ctx context.Context, req ApproveCommandRequest) (ApproveCommandResponse, error) {
	if _, err := uuid.Parse(req.CommandID); err != nil {
		return ApproveCommandResponse{}, ErrInvalidCommandID
	}
	resp, err := uc.repo.ApproveCommand(ctx, req)
	if err != nil {
		return resp, err
	}
	// Approving releases the command for leasing; wake the target agent so it
	// calls the lease endpoint immediately instead of waiting for the fallback
	// poll.
	if uc.notifier != nil && resp.AgentID != uuid.Nil {
		uc.notifier.Notify(resp.AgentID.String())
	}
	return resp, nil
}
