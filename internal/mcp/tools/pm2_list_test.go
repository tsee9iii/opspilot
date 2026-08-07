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

func TestPM2ListToolDefinition(t *testing.T) {
	tool := NewPM2ListTool(nil)
	assertInvestigationDefinition(t, tool, "pm2_list", "agent_id")
}

func TestPM2ListToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`[{"name":"api","status":"online","pid":4242,"cpu_percent":1.2,"memory_bytes":104857600,"uptime":3600}]`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewPM2ListTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.PM2ListTool {
		t.Fatalf("expected pm2.list dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) != 0 {
		t.Fatalf("pm2.list expects an empty payload, got %v", payload)
	}
}

func TestPM2ListToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "pm2 is not installed")
	tool := NewPM2ListTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestPM2ListToolAwaitingApproval(t *testing.T) {
	repo := investigationPendingApprovalRepo(uuid.New())
	tool := NewPM2ListTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "awaiting_approval", nil)
}

func TestPM2ListToolInvalidArgs(t *testing.T) {
	tool := NewPM2ListTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": 123})
	wantToolError(t, err, "invalid_args")
}

func TestPM2ListToolErrorMapping(t *testing.T) {
	t.Run("invalid agent id", func(t *testing.T) {
		tool := NewPM2ListTool(newDispatch(&dispatchRepo{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope"})
		wantToolError(t, err, "invalid_agent_id")
	})
	t.Run("capability not found", func(t *testing.T) {
		tool := NewPM2ListTool(newDispatchResolver(&dispatchRepo{}, errConfirmationResolver{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
		wantToolError(t, err, "capability_not_found")
	})
	t.Run("capability unavailable", func(t *testing.T) {
		tool := NewPM2ListTool(newDispatchResolver(&dispatchRepo{}, &errConfirmationResolver{err: appcommand.ErrCapabilityUnavailable}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
		wantToolError(t, err, "capability_unavailable")
	})
	t.Run("command timeout", func(t *testing.T) {
		repo := &dispatchRepo{
			createRes: createdResponse(uuid.New()),
			getRes:    pendingCommand(uuid.New()),
		}
		uc := newDispatch(repo)
		uc.PollInterval = time.Millisecond
		tool := NewPM2ListTool(uc)
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "timeout_seconds": 1})
		wantToolError(t, err, "command_timeout")
	})
}
