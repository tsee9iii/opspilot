package git

import (
	"context"
	"encoding/json"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolGitBranch            = "git.branch"
	toolGitBranchVersion     = "1.0.0"
	toolGitBranchDescription = "Return information about the currently checked-out branch"
)

const toolGitBranchParameterSchema = `{
  "type": "object",
  "required": ["repository"],
  "properties": {
    "repository": {
      "type": "string",
      "description": "Absolute path to the local Git repository"
    }
  },
  "additionalProperties": false
}`

type gitBranchResult struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Detached   bool   `json:"detached"`
	Tracking   bool   `json:"tracking"`
	Upstream   string `json:"upstream"`
}

// GitBranchTool reports the currently checked-out branch of a local Git
// repository via the CLI.
type GitBranchTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewGitBranchTool() *GitBranchTool {
	return &GitBranchTool{run: agent.RunCommand}
}

func (t *GitBranchTool) Name() string {
	return ToolGitBranch
}

func (t *GitBranchTool) Version() string {
	return toolGitBranchVersion
}

func (t *GitBranchTool) Description() string {
	return toolGitBranchDescription
}

func (t *GitBranchTool) ParameterSchema() string {
	return toolGitBranchParameterSchema
}

func (t *GitBranchTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *GitBranchTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "git")
}

func (t *GitBranchTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	repository, err := parseRepositoryRequest(payload, "git.branch")
	if err != nil {
		return nil, err
	}

	if err := ensureGit(ctx, t.run, "git.branch"); err != nil {
		return nil, err
	}
	if err := ensureRepository(ctx, t.run, "git.branch", repository); err != nil {
		return nil, err
	}

	info, err := currentBranch(ctx, t.run, "git.branch", repository)
	if err != nil {
		return nil, err
	}

	return json.Marshal(gitBranchResult{
		Repository: repository,
		Branch:     info.Branch,
		Detached:   info.Detached,
		Tracking:   info.Upstream != "",
		Upstream:   info.Upstream,
	})
}
