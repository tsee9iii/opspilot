package git

import (
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// fakeGitRun dispatches on git subcommands: --version, rev-parse, and status.
func fakeGitRun(statusOut string) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "rev-parse":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stdout: statusOut, ExitCode: 0})
			return b, nil
		}
	}
}

func executeStatus(t *testing.T, run func(context.Context, string, ...string) ([]byte, error), repository string) (gitStatusResult, error) {
	t.Helper()
	tool := NewGitStatusTool()
	tool.run = run
	out, err := tool.Execute(context.Background(), []byte(`{"repository":"`+repository+`"}`))
	if err != nil {
		return gitStatusResult{}, err
	}
	var res gitStatusResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res, nil
}

func TestGitStatusToolMetadata(t *testing.T) {
	tool := NewGitStatusTool()
	if tool.Name() != ToolGitStatus {
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

func TestGitStatusParameterSchema(t *testing.T) {
	tool := NewGitStatusTool()
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

func TestGitStatusToolAvailability(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		tool := NewGitStatusTool()
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
		tool := NewGitStatusTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "git is not installed" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("not runnable", func(t *testing.T) {
		tool := NewGitStatusTool()
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

func TestGitStatusCleanRepository(t *testing.T) {
	res, err := executeStatus(t, fakeGitRun("## main\n"), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Branch != "main" || res.Detached {
		t.Fatalf("unexpected branch info: %+v", res)
	}
	if res.Ahead != 0 || res.Behind != 0 {
		t.Fatalf("unexpected ahead/behind: %d/%d", res.Ahead, res.Behind)
	}
	if res.Dirty {
		t.Fatal("expected clean repository")
	}
	if len(res.Changes) != 0 {
		t.Fatalf("unexpected changes: %+v", res.Changes)
	}
}

func TestGitStatusDirtyRepository(t *testing.T) {
	res, err := executeStatus(t, fakeGitRun("## main\n M README.md\nM  main.go\n?? new.txt\n"), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Dirty {
		t.Fatal("expected dirty repository")
	}
	want := []GitChange{
		{Path: "README.md", IndexStatus: " ", WorktreeStatus: "M"},
		{Path: "main.go", IndexStatus: "M", WorktreeStatus: " "},
		{Path: "new.txt", IndexStatus: "?", WorktreeStatus: "?"},
	}
	if !reflect.DeepEqual(res.Changes, want) {
		t.Fatalf("unexpected changes: %+v", res.Changes)
	}
}

func TestGitStatusDetachedHead(t *testing.T) {
	res, err := executeStatus(t, fakeGitRun("## HEAD (no branch)\n?? new.txt\n"), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Detached || res.Branch != "" {
		t.Fatalf("expected detached HEAD with no branch, got %+v", res)
	}
	if len(res.Changes) != 1 || res.Changes[0].Path != "new.txt" {
		t.Fatalf("unexpected changes: %+v", res.Changes)
	}
}

func TestGitStatusAheadBehind(t *testing.T) {
	cases := []struct {
		name   string
		header string
		ahead  int
		behind int
	}{
		{"ahead", "## main...origin/main [ahead 2]\n", 2, 0},
		{"behind", "## main...origin/main [behind 3]\n", 0, 3},
		{"ahead and behind", "## main...origin/main [ahead 2, behind 1]\n", 2, 1},
		{"no tracking", "## main\n", 0, 0},
		{"gone upstream", "## main [gone]\n", 0, 0},
		{"detached ahead", "## HEAD (no branch, ahead 4)\n", 4, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := executeStatus(t, fakeGitRun(tc.header), t.TempDir())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Ahead != tc.ahead || res.Behind != tc.behind {
				t.Fatalf("ahead/behind = %d/%d, want %d/%d", res.Ahead, res.Behind, tc.ahead, tc.behind)
			}
		})
	}
}

func TestGitStatusParseBranchHeader(t *testing.T) {
	info, err := parseBranchHeader("## feature/x...origin/feature/x [ahead 5, behind 2]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Branch != "feature/x" || info.Detached || info.Ahead != 5 || info.Behind != 2 {
		t.Fatalf("unexpected branch info: %+v", info)
	}
}

func TestGitStatusRepositoryNotFound(t *testing.T) {
	tool := NewGitStatusTool()
	tool.run = fakeGitRun("## main\n")
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"/nonexistent/repo"}`))
	if err == nil || !strings.Contains(err.Error(), "repository does not exist") {
		t.Fatalf("expected repository-not-found error, got: %v", err)
	}
}

func TestGitStatusInvalidRepository(t *testing.T) {
	tool := NewGitStatusTool()
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

func TestGitStatusGitNotInstalled(t *testing.T) {
	tool := NewGitStatusTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "git is not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestGitStatusExecutionFailure(t *testing.T) {
	tool := NewGitStatusTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch {
		case len(args) > 0 && args[0] == "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "git version 2.39.2\n", ExitCode: 0})
			return b, nil
		case len(args) > 2 && args[2] == "rev-parse":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "true\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stderr: "fatal: index file corrupt\n", ExitCode: 128})
			return b, nil
		}
	}
	_, err := tool.Execute(context.Background(), []byte(`{"repository":"`+t.TempDir()+`"}`))
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected execution failure error, got: %v", err)
	}
}

func TestGitStatusMalformedOutput(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"missing branch header", " M README.md\n"},
		{"empty output", ""},
		{"bad change line", "## main\nnot-a-status-line\n"},
		{"short line", "## main\nM\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := executeStatus(t, fakeGitRun(tc.output), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), "malformed") {
				t.Fatalf("expected malformed output error, got: %v", err)
			}
		})
	}
}

func TestGitStatusPreservesFileOrdering(t *testing.T) {
	res, err := executeStatus(t, fakeGitRun("## main\n A b.txt\n M a.txt\n?? z.txt\n M c.txt\n"), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var paths []string
	for _, c := range res.Changes {
		paths = append(paths, c.Path)
	}
	want := []string{"b.txt", "a.txt", "z.txt", "c.txt"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("file ordering not preserved: %v", paths)
	}
}

func TestGitStatusParseRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"repository":""}`,
		`not json`,
	}
	for _, c := range cases {
		if _, err := parseRepositoryRequest([]byte(c), "git.status"); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}
