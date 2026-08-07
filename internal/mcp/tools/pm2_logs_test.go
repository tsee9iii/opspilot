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

func TestPM2LogsToolDefinition(t *testing.T) {
	tool := NewPM2LogsTool(nil)
	assertInvestigationDefinition(t, tool, "pm2_logs", "agent_id", "process")
}

func TestPM2LogsToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"process":"api","stdout":"GET / 200\n","stderr":"","lines":100}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewPM2LogsTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "process": "api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.PM2LogsTool {
		t.Fatalf("expected pm2.logs dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["process"] != "api" || payload["lines"] != float64(100) {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestPM2LogsToolLinesArg(t *testing.T) {
	repo := investigationCompletedRepo(uuid.New(), []byte(`{"process":"api","stdout":"","stderr":"","lines":50}`))
	tool := NewPM2LogsTool(newDispatch(repo))
	if _, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "process": "api", "lines": 50,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal(repo.created[0].Payload, &payload)
	if payload["lines"] != float64(50) {
		t.Fatalf("expected lines=50 forwarded, got %v", payload["lines"])
	}
}

func TestPM2LogsToolLinesBounds(t *testing.T) {
	tool := NewPM2LogsTool(newDispatch(&dispatchRepo{}))
	for _, lines := range []any{0, -1, 1001} {
		_, err := tool.Call(context.Background(), map[string]any{
			"agent_id": uuid.New().String(), "process": "api", "lines": lines,
		})
		wantToolError(t, err, "invalid_args")
	}
	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "process": "api", "lines": "many",
	})
	wantToolError(t, err, "invalid_args")
}

func TestPM2LogsToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "pm2.logs: process not found: ghost")
	tool := NewPM2LogsTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "process": "ghost",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestPM2LogsToolAwaitingApproval(t *testing.T) {
	repo := investigationPendingApprovalRepo(uuid.New())
	tool := NewPM2LogsTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "process": "api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "awaiting_approval", nil)
}

func TestPM2LogsToolInvalidArgs(t *testing.T) {
	tool := NewPM2LogsTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"process": "api"})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	wantToolError(t, err, "invalid_args")
}

func TestPM2LogsToolErrorMapping(t *testing.T) {
	t.Run("invalid agent id", func(t *testing.T) {
		tool := NewPM2LogsTool(newDispatch(&dispatchRepo{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "process": "api"})
		wantToolError(t, err, "invalid_agent_id")
	})
	t.Run("capability unavailable", func(t *testing.T) {
		tool := NewPM2LogsTool(newDispatchResolver(&dispatchRepo{}, &errConfirmationResolver{err: appcommand.ErrCapabilityUnavailable}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "process": "api"})
		wantToolError(t, err, "capability_unavailable")
	})
	t.Run("command timeout", func(t *testing.T) {
		repo := &dispatchRepo{createRes: createdResponse(uuid.New()), getRes: pendingCommand(uuid.New())}
		uc := newDispatch(repo)
		uc.PollInterval = time.Millisecond
		tool := NewPM2LogsTool(uc)
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "process": "api", "timeout_seconds": 1})
		wantToolError(t, err, "command_timeout")
	})
}
