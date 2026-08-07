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
	gitStatusName        = "git_status"
	gitStatusDescription = "Inspect the working tree and branch status of a Git repository on an agent. Investigation only: it runs the agent's bounded, read-only `git status` command remotely and never modifies the repository."
)

const gitStatusInputSchema = `{
  "type": "object",
  "required": ["agent_id", "repository"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which the repository lives"},
    "repository": {"type": "string", "description": "Absolute path to a local Git repository on the agent (validated by the agent as a git work tree)"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const gitStatusOutputSchema = `{
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
        "ahead": {"type": "integer"},
        "behind": {"type": "integer"},
        "dirty": {"type": "boolean"},
        "changes": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "path": {"type": "string"},
              "index_status": {"type": "string"},
              "worktree_status": {"type": "string"}
            }
          }
        }
      }
    },
    "error": {"type": "string"}
  }
}`

// GitStatusTool dispatches the agent's git.status tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
// The repository argument is relayed to the agent's own safe input model
// (a git work tree path it validates); the MCP layer adds no path handling of
// its own.
type GitStatusTool struct {
	investigationTool
}

func NewGitStatusTool(dispatch *dispatch.DispatchUseCase) *GitStatusTool {
	return &GitStatusTool{investigationTool: investigationTool{dispatch: dispatch, defaultTimeoutSeconds: 300}}
}

func (t *GitStatusTool) Name() string        { return gitStatusName }
func (t *GitStatusTool) Description() string { return gitStatusDescription }
func (t *GitStatusTool) Category() string    { return CategoryInvestigation }
func (t *GitStatusTool) InputSchema() json.RawMessage {
	return json.RawMessage(gitStatusInputSchema)
}
func (t *GitStatusTool) OutputSchema() json.RawMessage {
	return json.RawMessage(gitStatusOutputSchema)
}

func (t *GitStatusTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
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
		Tool:    dispatch.GitStatusTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("git_status: %w", mapDispatchError(err))
	}
	return buildInvestigationResult(resp, "Awaiting operator approval before the agent reads the repository status."), nil
}

var _ mcp.Tool = (*GitStatusTool)(nil)
