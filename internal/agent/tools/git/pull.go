package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolGitPull            = "git.pull"
	toolGitPullVersion     = "1.0.0"
	toolGitPullDescription = "Pull the latest changes from the configured upstream branch"
)

const toolGitPullParameterSchema = `{
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

type gitPullResult struct {
	Repository string `json:"repository"`
	Updated    bool   `json:"updated"`
	Branch     string `json:"branch"`
	Upstream   string `json:"upstream"`
	Message    string `json:"message"`
}

// GitPullTool fast-forwards a local Git repository to its configured upstream
// via `git pull --ff-only`. It never merges, rebases, or forces.
type GitPullTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewGitPullTool() *GitPullTool {
	return &GitPullTool{run: agent.RunCommand}
}

func (t *GitPullTool) Name() string {
	return ToolGitPull
}

func (t *GitPullTool) Version() string {
	return toolGitPullVersion
}

func (t *GitPullTool) Description() string {
	return toolGitPullDescription
}

func (t *GitPullTool) ParameterSchema() string {
	return toolGitPullParameterSchema
}

func (t *GitPullTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationRequired
}

func (t *GitPullTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "git")
}

func (t *GitPullTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	repository, err := parseRepositoryRequest(payload, "git.pull")
	if err != nil {
		return nil, err
	}

	if err := ensureGit(ctx, t.run, "git.pull"); err != nil {
		return nil, err
	}
	if err := ensureRepository(ctx, t.run, "git.pull", repository); err != nil {
		return nil, err
	}

	info, err := currentBranch(ctx, t.run, "git.pull", repository)
	if err != nil {
		return nil, err
	}
	if info.Detached {
		return nil, fmt.Errorf("git.pull: detached HEAD: cannot pull: %s", repository)
	}
	if info.Upstream == "" {
		return nil, fmt.Errorf("git.pull: no upstream configured for branch %q: %s", info.Branch, repository)
	}

	stdout, stderr, exitCode, err := runGitRaw(ctx, t.run, "-C", repository, "pull", "--ff-only")
	if err != nil {
		return nil, fmt.Errorf("git.pull: %w", err)
	}
	if exitCode != 0 {
		if strings.Contains(stderr, "Not possible to fast-forward") {
			return nil, fmt.Errorf("git.pull: fast-forward not possible: %s", strings.TrimSpace(stderr))
		}
		if strings.Contains(stderr, "would be overwritten") {
			return nil, fmt.Errorf("git.pull: merge required: %s", strings.TrimSpace(stderr))
		}
		return nil, fmt.Errorf("git.pull: git pull --ff-only failed: %s", strings.TrimSpace(stderr))
	}

	output := stdout + "\n" + stderr
	updated := false
	message := ""
	switch {
	case strings.Contains(output, "Already up to date"):
		updated = false
		message = "Already up to date."
	case strings.Contains(output, "Fast-forward"):
		updated = true
		message = "Fast-forward completed."
	default:
		return nil, errors.New("git.pull: malformed git output: unrecognized pull result")
	}

	return json.Marshal(gitPullResult{
		Repository: repository,
		Updated:    updated,
		Branch:     info.Branch,
		Upstream:   info.Upstream,
		Message:    message,
	})
}
