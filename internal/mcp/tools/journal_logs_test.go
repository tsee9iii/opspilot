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

func TestJournalLogsToolDefinition(t *testing.T) {
	tool := NewJournalLogsTool(nil)
	assertInvestigationDefinition(t, tool, "journal_logs", "agent_id", "service")
}

func TestJournalLogsToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"service":"nginx","stdout":"2026-08-06T08:00:00Z nginx started\n","stderr":"","lines":100}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewJournalLogsTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "service": "nginx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.JournalLogsTool {
		t.Fatalf("expected journal.logs dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]any
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["service"] != "nginx" || payload["lines"] != float64(100) {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestJournalLogsToolLinesArg(t *testing.T) {
	repo := investigationCompletedRepo(uuid.New(), []byte(`{"service":"nginx","stdout":"","stderr":"","lines":200}`))
	tool := NewJournalLogsTool(newDispatch(repo))
	if _, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "service": "nginx", "lines": 200,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var payload map[string]any
	_ = json.Unmarshal(repo.created[0].Payload, &payload)
	if payload["lines"] != float64(200) {
		t.Fatalf("expected lines=200 forwarded, got %v", payload["lines"])
	}
}

func TestJournalLogsToolLinesBounds(t *testing.T) {
	tool := NewJournalLogsTool(newDispatch(&dispatchRepo{}))
	for _, lines := range []any{0, -1, 1001} {
		_, err := tool.Call(context.Background(), map[string]any{
			"agent_id": uuid.New().String(), "service": "nginx", "lines": lines,
		})
		wantToolError(t, err, "invalid_args")
	}
	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "service": "nginx", "lines": "all",
	})
	wantToolError(t, err, "invalid_args")
}

func TestJournalLogsToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "journal.logs: service not found: ghost")
	tool := NewJournalLogsTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "service": "ghost",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestJournalLogsToolAwaitingApproval(t *testing.T) {
	repo := investigationPendingApprovalRepo(uuid.New())
	tool := NewJournalLogsTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "service": "nginx",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "awaiting_approval", nil)
}

func TestJournalLogsToolInvalidArgs(t *testing.T) {
	tool := NewJournalLogsTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"service": "nginx"})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	wantToolError(t, err, "invalid_args")
}

func TestJournalLogsToolErrorMapping(t *testing.T) {
	t.Run("invalid agent id", func(t *testing.T) {
		tool := NewJournalLogsTool(newDispatch(&dispatchRepo{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "service": "nginx"})
		wantToolError(t, err, "invalid_agent_id")
	})
	t.Run("capability not found", func(t *testing.T) {
		tool := NewJournalLogsTool(newDispatchResolver(&dispatchRepo{}, errConfirmationResolver{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "service": "nginx"})
		wantToolError(t, err, "capability_not_found")
	})
	t.Run("capability unavailable", func(t *testing.T) {
		tool := NewJournalLogsTool(newDispatchResolver(&dispatchRepo{}, &errConfirmationResolver{err: appcommand.ErrCapabilityUnavailable}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "service": "nginx"})
		wantToolError(t, err, "capability_unavailable")
	})
	t.Run("command timeout", func(t *testing.T) {
		repo := &dispatchRepo{createRes: createdResponse(uuid.New()), getRes: pendingCommand(uuid.New())}
		uc := newDispatch(repo)
		uc.PollInterval = time.Millisecond
		tool := NewJournalLogsTool(uc)
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "service": "nginx", "timeout_seconds": 1})
		wantToolError(t, err, "command_timeout")
	})
}
