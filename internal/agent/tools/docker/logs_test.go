package docker

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

func TestDockerLogsToolMetadata(t *testing.T) {
	tool := NewDockerLogsTool()
	if tool.Name() != ToolDockerLogs {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestDockerLogsParameterSchema(t *testing.T) {
	tool := NewDockerLogsTool()
	var schema struct {
		Type       string `json:"type"`
		Required   []string
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			Default     any    `json:"default"`
			Minimum     any    `json:"minimum"`
			Maximum     any    `json:"maximum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("unexpected schema type: %s", schema.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "container" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	if _, ok := schema.Properties["container"]; !ok {
		t.Fatal("missing container property")
	}
	lines, ok := schema.Properties["lines"]
	if !ok {
		t.Fatal("missing lines property")
	}
	if lines.Default != float64(100) {
		t.Fatalf("unexpected lines default: %v", lines.Default)
	}
	if lines.Minimum != float64(1) || lines.Maximum != float64(1000) {
		t.Fatalf("unexpected lines bounds: %v %v", lines.Minimum, lines.Maximum)
	}
}

func TestParseDockerLogsRequestDefaultLines(t *testing.T) {
	container, lines, err := parseDockerLogsRequest([]byte(`{"container":"web"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if container != "web" || lines != 100 {
		t.Fatalf("unexpected parse: container=%s lines=%d", container, lines)
	}
}

func TestParseDockerLogsRequestCustomLines(t *testing.T) {
	container, lines, err := parseDockerLogsRequest([]byte(`{"container":"web","lines":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if container != "web" || lines != 50 {
		t.Fatalf("unexpected parse: container=%s lines=%d", container, lines)
	}
}

func TestParseDockerLogsRequestInvalid(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"missing container", []byte(`{}`)},
		{"empty container", []byte(`{"container":""}`)},
		{"lines too small", []byte(`{"container":"web","lines":0}`)},
		{"lines too large", []byte(`{"container":"web","lines":1001}`)},
		{"malformed json", []byte(`{`)},
	}
	for _, tc := range cases {
		if _, _, err := parseDockerLogsRequest(tc.payload); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestContainerMatches(t *testing.T) {
	c := dockerContainer{ID: "abc123", Name: "/web, /web2"}
	cases := []struct {
		idOrName string
		want     bool
	}{
		{"abc123", true},
		{"web", true},
		{"/web", true},
		{"web2", true},
		{"other", false},
	}
	for _, tc := range cases {
		if got := containerMatches(c, tc.idOrName); got != tc.want {
			t.Fatalf("containerMatches(%q) = %v, want %v", tc.idOrName, got, tc.want)
		}
	}
}

func fakeDockerLogsRun(stdout, stderr string, exit int, psOut string) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Docker version 26.1.1\n", ExitCode: 0})
			return b, nil
		case "ps":
			b, _ := json.Marshal(agent.CommandResult{Stdout: psOut, ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stdout: stdout, Stderr: stderr, ExitCode: exit})
			return b, nil
		}
	}
}

func TestDockerLogsToolExecuteContainerExists(t *testing.T) {
	psOut := `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":"0.0.0.0:8080->80/tcp"}`
	tool := NewDockerLogsTool()
	tool.run = fakeDockerLogsRun("line out 1\nline out 2\n", "line err 1\n", 0, psOut)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"web","lines":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerLogsResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Container != "web" || res.Lines != 50 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDockerLogsToolExecuteStdoutCapture(t *testing.T) {
	psOut := `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":""}`
	tool := NewDockerLogsTool()
	tool.run = fakeDockerLogsRun("hello from logs\n", "", 0, psOut)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerLogsResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Stdout != "hello from logs\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestDockerLogsToolExecuteStderrCapture(t *testing.T) {
	psOut := `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":""}`
	tool := NewDockerLogsTool()
	tool.run = fakeDockerLogsRun("", "boom from stderr\n", 0, psOut)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerLogsResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Stderr != "boom from stderr\n" {
		t.Fatalf("unexpected stderr: %q", res.Stderr)
	}
}

func TestDockerLogsToolExecuteContainerNotFound(t *testing.T) {
	tool := NewDockerLogsTool()
	tool.run = fakeDockerLogsRun("", "", 0, "")

	_, err := tool.Execute(context.Background(), []byte(`{"container":"ghost"}`))
	if err == nil || !strings.Contains(err.Error(), "container not found") {
		t.Fatalf("expected container not found error, got: %v", err)
	}
}

func TestDockerLogsToolExecuteDockerNotInstalled(t *testing.T) {
	tool := NewDockerLogsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err == nil || !strings.Contains(err.Error(), "docker is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}

func TestDockerLogsToolExecuteNonZeroExit(t *testing.T) {
	psOut := `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":""}`
	tool := NewDockerLogsTool()
	tool.run = fakeDockerLogsRun("", "error from daemon\n", 1, psOut)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err == nil || !strings.Contains(err.Error(), "docker logs failed") {
		t.Fatalf("expected docker logs failure error, got: %v", err)
	}
}
