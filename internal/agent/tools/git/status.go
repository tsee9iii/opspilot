package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolGitStatus            = "git.status"
	toolGitStatusVersion     = "1.0.0"
	toolGitStatusDescription = "Return the current status of a local Git repository"
)

const toolGitStatusParameterSchema = `{
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

type gitStatusRequest struct {
	Repository string `json:"repository"`
}

type gitStatusResult struct {
	Repository string      `json:"repository"`
	Branch     string      `json:"branch"`
	Detached   bool        `json:"detached"`
	Ahead      int         `json:"ahead"`
	Behind     int         `json:"behind"`
	Dirty      bool        `json:"dirty"`
	Changes    []GitChange `json:"changes"`
}

// GitStatusTool reports the state of a local Git repository via the CLI.
type GitStatusTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewGitStatusTool() *GitStatusTool {
	return &GitStatusTool{run: agent.RunCommand}
}

func (t *GitStatusTool) Name() string {
	return ToolGitStatus
}

func (t *GitStatusTool) Version() string {
	return toolGitStatusVersion
}

func (t *GitStatusTool) Description() string {
	return toolGitStatusDescription
}

func (t *GitStatusTool) ParameterSchema() string {
	return toolGitStatusParameterSchema
}

func (t *GitStatusTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *GitStatusTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "git")
}

func (t *GitStatusTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	repository, err := parseGitStatusRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := ensureGit(ctx, t.run, "git.status"); err != nil {
		return nil, err
	}
	if err := ensureRepository(ctx, t.run, "git.status", repository); err != nil {
		return nil, err
	}

	stdout, err := runGit(ctx, t.run, "git.status", "-C", repository, "status", "--porcelain=v1", "--branch")
	if err != nil {
		return nil, err
	}

	result, err := parseGitStatus(repository, stdout)
	if err != nil {
		return nil, fmt.Errorf("git.status: %w", err)
	}
	return json.Marshal(result)
}

// parseGitStatusRequest extracts and validates the repository path.
func parseGitStatusRequest(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("git.status: payload is required")
	}
	var req gitStatusRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("git.status: invalid payload: %w", err)
	}
	if req.Repository == "" {
		return "", errors.New("git.status: repository is required")
	}
	return req.Repository, nil
}

// parseGitStatus splits porcelain output into a branch header and change
// entries, assembling the structured result.
func parseGitStatus(repository, stdout string) (gitStatusResult, error) {
	result := gitStatusResult{Repository: repository}
	lines := strings.Split(stdout, "\n")

	headerIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		headerIdx = i
		break
	}
	if headerIdx < 0 {
		return result, errors.New("malformed porcelain output: missing branch header")
	}

	info, err := parseBranchHeader(lines[headerIdx])
	if err != nil {
		return result, err
	}
	result.Branch = info.Branch
	result.Detached = info.Detached
	result.Ahead = info.Ahead
	result.Behind = info.Behind

	changes, err := parsePorcelainStatus(strings.Join(lines[headerIdx+1:], "\n"))
	if err != nil {
		return result, err
	}
	result.Changes = changes
	result.Dirty = len(changes) > 0
	return result, nil
}
