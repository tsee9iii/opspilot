package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	gitCurrentCommitName        = "git_current_commit"
	gitCurrentCommitDescription = "Inspect the currently checked-out commit of a Git repository on an agent. Investigation only: it runs the agent's bounded, read-only `git log -1` command remotely and never modifies the repository."
)

const gitCurrentCommitInputSchema = `{
  "type": "object",
  "required": ["agent_id", "repository"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which the repository lives"},
    "repository": {"type": "string", "description": "Absolute path to a local Git repository on the agent (validated by the agent as a git work tree)"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const gitCurrentCommitOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "result": {
      "type": "object",
      "properties": {
        "repository": {"type": "string"},
        "commit": {"type": "string"},
        "short_commit": {"type": "string"},
        "author_name": {"type": "string"},
        "author_email": {"type": "string"},
        "author_date": {"type": "string"},
        "subject": {"type": "string"}
      }
    },
    "error": {"type": "string"}
  }
}`

// GitCurrentCommitTool dispatches the agent's git.current_commit tool through
// the existing command pipeline. The tool is strictly read-only (confirmation
// level none), so dispatched commands complete or fail without awaiting
// operator approval.
type GitCurrentCommitTool struct {
	investigationTool
}

func NewGitCurrentCommitTool(dispatch *dispatch.DispatchUseCase) *GitCurrentCommitTool {
	return &GitCurrentCommitTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *GitCurrentCommitTool) Name() string        { return gitCurrentCommitName }
func (t *GitCurrentCommitTool) Description() string { return gitCurrentCommitDescription }
func (t *GitCurrentCommitTool) Category() string    { return CategoryInvestigation }
func (t *GitCurrentCommitTool) InputSchema() json.RawMessage {
	return json.RawMessage(gitCurrentCommitInputSchema)
}
func (t *GitCurrentCommitTool) OutputSchema() json.RawMessage {
	return json.RawMessage(gitCurrentCommitOutputSchema)
}

func (t *GitCurrentCommitTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	repository, err := requireString(args, "repository")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := t.timeout(args)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]string{"repository": repository})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.GitCurrentCommitTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("git_current_commit: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent reads the current commit."), nil
}

var _ mcp.Tool = (*GitCurrentCommitTool)(nil)
