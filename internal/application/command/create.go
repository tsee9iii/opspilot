package command

import (
	"context"
	"errors"
	"fmt"

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
	AgentID            string
	Tool               string
	Payload            []byte
	ConfirmationStatus string
}

type CreateCommandResponse struct {
	CommandID string
	Status    string
}

// ConfirmationResolver resolves a tool's confirmation level from the agent's
// registered capabilities.
type ConfirmationResolver interface {
	ConfirmationLevel(ctx context.Context, agentID uuid.UUID, toolName string) (string, error)
}

// Repository defines the persistence contract required by the command use cases.
type Repository interface {
	CreateCommand(ctx context.Context, req CreateCommandRequest) (CreateCommandResponse, error)
	LeaseNextCommand(ctx context.Context, req LeaseCommandRequest) (LeaseCommandResponse, error)
	StartCommand(ctx context.Context, req StartCommandRequest) (StartCommandResponse, error)
	CompleteCommand(ctx context.Context, req CompleteCommandRequest) (CompleteCommandResponse, error)
	FailCommand(ctx context.Context, req FailCommandRequest) (FailCommandResponse, error)
	ApproveCommand(ctx context.Context, req ApproveCommandRequest) (ApproveCommandResponse, error)
	GetCommand(ctx context.Context, req GetCommandRequest) (GetCommandResponse, error)
}

type CreateUseCase struct {
	repo    Repository
	confirm ConfirmationResolver
}

func NewCreateUseCase(repo Repository, confirm ConfirmationResolver) *CreateUseCase {
	return &CreateUseCase{repo: repo, confirm: confirm}
}

func (uc *CreateUseCase) Create(ctx context.Context, req CreateCommandRequest) (CreateCommandResponse, error) {
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return CreateCommandResponse{}, ErrInvalidAgentID
	}
	if req.Tool == "" {
		return CreateCommandResponse{}, ErrToolRequired
	}
	if len(req.Payload) == 0 {
		return CreateCommandResponse{}, ErrPayloadRequired
	}

	level, err := uc.confirm.ConfirmationLevel(ctx, agentID, req.Tool)
	if err != nil {
		return CreateCommandResponse{}, fmt.Errorf("create command: resolve confirmation: %w", err)
	}

	req.ConfirmationStatus = ConfirmationApproved
	if level == ConfirmationRequiredLevel {
		req.ConfirmationStatus = ConfirmationPending
	}
	return uc.repo.CreateCommand(ctx, req)
}
