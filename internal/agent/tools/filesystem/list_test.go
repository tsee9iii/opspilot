package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

func newTestLoader(t *testing.T, root string) *project.Loader {
	t.Helper()
	loader, err := project.New([]project.Config{{
		Name: "app",
		Path: root,
		Deploy: &project.DeployConfig{
			Type:        project.StrategyDockerCompose,
			ComposeFile: "docker-compose.yml",
		},
	}})
	if err != nil {
		t.Fatalf("build loader: %v", err)
	}
	return loader
}

// listTool builds a tool with absolute paths allowed. Existing tests exercise
// listing behaviour against absolute temp directories; the default-deny of
// absolute paths is covered separately by TestFilesystemListAbsolutePathDenied.
func listTool(loader *project.Loader) *FilesystemListTool {
	return NewFilesystemListToolWithPolicy(loader, true)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func executeList(t *testing.T, tool *FilesystemListTool, payload string) (listResult, error) {
	t.Helper()
	out, err := tool.Execute(context.Background(), []byte(payload))
	if err != nil {
		return listResult{}, err
	}
	var res listResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res, nil
}

func assertToolError(t *testing.T, err error, code string) {
	t.Helper()
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected structured ToolError, got: %v", err)
	}
	if te.Code != code {
		t.Fatalf("unexpected error_code %q, want %q: %v", te.Code, code, err)
	}
	if te.Message == "" {
		t.Fatal("ToolError missing message")
	}
}

func entryNames(entries []listEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

func TestFilesystemListToolMetadata(t *testing.T) {
	tool := listTool(nil)
	if tool.Name() != ToolFilesystemList {
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
	ok, reason := tool.Availability(context.Background())
	if !ok || reason != "" {
		t.Fatalf("expected always available, got ok=%v reason=%q", ok, reason)
	}
}

func TestFilesystemListParameterSchema(t *testing.T) {
	tool := listTool(nil)
	var schema struct {
		Type                 string   `json:"type"`
		Required             []string `json:"required"`
		AdditionalProperties bool     `json:"additionalProperties"`
		Properties           map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if schema.Type != "object" || len(schema.Required) != 1 || schema.Required[0] != "path" {
		t.Fatalf("unexpected schema: %+v", schema)
	}
	for _, key := range []string{"path", "recursive", "max_depth"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Fatalf("missing property %q", key)
		}
	}
	if schema.AdditionalProperties {
		t.Fatal("expected additionalProperties: false")
	}
}

func TestFilesystemListEmptyDirectory(t *testing.T) {
	res, err := executeList(t, listTool(nil), `{"path":"`+t.TempDir()+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("expected empty listing, got %+v", res.Entries)
	}
}

func TestFilesystemListMixedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.txt"), "b")
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	res, err := executeList(t, listTool(nil), `{"path":"`+root+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"bin", "logs", "a.txt", "b.txt", "link"}
	got := entryNames(res.Entries)
	if len(got) != len(want) {
		t.Fatalf("unexpected entries %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order %v, want %v", got, want)
		}
	}

	byName := map[string]listEntry{}
	for _, e := range res.Entries {
		byName[e.Name] = e
	}
	if byName["bin"].Type != "directory" || byName["logs"].Type != "directory" {
		t.Fatalf("unexpected directory types: %+v", byName)
	}
	if byName["a.txt"].Type != "file" || byName["b.txt"].Type != "file" {
		t.Fatalf("unexpected file types: %+v", byName)
	}
	if byName["link"].Type != "symlink" {
		t.Fatalf("unexpected symlink type: %+v", byName["link"])
	}
	if byName["a.txt"].SizeBytes == nil || *byName["a.txt"].SizeBytes != 1 {
		t.Fatalf("unexpected size: %+v", byName["a.txt"])
	}
	if byName["a.txt"].ModifiedAt == nil {
		t.Fatal("file missing modified_at")
	}
	if _, err := time.Parse(time.RFC3339, *byName["a.txt"].ModifiedAt); err != nil {
		t.Fatalf("modified_at is not RFC3339: %v", err)
	}
	if byName["bin"].SizeBytes != nil || byName["link"].SizeBytes != nil {
		t.Fatalf("directory/symlink must not carry size_bytes: %+v", byName)
	}
}

func TestFilesystemListHiddenFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".env"), "SECRET=1")
	writeFile(t, filepath.Join(root, "app.conf"), "x")

	res, err := executeList(t, listTool(nil), `{"path":"`+root+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := entryNames(res.Entries); len(got) != 2 {
		t.Fatalf("expected hidden + visible file, got %v", got)
	}
	if res.Entries[0].Name != ".env" {
		t.Fatalf("expected hidden file first (alphabetical), got %v", entryNames(res.Entries))
	}
}

func TestFilesystemListSymlinkNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "secret")
	if err := os.Symlink(outside, filepath.Join(root, "target")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "keep.txt"), "keep")

	res, err := executeList(t, listTool(nil), `{"path":"`+root+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range res.Entries {
		if e.Name == "target" && e.Type != "symlink" {
			t.Fatalf("symlink must be reported as symlink, got %+v", e)
		}
	}
}

func TestFilesystemListRecursive(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docker-compose.yml"), "x")
	writeFile(t, filepath.Join(root, "Makefile"), "x")
	if err := os.MkdirAll(filepath.Join(root, "logs", "nginx"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "logs", "app.log"), "x")
	writeFile(t, filepath.Join(root, "logs", "nginx", "default.conf"), "x")

	t.Run("default is not recursive", func(t *testing.T) {
		res, err := executeList(t, listTool(nil), `{"path":"`+root+`"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := entryNames(res.Entries); len(got) != 3 {
			t.Fatalf("expected only top-level entries, got %v", got)
		}
	})

	t.Run("recursive with default depth 1 lists only the top directory", func(t *testing.T) {
		res, err := executeList(t, listTool(nil), `{"path":"`+root+`","recursive":true}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := entryNames(res.Entries); len(got) != 3 {
			t.Fatalf("expected 3 entries, got %v", got)
		}
	})

	t.Run("depth 2 includes immediate subdirectories", func(t *testing.T) {
		res, err := executeList(t, listTool(nil), `{"path":"`+root+`","recursive":true,"max_depth":2}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := entryNames(res.Entries); len(got) != 5 {
			t.Fatalf("expected 5 entries, got %v", got)
		}
	})

	t.Run("depth 3 includes nested directories", func(t *testing.T) {
		res, err := executeList(t, listTool(nil), `{"path":"`+root+`","recursive":true,"max_depth":3}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := entryNames(res.Entries); len(got) != 6 {
			t.Fatalf("expected 6 entries, got %v", got)
		}
	})
}

func TestFilesystemListDepthClamped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "f1"), "x")
	if err := os.MkdirAll(filepath.Join(root, "d1", "d2", "d3", "d4", "d5"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "d1", "d2", "d3", "d4", "d5", "deep.txt"), "x")

	res, err := executeList(t, listTool(nil),
		`{"path":"`+root+`","recursive":true,"max_depth":99}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Clamped to depth 5: d5's own entry is included, but its child (depth 6)
	// is not.
	want := []string{"d1", "d2", "d3", "d4", "d5", "f1"}
	got := entryNames(res.Entries)
	if len(got) != len(want) {
		t.Fatalf("depth not clamped: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected entries %v, want %v", got, want)
		}
	}
}

func TestFilesystemListRelativePath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.conf"), "x")
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}

	tool := listTool(newTestLoader(t, root))
	res, err := executeList(t, tool, `{"path":"logs"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, "logs"))
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	if res.Path != want {
		t.Fatalf("unexpected path: %q, want %q", res.Path, want)
	}
}

