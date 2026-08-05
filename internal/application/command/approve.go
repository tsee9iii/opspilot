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
}

type ApproveCommandResponse struct {
	CommandID string
	Status    string
}

type ApprovalUseCase struct {
	repo Repository
}

func NewApprovalUseCase(repo Repository) *ApprovalUseCase {
	return &ApprovalUseCase{repo: repo}
}

func (uc *ApprovalUseCase) Approve(ctx context.Context, req ApproveCommandRequest) (ApproveCommandResponse, error) {
	if _, err := uuid.Parse(req.CommandID); err != nil {
		return ApproveCommandResponse{}, ErrInvalidCommandID
	}
	return uc.repo.ApproveCommand(ctx, req)
}
