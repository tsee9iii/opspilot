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
	Secret  string
}

type UnregisterResponse struct {
	Status string
}

type UnregisterUseCase struct {
	agents  Repository
	secretH SecretHasher
}

func NewUnregisterUseCase(agents Repository, secretH SecretHasher) *UnregisterUseCase {
	return &UnregisterUseCase{
		agents:  agents,
		secretH: secretH,
	}
}

// Unregister marks an agent as unregistered. It authenticates the caller with
// the same agent_id + secret check used by heartbeat and capability sync, so
// an unregistered agent cannot unregister again with a mismatched secret.
// Calling Unregister on an already-unregistered agent is an idempotent success
// (the repository no-ops the transition and the deleted metadata stays gone).
func (uc *UnregisterUseCase) Unregister(ctx context.Context, req UnregisterRequest) (UnregisterResponse, error) {
	id, err := uuid.Parse(req.AgentID)
	if err != nil {
		return UnregisterResponse{}, ErrAgentNotFound
	}

	ag, err := uc.agents.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return UnregisterResponse{}, ErrAgentNotFound
		}
		return UnregisterResponse{}, fmt.Errorf("agent: get agent: %w", err)
	}

	ok, err := uc.secretH.Verify(ctx, ag.Secret, req.Secret)
	if err != nil {
		return UnregisterResponse{}, fmt.Errorf("agent: verify secret: %w", err)
	}
	if !ok {
		return UnregisterResponse{}, ErrAgentSecretMismatch
	}

	if err := uc.agents.UnregisterAgent(ctx, id); err != nil {
		return UnregisterResponse{}, fmt.Errorf("agent: unregister: %w", err)
	}

	return UnregisterResponse{Status: StatusUnregistered}, nil
}
