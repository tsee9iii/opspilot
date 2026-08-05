package git

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// fakeGitLogRun dispatches on git subcommands: --version, rev-parse, and log.
func fakeGitLogRun(logOut string) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "rev-parse":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stdout: logOut, ExitCode: 0})
			return b, nil
		}
	}
}

func executeCurrentCommit(t *testing.T, run func(context.Context, string, ...string) ([]byte, error), repository string) (gitCurrentCommitResult, error) {
	t.Helper()
	tool := NewGitCurrentCommitTool()
	tool.run = run
	out, err := tool.Execute(context.Background(), []byte(`{"repository":"`+repository+`"}`))
	if err != nil {
		return gitCurrentCommitResult{}, err
	}
	var res gitCurrentCommitResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res, nil
}

func TestGitCurrentCommitToolMetadata(t *testing.T) {
	tool := NewGitCurrentCommitTool()
	if tool.Name() != ToolGitCurrentCommit {
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

func TestGitCurrentCommitParameterSchema(t *testing.T) {
	tool := NewGitCurrentCommitTool()
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

func TestGitCurrentCommitToolAvailability(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		tool := NewGitCurrentCommitTool()
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
		tool := NewGitCurrentCommitTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "git is not installed" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("not runnable", func(t *testing.T) {
		tool := NewGitCurrentCommitTool()
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

func TestGitCurrentCommitToolExecute(t *testing.T) {
	repo := t.TempDir()
	logOut := "3fbe91cb3d1f5d9d8a2b0c4d5e6f7a8b9c0d1e2f\n3fbe91c\nJohn Smith\njohn@example.com\n2026-08-06T14:35:12+08:00\nFix deployment race condition"
	res, err := executeCurrentCommit(t, fakeGitLogRun(logOut), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Repository != repo {
		t.Fatalf("unexpected repository: %s", res.Repository)
	}
	if res.Commit != "3fbe91cb3d1f5d9d8a2b0c4d5e6f7a8b9c0d1e2f" {
		t.Fatalf("unexpected commit: %s", res.Commit)
	}
	if res.ShortCommit != "3fbe91c" {
		t.Fatalf("unexpected short commit: %s", res.ShortCommit)
	}
	if res.AuthorName != "John Smith" {
		t.Fatalf("unexpected author name: %s", res.AuthorName)
	}
	if res.AuthorEmail != "john@example.com" {
		t.Fatalf("unexpected author email: %s", res.AuthorEmail)
	}
	if res.AuthorDate != "2026-08-06T14:35:12+08:00" {
		t.Fatalf("unexpected author date: %s", res.AuthorDate)
	}
	if res.Subject != "Fix deployment race condition" {
		t.Fatalf("unexpected subject: %s", res.Subject)
	}
}

func TestGitCurrentCommitToolEmptySubject(t *testing.T) {
	logOut := "3fbe91cb3d1f5d9d8a2b0c4d5e6f7a8b9c0d1e2f\n3fbe91c\nJohn Smith\njohn@example.com\n2026-08-06T14:35:12+08:00\n"
	res, err := executeCurrentCommit(t, fakeGitLogRun(logOut), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Subject != "" {
		t.Fatalf("unexpected subject: %q", res.Subject)
	}
	if res.Commit == "" || res.AuthorDate == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestGitCurrentCommitToolNoCommits(t *testing.T) {
	tool := NewGitCurrentCommitTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "rev-parse":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stderr: "fatal: your current branch 'main' does not have any commits yet\n", ExitCode: 128})
			return b, nil
		}
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "repository has no commits") {
		t.Fatalf("expected no-commits error, got: %v", err)
	}
}

func TestGitCurrentCommitToolRepositoryNotFound(t *testing.T) {
	tool := NewGitCurrentCommitTool()
	tool.run = fakeGitLogRun("")
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"/nonexistent/repo"}`))
	if err == nil || !strings.Contains(err.Error(), "repository does not exist") {
		t.Fatalf("expected repository-not-found error, got: %v", err)
	}
}

func TestGitCurrentCommitToolInvalidRepository(t *testing.T) {
	tool := NewGitCurrentCommitTool()
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

func TestGitCurrentCommitToolGitNotInstalled(t *testing.T) {
	tool := NewGitCurrentCommitTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "git is not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestGitCurrentCommitToolMalformedOutput(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"too few fields", "3fbe91c\nJohn Smith\n"},
		{"too many fields", "3fbe91c\nshort\nJohn Smith\njohn@example.com\n2026-08-06T14:35:12+08:00\nsubject\njunk"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := executeCurrentCommit(t, fakeGitLogRun(tc.output), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), "malformed") {
				t.Fatalf("expected malformed output error, got: %v", err)
			}
		})
	}
}

func TestGitCurrentCommitToolExecutionFailure(t *testing.T) {
	tool := NewGitCurrentCommitTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "rev-parse":
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

func TestGitCurrentCommitParseRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"repository":""}`,
		`not json`,
	}
	for _, c := range cases {
		if _, err := parseRepositoryRequest([]byte(c), "git.current_commit"); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}
