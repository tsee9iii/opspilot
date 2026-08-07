package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
)

func TestGitBranchToolDefinition(t *testing.T) {
	tool := NewGitBranchTool(nil)
	assertInvestigationDefinition(t, tool, "git_branch", "agent_id", "repository")
}

func TestGitBranchToolCompleted(t *testing.T) {
	commandID := uuid.New()
	result := []byte(`{"repository":"/srv/backend","branch":"main","detached":false,"tracking":true,"upstream":"origin/main"}`)
	repo := investigationCompletedRepo(commandID, result)
	tool := NewGitBranchTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/backend",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "completed", result)
	if repo.created[0].Tool != dispatch.GitBranchTool {
		t.Fatalf("expected git.branch dispatched, got %s", repo.created[0].Tool)
	}
	var payload map[string]string
	if err := json.Unmarshal(repo.created[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["repository"] != "/srv/backend" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestGitBranchToolFailed(t *testing.T) {
	repo := investigationFailedRepo(uuid.New(), "git.branch: not a git repository: /srv/plain")
	tool := NewGitBranchTool(newDispatch(repo))
	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "repository": "/srv/plain",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertInvestigationResult(t, out, "failed", nil)
}

func TestGitBranchToolInvalidArgs(t *testing.T) {
	tool := NewGitBranchTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"repository": "/srv/backend"})
	wantToolError(t, err, "invalid_args")
	_, err = tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	wantToolError(t, err, "invalid_args")
}

func TestGitBranchToolErrorMapping(t *testing.T) {
	tool := NewGitBranchTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope", "repository": "/srv/backend"})
	wantToolError(t, err, "invalid_agent_id")
}
