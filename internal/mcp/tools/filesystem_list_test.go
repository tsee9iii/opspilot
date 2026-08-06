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

func TestFilesystemListToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"path":"/srv/app","entries":[{"name":"logs","type":"directory"}]}`)
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             result,
		},
	}
	tool := NewFilesystemListTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "docker-compose.yml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got filesystemListOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "completed" || string(got.Listing) != string(result) {
		t.Fatalf("unexpected result: %+v", got)
	}
	if repo.created[0].Tool != dispatch.FilesystemListTool {
		t.Fatalf("expected filesystem.list dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["path"] != "docker-compose.yml" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestFilesystemListToolPassesOptionalArgs(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             []byte(`{}`),
		},
	}
	tool := NewFilesystemListTool(newDispatch(repo))

	if _, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "logs", "recursive": true, "max_depth": 3,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["recursive"] != true || payload["max_depth"] != float64(3) {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestFilesystemListToolFailed(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusFailed,
			Error:              "path does not exist: /nope",
		},
	}
	tool := NewFilesystemListTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "/nope",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got filesystemListOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "failed" || got.Error != "path does not exist: /nope" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestFilesystemListToolInvalidArgs(t *testing.T) {
	tool := NewFilesystemListTool(newDispatch(&dispatchRepo{}))
	cases := []map[string]any{
		{"path": "logs"},
		{"agent_id": uuid.New().String()},
		{"agent_id": 123, "path": "x"},
		{"agent_id": uuid.New().String(), "path": "x", "recursive": "yes"},
		{"agent_id": uuid.New().String(), "path": "x", "max_depth": "deep"},
		{"agent_id": uuid.New().String(), "path": "x", "timeout_seconds": 0},
	}
	for _, args := range cases {
		_, err := tool.Call(context.Background(), args)
		var te *mcp.ToolError
		if !errors.As(err, &te) || te.Code != "invalid_args" {
			t.Fatalf("expected invalid_args, got: %v", err)
		}
	}
}

func TestFilesystemListToolMapsInvalidAgentID(t *testing.T) {
	tool := NewFilesystemListTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "path": "x"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_agent_id" {
		t.Fatalf("expected invalid_agent_id, got: %v", err)
	}
}

func TestFilesystemListToolMapsTimeout(t *testing.T) {
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
	tool := NewFilesystemListTool(uc)

	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "path": "x", "timeout_seconds": 1,
	})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "command_timeout" {
		t.Fatalf("expected command_timeout, got: %v", err)
	}
}

func TestFilesystemListOutputSchema(t *testing.T) {
	tool := NewFilesystemListTool(nil)
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
