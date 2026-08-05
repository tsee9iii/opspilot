package capability

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	appagent "github.com/opspilot/opspilot/internal/application/agent"
	domainagent "github.com/opspilot/opspilot/internal/domain/agent"
)

var (
	ErrInvalidAgentID       = errors.New("invalid agent id")
	ErrCapabilitiesRequired = errors.New("capabilities are required")
)

type Capability struct {
	ToolName        string
	Version         string
	Description     string
	ParameterSchema []byte
}

type SyncRequest struct {
	AgentID      string
	Secret       string
	Capabilities []Capability
}

type SyncResponse struct {
	Count int
}

// AgentRepository is the subset of the agent persistence contract needed to
// authenticate a capability sync.
type AgentRepository interface {
	GetAgentByID(ctx context.Context, id uuid.UUID) (*domainagent.Agent, error)
}

type CapabilityRepository interface {
	Upsert(ctx context.Context, agentID uuid.UUID, cap Capability) error
}

type SyncUseCase struct {
	agents       AgentRepository
	capabilities CapabilityRepository
	secretH      appagent.SecretHasher
}

func NewSyncUseCase(agents AgentRepository, capabilities CapabilityRepository, secretH appagent.SecretHasher) *SyncUseCase {
	return &SyncUseCase{
		agents:       agents,
		capabilities: capabilities,
		secretH:      secretH,
	}
}

func (uc *SyncUseCase) Sync(ctx context.Context, req SyncRequest) (SyncResponse, error) {
	id, err := uuid.Parse(req.AgentID)
	if err != nil {
		return SyncResponse{}, ErrInvalidAgentID
	}
	if len(req.Capabilities) == 0 {
		return SyncResponse{}, ErrCapabilitiesRequired
	}

	ag, err := uc.agents.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, appagent.ErrAgentNotFound) {
			return SyncResponse{}, appagent.ErrAgentNotFound
		}
		return SyncResponse{}, fmt.Errorf("capability: get agent: %w", err)
	}

	ok, err := uc.secretH.Verify(ctx, ag.Secret, req.Secret)
	if err != nil {
		return SyncResponse{}, fmt.Errorf("capability: verify secret: %w", err)
	}
	if !ok {
		return SyncResponse{}, appagent.ErrAgentSecretMismatch
	}

	count := 0
	for _, cap := range req.Capabilities {
		if err := uc.capabilities.Upsert(ctx, id, cap); err != nil {
			return SyncResponse{}, fmt.Errorf("capability: upsert %s: %w", cap.ToolName, err)
		}
		count++
	}
	return SyncResponse{Count: count}, nil
}
