package agent

import (
	"context"
	"errors"
)

var ErrNotImplemented = errors.New("agent: registration not implemented")

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
	// UpsertServer finds a server by hostname and environment or creates it,
	// returning the server identifier.
	UpsertServer(ctx context.Context, hostname string, environment string) (serverID string, err error)

	// CreateAgent persists a registered agent bound to a server.
	CreateAgent(ctx context.Context, agentID string, secret string, version string, serverID string) error
}

type RegisterUseCase struct {
	repo Repository
}

func NewRegisterUseCase(repo Repository) *RegisterUseCase {
	return &RegisterUseCase{repo: repo}
}

func (uc *RegisterUseCase) Register(ctx context.Context, req RegisterAgentRequest) (RegisterAgentResponse, error) {
	return RegisterAgentResponse{}, ErrNotImplemented
}
