package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
)

func TestDockerListToolDefinition(t *testing.T) {
	tool := NewDockerListTool(nil)
	assertInvestigationDefinition(t, tool, "docker_list", "agent_id")
}

func TestDockerListToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"containers":[{"id":"abc123","name":"merchant-api","image":"merchant-api:latest","state":"running","status":"Up 2 minutes","ports":"0.0.0.0:8080->8080/tcp"}]}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewDockerListTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.DockerPsTool {
		t.Fatalf("expected docker.ps dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) != 0 {
		t.Fatalf("docker.ps expects an empty payload, got %v", payload)
	}
}

func TestDockerListToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "docker is not installed")
	tool := NewDockerListTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestDockerListToolAwaitingApproval(t *testing.T) {
	repo := investigationPendingApprovalRepo(uuid.New())
	tool := NewDockerListTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "awaiting_approval", nil)
}

func TestDockerListToolInvalidArgs(t *testing.T) {
	tool := NewDockerListTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{})
	wantToolError(t, err, "invalid_args")
}

func TestDockerListToolErrorMapping(t *testing.T) {
	t.Run("invalid agent id", func(t *testing.T) {
		tool := NewDockerListTool(newDispatch(&dispatchRepo{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope"})
		wantToolError(t, err, "invalid_agent_id")
	})
	t.Run("capability not found", func(t *testing.T) {
		tool := NewDockerListTool(newDispatchResolver(&dispatchRepo{}, errConfirmationResolver{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
		wantToolError(t, err, "capability_not_found")
	})
	t.Run("command timeout", func(t *testing.T) {
		repo := &dispatchRepo{createRes: createdResponse(uuid.New()), getRes: pendingCommand(uuid.New())}
		uc := newDispatch(repo)
		uc.PollInterval = time.Millisecond
		tool := NewDockerListTool(uc)
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "timeout_seconds": 1})
		wantToolError(t, err, "command_timeout")
	})
}
