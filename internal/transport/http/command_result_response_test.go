package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/application/command"
)

type getCommandRepo struct {
	cmd command.GetCommandResponse
	err error
}

func (r *getCommandRepo) CreateCommand(context.Context, command.CreateCommandRequest) (command.CreateCommandResponse, error) {
	return command.CreateCommandResponse{}, nil
}
func (r *getCommandRepo) LeaseNextCommand(context.Context, command.LeaseCommandRequest) (command.LeaseCommandResponse, error) {
	return command.LeaseCommandResponse{}, nil
}
func (r *getCommandRepo) StartCommand(context.Context, command.StartCommandRequest) (command.StartCommandResponse, error) {
	return command.StartCommandResponse{}, nil
}
func (r *getCommandRepo) CompleteCommand(context.Context, command.CompleteCommandRequest) (command.CompleteCommandResponse, error) {
	return command.CompleteCommandResponse{}, nil
}
func (r *getCommandRepo) FailCommand(context.Context, command.FailCommandRequest) (command.FailCommandResponse, error) {
	return command.FailCommandResponse{}, nil
}
func (r *getCommandRepo) ApproveCommand(context.Context, command.ApproveCommandRequest) (command.ApproveCommandResponse, error) {
	return command.ApproveCommandResponse{}, nil
}
func (r *getCommandRepo) GetCommand(context.Context, command.GetCommandRequest) (command.GetCommandResponse, error) {
	return r.cmd, r.err
}

func TestCommandResultResponseContract(t *testing.T) {
	commandID := uuid.New()
	agentID := uuid.New()
	completedAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	handler := NewCommandHandler(nil, nil, nil, nil, command.NewGetCommandUseCase(&getCommandRepo{
		cmd: command.GetCommandResponse{
			ID:                 commandID,
			AgentID:            agentID,
			Status:             command.StatusCompleted,
			ConfirmationStatus: "approved",
			Tool:               "system.uptime",
			Parameters:         []byte(`{"interval":5}`),
			Result:             []byte(`{"uptime_seconds":42}`),
			CreatedAt:          time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
			CompletedAt:        &completedAt,
		},
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/"+commandID.String(), nil)
	req.SetPathValue("id", commandID.String())
	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["id"] != commandID.String() || body["status"] != "completed" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if _, ok := body["error"]; ok {
		t.Fatalf("error should be omitted when empty: %s", rec.Body.String())
	}
	if _, ok := body["leased_at"]; ok {
		t.Fatalf("leased_at should be omitted when nil: %s", rec.Body.String())
	}
	if body["completed_at"] == nil {
		t.Fatalf("completed_at should be set: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"result":{"uptime_seconds":42}`) {
		t.Fatalf("result should be passed through as raw JSON: %s", rec.Body.String())
	}
}

func TestCommandResultNotFound(t *testing.T) {
	handler := NewCommandHandler(nil, nil, nil, nil, command.NewGetCommandUseCase(&getCommandRepo{
		err: command.ErrCommandNotFound,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/"+uuid.New().String(), nil)
	req.SetPathValue("id", uuid.New().String())
	rec := httptest.NewRecorder()
	handler.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["error"]["code"] != "command_not_found" {
		t.Fatalf("expected command_not_found, got: %s", rec.Body.String())
	}
}
