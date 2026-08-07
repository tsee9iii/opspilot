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

func TestGitStatusToolDefinition(t *testing.T) {
	tool := NewGitStatusTool(nil)
	assertInvestigationDefinition(t, tool, "git_status", "agent_id", "repository")
}

func TestGitStatusToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"repository":"/srv/backend","branch":"main","detached":false,"ahead":1,"behind":0,"dirty":true,"changes":[{"path":"go.mod","index_status":" ","worktree_status":"M"}]}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewGitStatusTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/backend",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.GitStatusTool {
		t.Fatalf("expected git.status dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]string
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["repository"] != "/srv/backend" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestGitStatusToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "git.status: repository does not exist: /srv/nope")
	tool := NewGitStatusTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/nope",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestGitStatusToolAwaitingApproval(t *testing.T) {
	repo := investigationPendingApprovalRepo(uuid.New())
	tool := NewGitStatusTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/backend",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "awaiting_approval", nil)
}

func TestGitStatusToolInvalidArgs(t *testing.T) {
	tool := NewGitStatusTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"repository": "/srv/backend"})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "repository": ""})
	wantToolError(t, err, "invalid_args")
}

func TestGitStatusToolErrorMapping(t *testing.T) {
	t.Run("invalid agent id", func(t *testing.T) {
		tool := NewGitStatusTool(newDispatch(&dispatchRepo{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "repository": "/srv/backend"})
		wantToolError(t, err, "invalid_agent_id")
	})
	t.Run("capability not found", func(t *testing.T) {
		tool := NewGitStatusTool(newDispatchResolver(&dispatchRepo{}, errConfirmationResolver{}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "repository": "/srv/backend"})
		wantToolError(t, err, "capability_not_found")
	})
	t.Run("capability unavailable", func(t *testing.T) {
		tool := NewGitStatusTool(newDispatchResolver(&dispatchRepo{}, &errConfirmationResolver{err: appcommand.ErrCapabilityUnavailable}))
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "repository": "/srv/backend"})
		wantToolError(t, err, "capability_unavailable")
	})
	t.Run("command timeout", func(t *testing.T) {
		repo := &dispatchRepo{createRes: createdResponse(uuid.New()), getRes: pendingCommand(uuid.New())}
		uc := newDispatch(repo)
		uc.PollInterval = time.Millisecond
		tool := NewGitStatusTool(uc)
		_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "repository": "/srv/backend", "timeout_seconds": 1})
		wantToolError(t, err, "command_timeout")
	})
}
