package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const DefaultHeartbeatInterval = 30 * time.Second

var ErrAgentNotFound = errors.New("agent not found")

type HeartbeatRequest struct {
	AgentID string
}

type HeartbeatResponse struct {
	NextHeartbeat time.Duration
}

type HeartbeatUseCase struct {
	agents Repository
}

func NewHeartbeatUseCase(agents Repository) *HeartbeatUseCase {
	return &HeartbeatUseCase{agents: agents}
}

// Heartbeat records the agent's last heartbeat. Identity is established by the
// transport middleware (HMAC request signing); this use case only performs the
// lifecycle transition and timestamp update.
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

	if ag.Status == StatusUnregistered {
		return HeartbeatResponse{}, ErrAgentUnregistered
	}

	if err := uc.agents.UpdateLastHeartbeat(ctx, id); err != nil {
		return HeartbeatResponse{}, fmt.Errorf("agent: update last heartbeat: %w", err)
	}

	return HeartbeatResponse{NextHeartbeat: DefaultHeartbeatInterval}, nil
}
