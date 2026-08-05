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
	ToolGitCurrentCommit            = "git.current_commit"
	toolGitCurrentCommitVersion     = "1.0.0"
	toolGitCurrentCommitDescription = "Return metadata for the currently checked-out commit"
)

const toolGitCurrentCommitParameterSchema = `{
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

type gitCurrentCommitResult struct {
	Repository  string `json:"repository"`
	Commit      string `json:"commit"`
	ShortCommit string `json:"short_commit"`
	AuthorName  string `json:"author_name"`
	AuthorEmail string `json:"author_email"`
	AuthorDate  string `json:"author_date"`
	Subject     string `json:"subject"`
}

// GitCurrentCommitTool reports metadata for the currently checked-out commit
// via the Git CLI.
type GitCurrentCommitTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewGitCurrentCommitTool() *GitCurrentCommitTool {
	return &GitCurrentCommitTool{run: agent.RunCommand}
}

func (t *GitCurrentCommitTool) Name() string {
	return ToolGitCurrentCommit
}

func (t *GitCurrentCommitTool) Version() string {
	return toolGitCurrentCommitVersion
}

func (t *GitCurrentCommitTool) Description() string {
	return toolGitCurrentCommitDescription
}

func (t *GitCurrentCommitTool) ParameterSchema() string {
	return toolGitCurrentCommitParameterSchema
}

func (t *GitCurrentCommitTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *GitCurrentCommitTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "git")
}

func (t *GitCurrentCommitTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	repository, err := parseRepositoryRequest(payload, "git.current_commit")
	if err != nil {
		return nil, err
	}

	if err := ensureGit(ctx, t.run, "git.current_commit"); err != nil {
		return nil, err
	}
	if err := ensureRepository(ctx, t.run, "git.current_commit", repository); err != nil {
		return nil, err
	}

	stdout, err := runGit(ctx, t.run, "git.current_commit", "-C", repository, "log", "-1",
		"--date=iso-strict", "--pretty=format:%H%n%h%n%an%n%ae%n%ad%n%s")
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, fmt.Errorf("git.current_commit: repository has no commits: %s", repository)
		}
		return nil, err
	}

	result, err := parseGitCurrentCommit(repository, stdout)
	if err != nil {
		return nil, fmt.Errorf("git.current_commit: %w", err)
	}
	return json.Marshal(result)
}

// parseGitCurrentCommit parses the six newline-separated fields emitted by
// `git log -1 --pretty=format:%H%n%h%n%an%n%ae%n%ad%n%s`: full hash, short
// hash, author name, author email, ISO-8601 author date, and subject. An
// empty subject yields an empty trailing field.
func parseGitCurrentCommit(repository, stdout string) (gitCurrentCommitResult, error) {
	lines := strings.Split(stdout, "\n")
	if len(lines) != 6 || lines[0] == "" {
		return gitCurrentCommitResult{}, errors.New("malformed git output: expected 6 fields")
	}
	return gitCurrentCommitResult{
		Repository:  repository,
		Commit:      lines[0],
		ShortCommit: lines[1],
		AuthorName:  lines[2],
		AuthorEmail: lines[3],
		AuthorDate:  lines[4],
		Subject:     lines[5],
	}, nil
}
