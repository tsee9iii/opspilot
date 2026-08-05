package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const DefaultHeartbeatInterval = 30 * time.Second

var (
	ErrAgentNotFound       = errors.New("agent not found")
	ErrAgentSecretMismatch = errors.New("agent secret mismatch")
)

type HeartbeatRequest struct {
	AgentID string
	Secret  string
}

type HeartbeatResponse struct {
	NextHeartbeat time.Duration
}

type HeartbeatUseCase struct {
	agents  Repository
	secretH SecretHasher
}

func NewHeartbeatUseCase(agents Repository, secretH SecretHasher) *HeartbeatUseCase {
	return &HeartbeatUseCase{
		agents:  agents,
		secretH: secretH,
	}
}

func (uc *HeartbeatUseCase) Heartbeat(ctx context.Context, req HeartbeatRequest) (HeartbeatResponse, error) {
	id, err := uuid.Parse(req.AgentID)
	if err != nil {
		return HeartbeatResponse{}, ErrAgentNotFound
	}

	ag, err := uc.agents.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return HeartbeatResponse{}, ErrAgentNotFound
		}
		return HeartbeatResponse{}, fmt.Errorf("agent: get agent: %w", err)
	}

	ok, err := uc.secretH.Verify(ctx, ag.Secret, req.Secret)
	if err != nil {
		return HeartbeatResponse{}, fmt.Errorf("agent: verify secret: %w", err)
	}
	if !ok {
		return HeartbeatResponse{}, ErrAgentSecretMismatch
	}

	if ag.Status == StatusUnregistered {
		return HeartbeatResponse{}, ErrAgentUnregistered
	}

	if err := uc.agents.UpdateLastHeartbeat(ctx, id); err != nil {
		return HeartbeatResponse{}, fmt.Errorf("agent: update last heartbeat: %w", err)
	}

	return HeartbeatResponse{NextHeartbeat: DefaultHeartbeatInterval}, nil
}
