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
	gitBranchName        = "git_branch"
	gitBranchDescription = "Inspect the currently checked-out branch of a Git repository on an agent. Investigation only: it runs the agent's bounded, read-only `git branch` command remotely and never modifies the repository."
)

const gitBranchInputSchema = `{
  "type": "object",
  "required": ["agent_id", "repository"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which the repository lives"},
    "repository": {"type": "string", "description": "Absolute path to a local Git repository on the agent (validated by the agent as a git work tree)"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const gitBranchOutputSchema = `{
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
        "branch": {"type": "string"},
        "detached": {"type": "boolean"},
        "tracking": {"type": "boolean"},
        "upstream": {"type": "string"}
      }
    },
    "error": {"type": "string"}
  }
}`

// GitBranchTool dispatches the agent's git.branch tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
type GitBranchTool struct {
	investigationTool
}

func NewGitBranchTool(dispatch *dispatch.DispatchUseCase) *GitBranchTool {
	return &GitBranchTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *GitBranchTool) Name() string        { return gitBranchName }
func (t *GitBranchTool) Description() string { return gitBranchDescription }
func (t *GitBranchTool) Category() string    { return CategoryInvestigation }
func (t *GitBranchTool) InputSchema() json.RawMessage {
	return json.RawMessage(gitBranchInputSchema)
}
func (t *GitBranchTool) OutputSchema() json.RawMessage {
	return json.RawMessage(gitBranchOutputSchema)
}

func (t *GitBranchTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
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
		Tool:    dispatch.GitBranchTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("git_branch: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent reads the current branch."), nil
}

var _ mcp.Tool = (*GitBranchTool)(nil)
