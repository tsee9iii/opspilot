package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domainagent "github.com/opspilot/opspilot/internal/domain/agent"
)

type RegisterAgentRequest struct {
	RegistrationToken string
	Secret            string
	Version           string
	Hostname          string
	Environment       string
}

type RegisterAgentResponse struct {
	AgentID string
	Status  string
}

// Repository defines the persistence contract required by the agent
// application layer. Its concrete implementation lives in the infrastructure
// layer and is injected at composition root.
type Repository interface {
	// RegisterAgent persists a registered agent and its server as a single
	// unit of work. The implementation owns all persistence details.
	RegisterAgent(ctx context.Context, req RegisterAgentRequest) (RegisterAgentResponse, error)

	// GetAgentByID returns an agent including its stored secret hash.
	GetAgentByID(ctx context.Context, id uuid.UUID) (*domainagent.Agent, error)

	// UpdateLastHeartbeat records the agent's latest heartbeat.
	UpdateLastHeartbeat(ctx context.Context, id uuid.UUID) error
}

type RegisterUseCase struct {
	agents  Repository
	tokens  TokenRepository
	tokenH  TokenHasher
	secretH SecretHasher
}

func NewRegisterUseCase(agents Repository, tokens TokenRepository, tokenH TokenHasher, secretH SecretHasher) *RegisterUseCase {
	return &RegisterUseCase{
		agents:  agents,
		tokens:  tokens,
		tokenH:  tokenH,
		secretH: secretH,
	}
}

func (uc *RegisterUseCase) Register(ctx context.Context, req RegisterAgentRequest) (RegisterAgentResponse, error) {
	tokenHash := uc.tokenH.Hash(req.RegistrationToken)

	token, err := uc.tokens.FindByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return RegisterAgentResponse{}, ErrTokenNotFound
		}
		return RegisterAgentResponse{}, fmt.Errorf("agent: validate registration token: %w", err)
	}

	if token.ExpiresAt.Before(time.Now()) {
		return RegisterAgentResponse{}, ErrTokenExpired
	}
	if token.RevokedAt != nil {
		return RegisterAgentResponse{}, ErrTokenRevoked
	}

	consumed, err := uc.tokens.Consume(ctx, tokenHash)
	if err != nil {
		return RegisterAgentResponse{}, fmt.Errorf("agent: consume registration token: %w", err)
	}
	if !consumed {
		return RegisterAgentResponse{}, ErrTokenUsed
	}

	secretHash, err := uc.secretH.Hash(ctx, req.Secret)
	if err != nil {
		return RegisterAgentResponse{}, fmt.Errorf("agent: hash agent secret: %w", err)
	}

	hashed := req
	hashed.Secret = secretHash
	return uc.agents.RegisterAgent(ctx, hashed)
}
