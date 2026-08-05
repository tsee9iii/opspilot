package command

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrInvalidAgentID  = errors.New("invalid agent id")
	ErrToolRequired    = errors.New("tool is required")
	ErrPayloadRequired = errors.New("payload is required")
	ErrResultRequired  = errors.New("result is required")
	ErrErrorRequired   = errors.New("error is required")
)

type CreateCommandRequest struct {
	AgentID string
	Tool    string
	Payload []byte
}

type CreateCommandResponse struct {
	CommandID string
	Status    string
}

// Repository defines the persistence contract required by the command use cases.
type Repository interface {
	CreateCommand(ctx context.Context, req CreateCommandRequest) (CreateCommandResponse, error)
	LeaseNextCommand(ctx context.Context, req LeaseCommandRequest) (LeaseCommandResponse, error)
	StartCommand(ctx context.Context, req StartCommandRequest) (StartCommandResponse, error)
	CompleteCommand(ctx context.Context, req CompleteCommandRequest) (CompleteCommandResponse, error)
	FailCommand(ctx context.Context, req FailCommandRequest) (FailCommandResponse, error)
}

type CreateUseCase struct {
	repo Repository
}

func NewCreateUseCase(repo Repository) *CreateUseCase {
	return &CreateUseCase{repo: repo}
}

func (uc *CreateUseCase) Create(ctx context.Context, req CreateCommandRequest) (CreateCommandResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return CreateCommandResponse{}, ErrInvalidAgentID
	}
	if req.Tool == "" {
		return CreateCommandResponse{}, ErrToolRequired
	}
	if len(req.Payload) == 0 {
		return CreateCommandResponse{}, ErrPayloadRequired
	}
	return uc.repo.CreateCommand(ctx, req)
}
