package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrAgentUnregistered = errors.New("agent already unregistered")

type UnregisterRequest struct {
	AgentID string
}

type UnregisterResponse struct {
	Status string
}

type UnregisterUseCase struct {
	agents Repository
}

func NewUnregisterUseCase(agents Repository) *UnregisterUseCase {
	return &UnregisterUseCase{agents: agents}
}

// Unregister marks an agent as unregistered. Calling it on an already
// unregistered agent is an idempotent success (the repository no-ops the
// transition and the deleted metadata stays gone). Identity is established by
// the transport middleware (HMAC request signing).
func (uc *UnregisterUseCase) Unregister(ctx context.Context, req UnregisterRequest) (UnregisterResponse, error) {
	id, err := uuid.Parse(req.AgentID)
	if err != nil {
		return UnregisterResponse{}, ErrAgentNotFound
	}

	if err := uc.agents.UnregisterAgent(ctx, id); err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return UnregisterResponse{}, ErrAgentNotFound
		}
		return UnregisterResponse{}, fmt.Errorf("agent: unregister: %w", err)
	}

	return UnregisterResponse{Status: StatusUnregistered}, nil
}
