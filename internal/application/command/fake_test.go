package command

import (
	"context"

	"github.com/google/uuid"
)

type fakeRepo struct {
	created       []CreateCommandRequest
	approveResult ApproveCommandResponse
	approveErr    error
	getResult     GetCommandResponse
	getErr        error
}

func (r *fakeRepo) CreateCommand(_ context.Context, req CreateCommandRequest) (CreateCommandResponse, error) {
	r.created = append(r.created, req)
	return CreateCommandResponse{CommandID: "cmd-1", Status: StatusPending}, nil
}

func (r *fakeRepo) LeaseNextCommand(_ context.Context, _ LeaseCommandRequest) (LeaseCommandResponse, error) {
	return LeaseCommandResponse{}, nil
}

func (r *fakeRepo) StartCommand(_ context.Context, _ StartCommandRequest) (StartCommandResponse, error) {
	return StartCommandResponse{Status: StatusRunning}, nil
}

func (r *fakeRepo) CompleteCommand(_ context.Context, _ CompleteCommandRequest) (CompleteCommandResponse, error) {
	return CompleteCommandResponse{Status: StatusCompleted}, nil
}

func (r *fakeRepo) FailCommand(_ context.Context, _ FailCommandRequest) (FailCommandResponse, error) {
	return FailCommandResponse{Status: StatusFailed}, nil
}

func (r *fakeRepo) ApproveCommand(_ context.Context, _ ApproveCommandRequest) (ApproveCommandResponse, error) {
	return r.approveResult, r.approveErr
}

func (r *fakeRepo) GetCommand(_ context.Context, _ GetCommandRequest) (GetCommandResponse, error) {
	return r.getResult, r.getErr
}

type fakeResolver struct {
	level func(agentID uuid.UUID, toolName string) (string, error)
}

func (r *fakeResolver) ConfirmationLevel(_ context.Context, agentID uuid.UUID, toolName string) (string, error) {
	return r.level(agentID, toolName)
}
