package git

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// fakeGitPullRun dispatches on git subcommands: --version, the work-tree
// rev-parse check, branch --show-current, the @{u} upstream lookup, and pull.
func fakeGitPullRun(branch, upstream, pullOut, pullErr string, pullExit int) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 3 && args[3] == "--is-inside-work-tree":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "branch":
			b, _ := json.Marshal(agent.CommandResult{Stdout: branch, ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "rev-parse":
			if upstream == "" {
				b, _ := json.Marshal(agent.CommandResult{Stderr: "fatal: no upstream configured for branch 'x'\n", ExitCode: 128})
				return b, nil
			}
			b, _ := json.Marshal(agent.CommandResult{Stdout: upstream, ExitCode: 0})
			return b, nil
		default: // pull
			b, _ := json.Marshal(agent.CommandResult{Stdout: pullOut, Stderr: pullErr, ExitCode: pullExit})
			return b, nil
		}
	}
}

func executePull(t *testing.T, run func(context.Context, string, ...string) ([]byte, error), repository string) (gitPullResult, error) {
	t.Helper()
	tool := NewGitPullTool()
	tool.run = run
	out, err := tool.Execute(context.Background(), []byte(`{"repository":"`+repository+`"}`))
	if err != nil {
		return gitPullResult{}, err
	}
	var res gitPullResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res, nil
}

func TestGitPullToolMetadata(t *testing.T) {
	tool := NewGitPullTool()
	if tool.Name() != ToolGitPull {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() != "1.0.0" {
		t.Fatalf("unexpected version: %s", tool.Version())
	}
	if tool.Description() == "" {
		t.Fatal("missing description")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationRequired {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestGitPullParameterSchema(t *testing.T) {
	tool := NewGitPullTool()
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

func TestGitPullToolAvailability(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		tool := NewGitPullTool()
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
		tool := NewGitPullTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "git is not installed" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("not runnable", func(t *testing.T) {
		tool := NewGitPullTool()
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

func TestGitPullAlreadyUpToDate(t *testing.T) {
	repo := t.TempDir()
	res, err := executePull(t, fakeGitPullRun("main\n", "origin/main\n", "Already up to date.\n", "", 0), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Repository != repo {
		t.Fatalf("unexpected repository: %s", res.Repository)
	}
	if res.Updated {
		t.Fatal("expected updated=false")
	}
	if res.Branch != "main" || res.Upstream != "origin/main" {
		t.Fatalf("unexpected branch/upstream: %+v", res)
	}
	if res.Message != "Already up to date." {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestGitPullSuccessfulFastForward(t *testing.T) {
	pullOut := "From ./upstream\n   abc123..def456  main -> origin/main\nUpdating abc123..def456\nFast-forward\n file.txt | 1 +\n 1 file changed, 1 insertion(+)\n"
	res, err := executePull(t, fakeGitPullRun("main\n", "origin/main\n", pullOut, "", 0), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Updated {
		t.Fatal("expected updated=true")
	}
	if res.Branch != "main" || res.Upstream != "origin/main" {
		t.Fatalf("unexpected branch/upstream: %+v", res)
	}
	if res.Message != "Fast-forward completed." {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestGitPullDetachedHead(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("", "", "", "", 0)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("expected detached HEAD error, got: %v", err)
	}
}

func TestGitPullNoUpstream(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("feature\n", "", "", "", 0)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "no upstream configured") {
		t.Fatalf("expected no-upstream error, got: %v", err)
	}
}

func TestGitPullRepositoryNotFound(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("main\n", "origin/main\n", "", "", 0)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"/nonexistent/repo"}`))
	if err == nil || !strings.Contains(err.Error(), "repository does not exist") {
		t.Fatalf("expected repository-not-found error, got: %v", err)
	}
}

func TestGitPullInvalidRepository(t *testing.T) {
	tool := NewGitPullTool()
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

func TestGitPullGitNotInstalled(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "git is not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestGitPullFastForwardNotPossible(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("main\n", "origin/main\n", "", "fatal: Not possible to fast-forward, aborting.\n", 128)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "fast-forward not possible") {
		t.Fatalf("expected fast-forward-not-possible error, got: %v", err)
	}
}

func TestGitPullMergeRequired(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("main\n", "origin/main\n", "", "error: Your local changes to the following files would be overwritten by merge:\n\ta.txt\nPlease commit your changes or stash them before you merge.\nAborting\n", 1)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "merge required") {
		t.Fatalf("expected merge-required error, got: %v", err)
	}
}

func TestGitPullMalformedOutput(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("main\n", "origin/main\n", "unrecognized output\n", "", 0)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed output error, got: %v", err)
	}
}

func TestGitPullExecutionFailure(t *testing.T) {
	tool := NewGitPullTool()
	tool.run = fakeGitPullRun("main\n", "origin/main\n", "", "fatal: unable to access 'https://example.com/repo.git/': Could not resolve host\n", 128)
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected execution failure error, got: %v", err)
	}
}

func TestGitPullParseRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"repository":""}`,
		`not json`,
	}
	for _, c := range cases {
		if _, err := parseRepositoryRequest([]byte(c), "git.pull"); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}
