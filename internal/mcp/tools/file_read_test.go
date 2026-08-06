package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

func TestFileReadToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"path":"/srv/app/docker-compose.yml","size_bytes":22,"encoding":"utf-8","content":"version: '3'\n"}`)
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             result,
		},
	}
	tool := NewFileReadTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "docker-compose.yml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got fileReadOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "completed" || string(got.File) != string(result) {
		t.Fatalf("unexpected result: %+v", got)
	}
	if repo.created[0].Tool != dispatch.FileReadTool {
		t.Fatalf("expected file.read dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]string
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["path"] != "docker-compose.yml" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestFileReadToolFailed(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusFailed,
			Error:              "file does not exist: /nope.conf",
		},
	}
	tool := NewFileReadTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "/nope.conf",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got fileReadOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "failed" || got.Error != "file does not exist: /nope.conf" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestFileReadToolAwaitingApproval(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationPending},
	}
	tool := NewFileReadTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "docker-compose.yml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got fileReadOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "awaiting_approval" || got.Message == "" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestFileReadToolInvalidArgs(t *testing.T) {
	tool := NewFileReadTool(newDispatch(&dispatchRepo{}))
	cases := []map[string]any{
		{"path": "docker-compose.yml"},
		{"agent_id": uuid.New().String()},
		{"agent_id": 123, "path": "x"},
		{"agent_id": uuid.New().String(), "path": "x", "timeout_seconds": 0},
		{"agent_id": uuid.New().String(), "path": "x", "timeout_seconds": 601},
	}
	for _, args := range cases {
		_, err := tool.Call(context.Background(), args)
		var te *mcp.ToolError
		if !errors.As(err, &te) || te.Code != "invalid_args" {
			t.Fatalf("expected invalid_args, got: %v", err)
		}
	}
}

func TestFileReadToolMapsInvalidAgentID(t *testing.T) {
	tool := NewFileReadTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "path": "x"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_agent_id" {
		t.Fatalf("expected invalid_agent_id, got: %v", err)
	}
}

func TestFileReadToolMapsTimeout(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusPending,
		},
	}
	uc := newDispatch(repo)
	uc.PollInterval = time.Millisecond
	tool := NewFileReadTool(uc)

	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "x", "timeout_seconds": 1,
	})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "command_timeout" {
		t.Fatalf("expected command_timeout, got: %v", err)
	}
}

func TestFileReadOutputSchema(t *testing.T) {
	tool := NewFileReadTool(nil)
	var schema struct {
		Type     string   `json:"type"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.OutputSchema(), &schema); err != nil {
		t.Fatalf("invalid output schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("unexpected output schema type: %s", schema.Type)
	}
}
