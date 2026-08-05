package command

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	StatusPending   = "pending"
	StatusLeased    = "leased"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

var (
	ErrInvalidCommandID  = errors.New("invalid command id")
	ErrCommandNotFound   = errors.New("command not found")
	ErrCommandNotOwned   = errors.New("command is not owned by agent")
	ErrInvalidTransition = errors.New("command is not in the expected state")
)

type StartCommandRequest struct {
	AgentID   string
	CommandID string
}

type StartCommandResponse struct {
	CommandID string
	Status    string
}

type CompleteCommandRequest struct {
	AgentID   string
	CommandID string
	Result    []byte
}

type CompleteCommandResponse struct {
	CommandID string
	Status    string
}

type FailCommandRequest struct {
	AgentID   string
	CommandID string
	Error     string
}

type FailCommandResponse struct {
	CommandID string
	Status    string
}

type ExecutionUseCase struct {
	repo Repository
}

func NewExecutionUseCase(repo Repository) *ExecutionUseCase {
	return &ExecutionUseCase{repo: repo}
}

func (uc *ExecutionUseCase) Start(ctx context.Context, req StartCommandRequest) (StartCommandResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return StartCommandResponse{}, ErrInvalidAgentID
	}
	if _, err := uuid.Parse(req.CommandID); err != nil {
		return StartCommandResponse{}, ErrInvalidCommandID
	}
	return uc.repo.StartCommand(ctx, req)
}

func (uc *ExecutionUseCase) Complete(ctx context.Context, req CompleteCommandRequest) (CompleteCommandResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return CompleteCommandResponse{}, ErrInvalidAgentID
	}
	if _, err := uuid.Parse(req.CommandID); err != nil {
		return CompleteCommandResponse{}, ErrInvalidCommandID
	}
	if len(req.Result) == 0 {
		return CompleteCommandResponse{}, ErrResultRequired
	}
	return uc.repo.CompleteCommand(ctx, req)
}

func (uc *ExecutionUseCase) Fail(ctx context.Context, req FailCommandRequest) (FailCommandResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return FailCommandResponse{}, ErrInvalidAgentID
	}
	if _, err := uuid.Parse(req.CommandID); err != nil {
		return FailCommandResponse{}, ErrInvalidCommandID
	}
	if req.Error == "" {
		return FailCommandResponse{}, ErrErrorRequired
	}
	return uc.repo.FailCommand(ctx, req)
}
