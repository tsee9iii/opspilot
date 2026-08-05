package agent

import (
	"context"
)

type RegisterAgentRequest struct {
	Secret      string
	Version     string
	Hostname    string
	Environment string
}

type RegisterAgentResponse struct {
	AgentID string
	Status  string
}

// Repository defines the persistence contract required by the Register use
// case. Its concrete implementation lives in the infrastructure layer and is
// injected at composition root.
type Repository interface {
	// RegisterAgent persists a registered agent and its server as a single
	// unit of work. The implementation owns all persistence details.
	RegisterAgent(ctx context.Context, req RegisterAgentRequest) (RegisterAgentResponse, error)
}

type RegisterUseCase struct {
	repo Repository
}

func NewRegisterUseCase(repo Repository) *RegisterUseCase {
	return &RegisterUseCase{repo: repo}
}

func (uc *RegisterUseCase) Register(ctx context.Context, req RegisterAgentRequest) (RegisterAgentResponse, error) {
	return uc.repo.RegisterAgent(ctx, req)
}
