package journal

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

func TestJournalLogsToolMetadata(t *testing.T) {
	tool := NewJournalLogsTool()
	if tool.Name() != ToolJournalLogs {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestJournalLogsParameterSchema(t *testing.T) {
	tool := NewJournalLogsTool()
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
	if len(schema.Required) != 1 || schema.Required[0] != "service" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	if _, ok := schema.Properties["service"]; !ok {
		t.Fatal("missing service property")
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

func TestParseJournalLogsRequestDefaultLines(t *testing.T) {
	service, lines, err := parseJournalLogsRequest([]byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if service != "nginx.service" || lines != 100 {
		t.Fatalf("unexpected parse: service=%s lines=%d", service, lines)
	}
}

func TestParseJournalLogsRequestCustomLines(t *testing.T) {
	service, lines, err := parseJournalLogsRequest([]byte(`{"service":"nginx.service","lines":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if service != "nginx.service" || lines != 50 {
		t.Fatalf("unexpected parse: service=%s lines=%d", service, lines)
	}
}

func TestParseJournalLogsRequestInvalid(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"missing service", []byte(`{}`)},
		{"empty service", []byte(`{"service":""}`)},
		{"lines too small", []byte(`{"service":"nginx.service","lines":0}`)},
		{"lines too large", []byte(`{"service":"nginx.service","lines":1001}`)},
		{"malformed json", []byte(`{`)},
	}
	for _, tc := range cases {
		if _, _, err := parseJournalLogsRequest(tc.payload); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func fakeJournalRun(stdout, stderr string, exit int) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			b, _ := json.Marshal(agent.CommandResult{Stdout: "systemd 252\n", ExitCode: 0})
			return b, nil
		}
		b, _ := json.Marshal(agent.CommandResult{Stdout: stdout, Stderr: stderr, ExitCode: exit})
		return b, nil
	}
}

func TestJournalLogsToolExecuteSuccess(t *testing.T) {
	stdout := "2026-01-01T10:00:00+00:00 host nginx[1234]: worker started\n2026-01-01T10:00:01+00:00 host nginx[1234]: handling request\n"
	tool := NewJournalLogsTool()
	tool.run = fakeJournalRun(stdout, "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service","lines":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res journalLogsResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Service != "nginx.service" || res.Lines != 50 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Stdout != stdout || res.Stderr != "" {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
}

func TestJournalLogsToolExecuteDefaultLines(t *testing.T) {
	tool := NewJournalLogsTool()
	tool.run = fakeJournalRun("2026-01-01T10:00:00+00:00 host nginx[1234]: line\n", "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var res journalLogsResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Lines != 100 {
		t.Fatalf("unexpected lines: %d", res.Lines)
	}
}

func TestJournalLogsToolExecuteServiceNotFound(t *testing.T) {
	tool := NewJournalLogsTool()
	tool.run = fakeJournalRun("", "", 0)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"missing.service"}`))
	if err == nil || !strings.Contains(err.Error(), "service not found") {
		t.Fatalf("expected service not found error, got: %v", err)
	}
}

func TestJournalLogsToolExecuteNotInstalled(t *testing.T) {
	tool := NewJournalLogsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "journalctl is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}

func TestJournalLogsToolExecuteNonZeroExit(t *testing.T) {
	tool := NewJournalLogsTool()
	tool.run = fakeJournalRun("", "Failed to open journal files\n", 1)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "journalctl failed") {
		t.Fatalf("expected journalctl failure error, got: %v", err)
	}
}

func TestJournalLogsToolExecuteExecutionFailure(t *testing.T) {
	tool := NewJournalLogsTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			b, _ := json.Marshal(agent.CommandResult{Stdout: "systemd 252\n", ExitCode: 0})
			return b, nil
		}
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil {
		t.Fatal("expected execution failure error")
	}
}
