package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// GitChange is a single entry in a porcelain v1 status listing. The index and
// worktree status fields preserve git's status codes verbatim (e.g. " " for
// unchanged, "M" modified, "A" added, "D" deleted, "?" untracked).
type GitChange struct {
	Path           string `json:"path"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
}

// branchInfo is the parsed `## ...` header of
// `git status --porcelain=v1 --branch` output, or the result of the runtime
// branch/upstream lookup shared by the git.* tools.
type branchInfo struct {
	Branch   string
	Detached bool
	Ahead    int
	Behind   int
	Upstream string
}

// ensureGit verifies the git CLI is available on PATH.
func ensureGit(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool string) error {
	if ok, reason := agent.BinaryAvailable(ctx, run, "git"); !ok {
		return fmt.Errorf("%s: %s", tool, reason)
	}
	return nil
}

// ensureRepository verifies path exists and is inside a Git work tree.
func ensureRepository(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool, path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: repository does not exist: %s", tool, path)
		}
		return fmt.Errorf("%s: cannot access repository: %s: %w", tool, path, err)
	}
	stdout, err := runGit(ctx, run, tool, "-C", path, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("%s: not a git repository: %s", tool, path)
	}
	if strings.TrimSpace(stdout) != "true" {
		return fmt.Errorf("%s: not a git repository: %s", tool, path)
	}
	return nil
}

// runGitRaw runs git with the given arguments and returns stdout, stderr and
// the exit code without treating a non-zero exit as an error. It lets callers
// distinguish an expected failure (e.g. `git rev-parse @{u}` when no upstream
// exists) from a hard error.
func runGitRaw(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), args ...string) (stdout, stderr string, exitCode int, err error) {
	out, err := run(ctx, "git", args...)
	if err != nil {
		return "", "", 0, err
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return "", "", 0, err
	}
	return res.Stdout, res.Stderr, res.ExitCode, nil
}

// runGit runs git with the given arguments and returns stdout, treating a
// non-zero exit as an error.
func runGit(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool string, args ...string) (string, error) {
	stdout, stderr, exitCode, err := runGitRaw(ctx, run, args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%s: git is not installed: %w", tool, err)
		}
		return "", fmt.Errorf("%s: decode command result: %w", tool, err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("%s: git %s failed: %s", tool, strings.Join(args, " "), strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// parseRepositoryRequest extracts and validates the required repository path
// shared by the git.* tools.
func parseRepositoryRequest(payload []byte, tool string) (string, error) {
	if len(payload) == 0 {
		return "", fmt.Errorf("%s: payload is required", tool)
	}
	var req struct {
		Repository string `json:"repository"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("%s: invalid payload: %w", tool, err)
	}
	if req.Repository == "" {
		return "", fmt.Errorf("%s: repository is required", tool)
	}
	return req.Repository, nil
}

// parseBranchName trims a single branch or upstream name from git output and
// rejects output that spans more than one line.
func parseBranchName(stdout string) (string, error) {
	name := strings.TrimSpace(stdout)
	if strings.Contains(name, "\n") {
		return "", errors.New("malformed git output: unexpected newline in name")
	}
	return name, nil
}

// currentBranch resolves the checked-out branch via `git branch
// --show-current` and its upstream via `git rev-parse @{u}`. An empty branch
// means detached HEAD; a non-zero exit from the upstream lookup means no
// upstream is configured (Upstream stays empty).
func currentBranch(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool, repository string) (branchInfo, error) {
	branchOut, err := runGit(ctx, run, tool, "-C", repository, "branch", "--show-current")
	if err != nil {
		return branchInfo{}, err
	}
	branch, err := parseBranchName(branchOut)
	if err != nil {
		return branchInfo{}, fmt.Errorf("%s: %w", tool, err)
	}
	info := branchInfo{Branch: branch, Detached: branch == ""}

	upstreamOut, _, exitCode, err := runGitRaw(ctx, run, "-C", repository, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return branchInfo{}, fmt.Errorf("%s: %w", tool, err)
	}
	if exitCode == 0 {
		upstream, err := parseBranchName(upstreamOut)
		if err != nil {
			return branchInfo{}, fmt.Errorf("%s: %w", tool, err)
		}
		if upstream == "" {
			return branchInfo{}, fmt.Errorf("%s: malformed output: empty upstream", tool)
		}
		info.Upstream = upstream
	}
	return info, nil
}

// parseBranchHeader parses the first line of `git status --porcelain=v1
// --branch` output, e.g. `## main...origin/main [ahead 2, behind 1]` or
// `## HEAD (no branch, ahead 4)`.
func parseBranchHeader(header string) (branchInfo, error) {
	if !strings.HasPrefix(header, "## ") {
		return branchInfo{}, fmt.Errorf("malformed branch header: %q", header)
	}
	remainder := strings.TrimPrefix(header, "## ")
	info := branchInfo{}

	if strings.HasPrefix(remainder, "HEAD (no branch") {
		info.Detached = true
		info.Ahead = extractCount(remainder, "ahead ")
		info.Behind = extractCount(remainder, "behind ")
		return info, nil
	}

	for _, marker := range []string{" [ahead", " [behind", " [gone"} {
		if idx := strings.Index(remainder, marker); idx >= 0 {
			info.Ahead = extractCount(remainder[idx:], "ahead ")
			info.Behind = extractCount(remainder[idx:], "behind ")
			remainder = remainder[:idx]
			break
		}
	}

	if i := strings.Index(remainder, "..."); i >= 0 {
		info.Branch = remainder[:i]
		info.Upstream = remainder[i+3:]
		return info, nil
	}
	info.Branch = remainder
	return info, nil
}

// extractCount returns the integer that follows marker in s, or 0 when the
// marker is absent. The count ends at the first non-digit.
func extractCount(s, marker string) int {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return 0
	}
	rest := s[idx+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, _ := strconv.Atoi(rest[:end])
	return n
}

// parsePorcelainStatus parses the change entries of porcelain v1 status
// output, preserving file ordering. Blank lines are skipped.
func parsePorcelainStatus(stdout string) ([]GitChange, error) {
	var changes []GitChange
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		if len(line) < 4 || line[2] != ' ' {
			return nil, fmt.Errorf("malformed porcelain entry: %q", line)
		}
		index, worktree := line[0], line[1]
		if !validStatusCode(index) || !validStatusCode(worktree) {
			return nil, fmt.Errorf("malformed porcelain entry: %q", line)
		}
		changes = append(changes, GitChange{
			Path:           line[3:],
			IndexStatus:    string(index),
			WorktreeStatus: string(worktree),
		})
	}
	return changes, nil
}

// validStatusCode reports whether c is a valid porcelain v1 status code.
func validStatusCode(c byte) bool {
	switch c {
	case ' ', 'M', 'A', 'D', 'R', 'C', 'U', '?', 'T':
		return true
	}
	return false
}
