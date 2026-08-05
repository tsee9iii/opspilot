package command

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNoPendingCommands = errors.New("no pending commands")

type LeaseCommandRequest struct {
	AgentID string
}

type LeaseCommandResponse struct {
	CommandID string
	Tool      string
	Payload   []byte
}

type LeaseUseCase struct {
	repo Repository
}

func NewLeaseUseCase(repo Repository) *LeaseUseCase {
	return &LeaseUseCase{repo: repo}
}

func (uc *LeaseUseCase) Lease(ctx context.Context, req LeaseCommandRequest) (LeaseCommandResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return LeaseCommandResponse{}, ErrInvalidAgentID
	}
	return uc.repo.LeaseNextCommand(ctx, req)
}
