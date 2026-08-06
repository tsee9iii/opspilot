package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// newTestLoader builds a project loader whose single project lives at root.
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

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func executeRead(t *testing.T, tool *FileReadTool, payload string) (fileReadResult, error) {
	t.Helper()
	out, err := tool.Execute(context.Background(), []byte(payload))
	if err != nil {
		return fileReadResult{}, err
	}
	var res fileReadResult
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

func TestFileReadToolMetadata(t *testing.T) {
	tool := NewFileReadTool(nil)
	if tool.Name() != ToolFileRead {
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

func TestFileReadParameterSchema(t *testing.T) {
	tool := NewFileReadTool(nil)
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
	if len(schema.Required) != 1 || schema.Required[0] != "path" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	prop, ok := schema.Properties["path"]
	if !ok || prop.Type != "string" || prop.Description == "" {
		t.Fatalf("unexpected path property: %+v", prop)
	}
	if schema.AdditionalProperties {
		t.Fatal("expected additionalProperties: false")
	}
}

func TestFileReadAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nginx.conf")
	content := []byte("worker_processes 4;\n")
	writeFile(t, path, content)

	res, err := executeRead(t, NewFileReadTool(nil), `{"path":"`+path+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	if res.Path != want {
		t.Fatalf("unexpected path: %q, want %q", res.Path, want)
	}
	if res.SizeBytes != int64(len(content)) {
		t.Fatalf("unexpected size: %d", res.SizeBytes)
	}
	if res.Encoding != "utf-8" {
		t.Fatalf("unexpected encoding: %q", res.Encoding)
	}
	if res.Content != string(content) {
		t.Fatalf("unexpected content: %q", res.Content)
	}
}

func TestFileReadRelativePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docker-compose.yml")
	writeFile(t, path, []byte("version: '3'\nservices: {}\n"))

	tool := NewFileReadTool(newTestLoader(t, root))
	res, err := executeRead(t, tool, `{"path":"docker-compose.yml"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	if res.Path != want {
		t.Fatalf("unexpected path: %q, want %q", res.Path, want)
	}
	if !strings.Contains(res.Content, "version: '3'") {
		t.Fatalf("unexpected content: %q", res.Content)
	}
}

func TestFileReadPreservesContent(t *testing.T) {
	content := []byte("# heading\r\nvalue = \"x\"  \n\tunicode ünïcødé\n")
	path := filepath.Join(t.TempDir(), "app.conf")
	writeFile(t, path, content)

	res, err := executeRead(t, NewFileReadTool(nil), `{"path":"`+path+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Content != string(content) {
		t.Fatalf("content not preserved verbatim:\n got %q\nwant %q", res.Content, string(content))
	}
	if res.SizeBytes != int64(len(content)) {
		t.Fatalf("unexpected size: %d", res.SizeBytes)
	}
}

func TestFileReadMissingFile(t *testing.T) {
	_, err := executeRead(t, NewFileReadTool(nil), `{"path":"/nonexistent/missing.conf"}`)
	assertToolError(t, err, "file_not_found")
}

func TestFileReadDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := executeRead(t, NewFileReadTool(nil), `{"path":"`+dir+`"}`)
	assertToolError(t, err, "directory_not_allowed")
}

func TestFileReadBinaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary.bin")
	writeFile(t, path, []byte{0xff, 0xfe, 0x00, 0x01, 0x80})

	_, err := executeRead(t, NewFileReadTool(nil), `{"path":"`+path+`"}`)
	assertToolError(t, err, "binary_file")
}

func TestFileReadTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.log")
	writeFile(t, path, make([]byte, toolReadMaxSize+1))

	_, err := executeRead(t, NewFileReadTool(nil), `{"path":"`+path+`"}`)
	assertToolError(t, err, "file_too_large")
}

func TestFileReadPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed when running as root")
	}
	path := filepath.Join(t.TempDir(), "secret.conf")
	writeFile(t, path, []byte("secret\n"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := executeRead(t, NewFileReadTool(nil), `{"path":"`+path+`"}`)
	assertToolError(t, err, "permission_denied")
}

func TestFileReadPathEscape(t *testing.T) {
	root := t.TempDir()
	tool := NewFileReadTool(newTestLoader(t, root))

	for _, p := range []string{"../secret", "sub/../../outside.conf", "../.."} {
		t.Run(p, func(t *testing.T) {
			_, err := executeRead(t, tool, `{"path":"`+p+`"}`)
			assertToolError(t, err, "invalid_path")
		})
	}
}

func TestFileReadSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.conf")
	writeFile(t, outside, []byte("outside\n"))

	link := filepath.Join(root, "escape.conf")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	tool := NewFileReadTool(newTestLoader(t, root))
	_, err := executeRead(t, tool, `{"path":"escape.conf"}`)
	assertToolError(t, err, "invalid_path")
}

func TestFileReadSymlinkWithinRoot(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real.conf")
	writeFile(t, real, []byte("real\n"))

	link := filepath.Join(root, "alias.conf")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	tool := NewFileReadTool(newTestLoader(t, root))
	res, err := executeRead(t, tool, `{"path":"alias.conf"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Content != "real\n" {
		t.Fatalf("unexpected content: %q", res.Content)
	}
}

func TestFileReadNoProjectConfigured(t *testing.T) {
	tool := NewFileReadTool(nil)
	_, err := executeRead(t, tool, `{"path":"docker-compose.yml"}`)
	assertToolError(t, err, "invalid_path")
}

func TestFileReadParseErrors(t *testing.T) {
	tool := NewFileReadTool(nil)
	cases := []string{
		``,
		`not json`,
		`{}`,
		`{"path":""}`,
	}
	for _, c := range cases {
		_, err := tool.Execute(context.Background(), []byte(c))
		if err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"path":""}`))
	assertToolError(t, err, "invalid_path")
}

func TestFileReadRegisteredTool(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app.env"), []byte("FOO=bar\n"))

	registry := agent.NewRegistry()
	registry.Register(NewFileReadTool(newTestLoader(t, root)))
	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})

	out, err := exec.Execute(context.Background(), ToolFileRead, []byte(`{"path":"app.env"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var res fileReadResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Content != "FOO=bar\n" {
		t.Fatalf("unexpected content: %q", res.Content)
	}

	if _, ok := registry.Find(ToolFileRead); !ok {
		t.Fatalf("tool %s not registered", ToolFileRead)
	}
}

func TestFileReadRegistryPayloadValidation(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(NewFileReadTool(nil))
	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})

	for _, payload := range []string{`{}`, `{"path":1}`, `{"path":"/x","extra":true}`} {
		if _, err := exec.Execute(context.Background(), ToolFileRead, []byte(payload)); err == nil {
			t.Fatalf("expected payload validation error for %q", payload)
		}
	}
}
