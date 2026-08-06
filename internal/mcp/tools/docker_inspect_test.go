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

func TestDockerInspectToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"id":"abc123","name":"merchant-api","image":"merchant-api:latest","state":"running","status":"Up 2 minutes","restart_count":2,"health":"healthy","started_at":"2026-08-06T08:00:00Z","ports":[{"container":"8080/tcp","host":"0.0.0.0:8080"}],"mounts":[{"source":"/data","destination":"/app/data"}],"networks":["bridge"]}`)
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             result,
		},
	}
	tool := NewDockerInspectTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "merchant-api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got dockerInspectOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "completed" || string(got.Container) != string(result) {
		t.Fatalf("unexpected result: %+v", got)
	}
	if repo.created[0].Tool != dispatch.DockerInspectTool {
		t.Fatalf("expected docker.inspect dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]string
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["container"] != "merchant-api" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestDockerInspectToolFailed(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusFailed,
			Error:              "container not found: ghost",
		},
	}
	tool := NewDockerInspectTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "ghost",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got dockerInspectOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "failed" || got.Error != "container not found: ghost" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestDockerInspectToolAwaitingApproval(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationPending},
	}
	tool := NewDockerInspectTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "merchant-api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got dockerInspectOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "awaiting_approval" || got.Message == "" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestDockerInspectToolInvalidArgs(t *testing.T) {
	tool := NewDockerInspectTool(newDispatch(&dispatchRepo{}))
	cases := []map[string]any{
		{"container": "merchant-api"},
		{"agent_id": uuid.New().String()},
		{"agent_id": 123, "container": "x"},
		{"agent_id": uuid.New().String(), "container": "x", "timeout_seconds": 0},
		{"agent_id": uuid.New().String(), "container": "x", "timeout_seconds": 601},
	}
	for _, args := range cases {
		_, err := tool.Call(context.Background(), args)
		var te *mcp.ToolError
		if !errors.As(err, &te) || te.Code != "invalid_args" {
			t.Fatalf("expected invalid_args, got: %v", err)
		}
	}
}

func TestDockerInspectToolMapsInvalidAgentID(t *testing.T) {
	tool := NewDockerInspectTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "container": "x"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_agent_id" {
		t.Fatalf("expected invalid_agent_id, got: %v", err)
	}
}

func TestDockerInspectToolMapsTimeout(t *testing.T) {
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
	tool := NewDockerInspectTool(uc)

	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "x", "timeout_seconds": 1,
	})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "command_timeout" {
		t.Fatalf("expected command_timeout, got: %v", err)
	}
}

func TestDockerInspectOutputSchema(t *testing.T) {
	tool := NewDockerInspectTool(nil)
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
