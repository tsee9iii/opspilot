package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNoPendingCommands = errors.New("no pending commands")

type LeaseCommandRequest struct {
	AgentID string
	// LeaseTTL bounds how long a leased-but-not-started command may stay
	// leased before the repository returns it to pending. Zero disables
	// expiry.
	LeaseTTL time.Duration
}

type LeaseCommandResponse struct {
	CommandID string
	Tool      string
	Payload   []byte
}

type LeaseUseCase struct {
	repo     Repository
	leaseTTL time.Duration
}

func NewLeaseUseCase(repo Repository, leaseTTL time.Duration) *LeaseUseCase {
	return &LeaseUseCase{repo: repo, leaseTTL: leaseTTL}
}

func (uc *LeaseUseCase) Lease(ctx context.Context, req LeaseCommandRequest) (LeaseCommandResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return LeaseCommandResponse{}, ErrInvalidAgentID
	}
	req.LeaseTTL = uc.leaseTTL
	return uc.repo.LeaseNextCommand(ctx, req)
}