func TestFilesystemListTraversalAttack(t *testing.T) {
	root := t.TempDir()
	tool := listTool(newTestLoader(t, root))
	for _, p := range []string{"../secret", "sub/../../outside"} {
		t.Run(p, func(t *testing.T) {
			_, err := executeList(t, tool, `{"path":"`+p+`"}`)
			assertToolError(t, err, "invalid_path")
		})
	}
}

func TestFilesystemListSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "esc")); err != nil {
		t.Fatal(err)
	}

	tool := listTool(newTestLoader(t, root))
	_, err := executeList(t, tool, `{"path":"esc"}`)
	assertToolError(t, err, "invalid_path")
}

func TestFilesystemListDirectoryTooLarge(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < toolListMaxDirEntries+1; i++ {
		writeFile(t, filepath.Join(root, fmt.Sprintf("f%04d", i)), "x")
	}

	_, err := executeList(t, listTool(nil), `{"path":"`+root+`"}`)
	assertToolError(t, err, "directory_too_large")
}

func TestFilesystemListPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed when running as root")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.conf"), "x")
	if err := os.Chmod(root, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(root, 0o755)

	_, err := executeList(t, listTool(nil), `{"path":"`+root+`"}`)
	assertToolError(t, err, "permission_denied")
}

func TestFilesystemListNotADirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.conf")
	writeFile(t, path, "x")

	_, err := executeList(t, listTool(nil), `{"path":"`+path+`"}`)
	assertToolError(t, err, "not_a_directory")
}

func TestFilesystemListMissingDirectory(t *testing.T) {
	_, err := executeList(t, listTool(nil), `{"path":"/nonexistent/missing"}`)
	assertToolError(t, err, "directory_not_found")
}

func TestFilesystemListInvalidPath(t *testing.T) {
	t.Run("no project root for relative path", func(t *testing.T) {
		_, err := executeList(t, listTool(nil), `{"path":"logs"}`)
		assertToolError(t, err, "invalid_path")
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := executeList(t, listTool(nil), `{"path":""}`)
		assertToolError(t, err, "invalid_path")
	})
}

func TestFilesystemListAbsolutePathDenied(t *testing.T) {
	root := t.TempDir()
	tool := NewFilesystemListTool(nil)
	_, err := executeList(t, tool, `{"path":"`+root+`"}`)
	assertToolError(t, err, "invalid_path")

	if _, err := executeList(t, tool, `{"path":"/etc"}`); err == nil {
		t.Fatal("expected /etc to be denied by default")
	}
}

func TestFilesystemListAbsolutePathOptIn(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.conf"), "x")

	tool := NewFilesystemListToolWithPolicy(nil, true)
	res, err := executeList(t, tool, `{"path":"`+root+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Name != "app.conf" {
		t.Fatalf("unexpected entries: %+v", res.Entries)
	}
}

func TestFilesystemListParseErrors(t *testing.T) {
	tool := listTool(nil)
	for _, c := range []string{``, `not json`, `{}`} {
		if _, err := tool.Execute(context.Background(), []byte(c)); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}

func TestFilesystemListRegisteredTool(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.conf"), "x")

	registry := agent.NewRegistry()
	if err := registry.Register(listTool(newTestLoader(t, root))); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})

	out, err := exec.Execute(context.Background(), ToolFilesystemList, []byte(`{"path":"."}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var res listResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Name != "app.conf" {
		t.Fatalf("unexpected entries: %+v", res.Entries)
	}

	if _, ok := registry.Find(ToolFilesystemList); !ok {
		t.Fatalf("tool %s not registered", ToolFilesystemList)
	}
}

func TestFilesystemListRegistryPayloadValidation(t *testing.T) {
	registry := agent.NewRegistry()
	if err := registry.Register(listTool(nil)); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})

	for _, payload := range []string{`{}`, `{"path":1}`, `{"path":"/x","extra":true}`} {
		if _, err := exec.Execute(context.Background(), ToolFilesystemList, []byte(payload)); err == nil {
			t.Fatalf("expected payload validation error for %q", payload)
		}
	}
}
