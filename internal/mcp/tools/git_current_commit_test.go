package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
)

func TestGitCurrentCommitToolDefinition(t *testing.T) {
	tool := NewGitCurrentCommitTool(nil)
	assertInvestigationDefinition(t, tool, "git_current_commit", "agent_id", "repository")
}

func TestGitCurrentCommitToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"repository":"/srv/backend","commit":"abcdef1234567890abcdef1234567890abcdef12","short_commit":"abcdef1","author_name":"Dev","author_email":"dev@example.com","author_date":"2026-08-06T08:00:00+08:00","subject":"feat: add git tools"}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewGitCurrentCommitTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/backend",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.GitCurrentCommitTool {
		t.Fatalf("expected git.current_commit dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]string
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["repository"] != "/srv/backend" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestGitCurrentCommitToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "git.current_commit: repository has no commits: /srv/empty")
	tool := NewGitCurrentCommitTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/empty",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestGitCurrentCommitToolInvalidArgs(t *testing.T) {
	tool := NewGitCurrentCommitTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"repository": "/srv/backend"})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	wantToolError(t, err, "invalid_args")
}

func TestGitCurrentCommitToolErrorMapping(t *testing.T) {
	tool := NewGitCurrentCommitTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "repository": "/srv/backend"})
	wantToolError(t, err, "invalid_agent_id")
}
