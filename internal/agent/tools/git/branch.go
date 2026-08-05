package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

	branchOut, err := runGit(ctx, t.run, "git.branch", "-C", repository, "branch", "--show-current")
	if err != nil {
		return nil, err
	}
	branch, err := parseBranchName(branchOut)
	if err != nil {
		return nil, fmt.Errorf("git.branch: %w", err)
	}

	result := gitBranchResult{
		Repository: repository,
		Branch:     branch,
		Detached:   branch == "",
	}

	// A non-zero exit for @{u} means no upstream is configured — that is not
	// an error, it just leaves tracking false.
	upstreamOut, _, exitCode, err := runGitRaw(ctx, t.run, "-C", repository, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return nil, fmt.Errorf("git.branch: %w", err)
	}
	if exitCode == 0 {
		upstream, err := parseBranchName(upstreamOut)
		if err != nil {
			return nil, fmt.Errorf("git.branch: %w", err)
		}
		if upstream == "" {
			return nil, errors.New("git.branch: malformed output: empty upstream")
		}
		result.Tracking = true
		result.Upstream = upstream
	}

	return json.Marshal(result)
}
