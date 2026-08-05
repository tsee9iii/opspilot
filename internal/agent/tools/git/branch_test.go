package git

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// fakeGitBranchRun dispatches on git subcommands: --version, the work-tree
// rev-parse check, branch --show-current, and the @{u} upstream lookup.
// An empty upstream makes the @{u} command fail like git does when no
// upstream is configured.
func fakeGitBranchRun(branchName, upstream string) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 3 && args[3] == "--is-inside-work-tree":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "branch":
			b, _ := json.Marshal(agent.CommandResult{Stdout: branchName, ExitCode: 0})
			return b, nil
		default: // @{u}
			if upstream == "" {
				b, _ := json.Marshal(agent.CommandResult{Stderr: "fatal: no upstream configured for branch 'x'\n", ExitCode: 128})
				return b, nil
			}
			b, _ := json.Marshal(agent.CommandResult{Stdout: upstream, ExitCode: 0})
			return b, nil
		}
	}
}

func executeBranch(t *testing.T, run func(context.Context, string, ...string) ([]byte, error), repository string) (gitBranchResult, error) {
	t.Helper()
	tool := NewGitBranchTool()
	tool.run = run
	out, err := tool.Execute(context.Background(), []byte(`{"repository":"`+repository+`"}`))
	if err != nil {
		return gitBranchResult{}, err
	}
	var res gitBranchResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res, nil
}

func TestGitBranchToolMetadata(t *testing.T) {
	tool := NewGitBranchTool()
	if tool.Name() != ToolGitBranch {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() != "1.0.0" {
		t.Fatalf("unexpected version: %s", tool.Version())
	}
	if tool.Description() == "" {
		t.Fatal("missing description")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestGitBranchParameterSchema(t *testing.T) {
	tool := NewGitBranchTool()
	var schema struct {
		Type                 string   `json:"type"`
		Required             []string `json:"required"`
		AdditionalProperties bool     `json:"additionalProperties"`
		Properties           map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("unexpected schema type: %s", schema.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "repository" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	prop, ok := schema.Properties["repository"]
	if !ok || prop.Type != "string" || prop.Description == "" {
		t.Fatalf("unexpected repository property: %+v", prop)
	}
	if schema.AdditionalProperties {
		t.Fatal("expected additionalProperties: false")
	}
}

func TestGitBranchToolAvailability(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		tool := NewGitBranchTool()
		var binary string
		tool.run = func(_ context.Context, bin string, _ ...string) ([]byte, error) {
			binary = bin
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		}
		ok, reason := tool.Availability(context.Background())
		if !ok || reason != "" {
			t.Fatalf("expected available, got ok=%v reason=%q", ok, reason)
		}
		if binary != "git" {
			t.Fatalf("expected check of git binary, got %q", binary)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		tool := NewGitBranchTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "git is not installed" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("not runnable", func(t *testing.T) {
		tool := NewGitBranchTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			b, _ := json.Marshal(agent.CommandResult{ExitCode: 1})
			return b, nil
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "git is not runnable" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})
}

func TestGitBranchWithUpstream(t *testing.T) {
	repo := t.TempDir()
	res, err := executeBranch(t, fakeGitBranchRun("main\n", "origin/main\n"), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Repository != repo {
		t.Fatalf("unexpected repository: %s", res.Repository)
	}
	if res.Branch != "main" || res.Detached {
		t.Fatalf("unexpected branch: %+v", res)
	}
	if !res.Tracking || res.Upstream != "origin/main" {
		t.Fatalf("unexpected upstream: tracking=%v upstream=%q", res.Tracking, res.Upstream)
	}
}

func TestGitBranchWithoutUpstream(t *testing.T) {
	repo := t.TempDir()
	res, err := executeBranch(t, fakeGitBranchRun("feature/login\n", ""), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Branch != "feature/login" || res.Detached {
		t.Fatalf("unexpected branch: %+v", res)
	}
	if res.Tracking || res.Upstream != "" {
		t.Fatalf("expected no upstream, got tracking=%v upstream=%q", res.Tracking, res.Upstream)
	}
}

func TestGitBranchDetachedHead(t *testing.T) {
	res, err := executeBranch(t, fakeGitBranchRun("", ""), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Detached || res.Branch != "" {
		t.Fatalf("expected detached HEAD with empty branch, got %+v", res)
	}
	if res.Tracking || res.Upstream != "" {
		t.Fatalf("expected no tracking when detached, got %+v", res)
	}
}

func TestGitBranchRepositoryNotFound(t *testing.T) {
	tool := NewGitBranchTool()
	tool.run = fakeGitBranchRun("main\n", "origin/main\n")
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"/nonexistent/repo"}`))
	if err == nil || !strings.Contains(err.Error(), "repository does not exist") {
		t.Fatalf("expected repository-not-found error, got: %v", err)
	}
}

func TestGitBranchInvalidRepository(t *testing.T) {
	tool := NewGitBranchTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "--version" {
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		}
		b, _ := json.Marshal(agent.CommandResult{Stderr: "fatal: not a git repository\n", ExitCode: 128})
		return b, nil
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("expected not-a-git-repository error, got: %v", err)
	}
}

func TestGitBranchGitNotInstalled(t *testing.T) {
	tool := NewGitBranchTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "git is not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestGitBranchMalformedOutput(t *testing.T) {
	cases := []struct {
		name     string
		branch   string
		upstream string
	}{
		{"branch with newline", "main\nextra\n", "origin/main\n"},
		{"upstream with newline", "main\n", "origin/main\njunk\n"},
		{"empty upstream", "main\n", "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := executeBranch(t, fakeGitBranchRun(tc.branch, tc.upstream), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), "malformed") {
				t.Fatalf("expected malformed output error, got: %v", err)
			}
		})
	}
}

func TestGitBranchExecutionFailure(t *testing.T) {
	tool := NewGitBranchTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 3 && args[3] == "--is-inside-work-tree":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stderr: "fatal: bad config line 1 in file .git/config\n", ExitCode: 128})
			return b, nil
		}
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected execution failure error, got: %v", err)
	}
}

func TestGitBranchParseRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"repository":""}`,
		`not json`,
	}
	for _, c := range cases {
		if _, err := parseRepositoryRequest([]byte(c), "git.branch"); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}
