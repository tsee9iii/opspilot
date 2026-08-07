package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
)

func TestDockerLogsToolDefinition(t *testing.T) {
	tool := NewDockerLogsTool(nil)
	assertInvestigationDefinition(t, tool, "docker_logs", "agent_id", "container")
}

func TestDockerLogsToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"container":"merchant-api","stdout":"listening on :8080\n","stderr":"","lines":100}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewDockerLogsTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "merchant-api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.DockerLogsTool {
		t.Fatalf("expected docker.logs dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["container"] != "merchant-api" || payload["lines"] != float64(100) {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestDockerLogsToolLinesArg(t *testing.T) {
	repo := investigationCompletedRepo(uuid.New(), []byte(`{"container":"db","stdout":"","stderr":"","lines":25}`))
	tool := NewDockerLogsTool(newDispatch(repo))
	if _, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "db", "lines": 25,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal(repo.created[0].Payload, &payload)
	if payload["lines"] != float64(25) {
		t.Fatalf("expected lines=25 forwarded, got %v", payload["lines"])
	}
}

func TestDockerLogsToolLinesBounds(t *testing.T) {
	tool := NewDockerLogsTool(newDispatch(&dispatchRepo{}))
	for _, lines := range []any{0, -5, 1001} {
		_, err := tool.Call(context.Background(), map[string]any{
			"agent_id": uuid.New().String(), "container": "db", "lines": lines,
		})
		wantToolError(t, err, "invalid_args")
	}
	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "db", "lines": 1.5,
	})
	wantToolError(t, err, "invalid_args")
}

func TestDockerLogsToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "docker.logs: container not found: ghost")
	tool := NewDockerLogsTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "ghost",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestDockerLogsToolAwaitingApproval(t *testing.T) {
	repo := investigationPendingApprovalRepo(uuid.New())
	tool := NewDockerLogsTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "container": "merchant-api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "awaiting_approval", nil)
}

func TestDockerLogsToolInvalidArgs(t *testing.T) {
	tool := NewDockerLogsTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"container": "db"})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	wantToolError(t, err, "invalid_args")
}

func TestDockerLogsToolErrorMapping(t *testing.T) {
	t.Run("invalid agent id", func(t *testing.T) {
		tool := NewDockerLogsTool(newDispatch(&dispatchRepo{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "container": "db"})
		wantToolError(t, err, "invalid_agent_id")
	})
	t.Run("capability unavailable", func(t *testing.T) {
		tool := NewDockerLogsTool(newDispatchResolver(&dispatchRepo{}, &errConfirmationResolver{err: appcommand.ErrCapabilityUnavailable}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "container": "db"})
		wantToolError(t, err, "capability_unavailable")
	})
	t.Run("command timeout", func(t *testing.T) {
		repo := &dispatchRepo{createRes: createdResponse(uuid.New()), getRes: pendingCommand(uuid.New())}
		uc := newDispatch(repo)
		uc.PollInterval = time.Millisecond
		tool := NewDockerLogsTool(uc)
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "container": "db", "timeout_seconds": 1})
		wantToolError(t, err, "command_timeout")
	})
}
