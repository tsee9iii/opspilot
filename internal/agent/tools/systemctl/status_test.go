package systemctl

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

func TestSystemCtlStatusToolMetadata(t *testing.T) {
	tool := NewSystemCtlStatusTool()
	if tool.Name() != ToolSystemCtlStatus {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestSystemCtlStatusParameterSchema(t *testing.T) {
	tool := NewSystemCtlStatusTool()
	var schema struct {
		Type       string `json:"type"`
		Required   []string
		Properties map[string]struct {
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
	if len(schema.Required) != 1 || schema.Required[0] != "service" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	service, ok := schema.Properties["service"]
	if !ok {
		t.Fatal("missing service property")
	}
	if service.Type != "string" {
		t.Fatalf("unexpected service type: %s", service.Type)
	}
}

func TestParseSystemCtlStatusRequest(t *testing.T) {
	service, err := parseSystemCtlStatusRequest([]byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if service != "nginx.service" {
		t.Fatalf("unexpected service: %s", service)
	}
}

func TestParseSystemCtlStatusRequestInvalid(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"missing service", []byte(`{}`)},
		{"empty service", []byte(`{"service":""}`)},
		{"malformed json", []byte(`{`)},
	}
	for _, tc := range cases {
		if _, err := parseSystemCtlStatusRequest(tc.payload); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestParseSystemCtlStatus(t *testing.T) {
	stdout := `Id=nginx.service
Description=A high performance web server
LoadState=loaded
ActiveState=active
SubState=running
UnitFileState=enabled
MainPID=1234
ExecMainStatus=0
`
	result, err := parseSystemCtlStatus(stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Service != "nginx.service" {
		t.Fatalf("unexpected service: %s", result.Service)
	}
	if result.Description != "A high performance web server" {
		t.Fatalf("unexpected description: %s", result.Description)
	}
	if result.LoadState != "loaded" || result.ActiveState != "active" || result.SubState != "running" || result.UnitFileState != "enabled" {
		t.Fatalf("unexpected states: %+v", result)
	}
	if result.MainPID != 1234 || result.ExitStatus != 0 {
		t.Fatalf("unexpected numeric fields: main_pid=%d exit_status=%d", result.MainPID, result.ExitStatus)
	}
}

func TestParseSystemCtlStatusDescriptionContainsEquals(t *testing.T) {
	stdout := `Id=my.service
Description=a=b=c
ActiveState=active
`
	result, err := parseSystemCtlStatus(stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Description != "a=b=c" {
		t.Fatalf("unexpected description: %s", result.Description)
	}
}

func TestParseSystemCtlStatusMalformed(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
	}{
		{"no equals", "Id=nginx.service\nnot a property line\n"},
		{"bad main pid", "Id=nginx.service\nMainPID=abc\n"},
		{"bad exit status", "Id=nginx.service\nExecMainStatus=xyz\n"},
		{"missing id", "ActiveState=active\n"},
	}
	for _, tc := range cases {
		if _, err := parseSystemCtlStatus(tc.stdout); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func fakeSystemCtlRun(stdout, stderr string, exit int) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			b, _ := json.Marshal(agent.CommandResult{Stdout: "systemd 252\n", ExitCode: 0})
			return b, nil
		}
		b, _ := json.Marshal(agent.CommandResult{Stdout: stdout, Stderr: stderr, ExitCode: exit})
		return b, nil
	}
}

func TestSystemCtlStatusToolExecuteSuccess(t *testing.T) {
	stdout := `Id=nginx.service
Description=A high performance web server
LoadState=loaded
ActiveState=active
SubState=running
UnitFileState=enabled
MainPID=1234
ExecMainStatus=0
`
	tool := NewSystemCtlStatusTool()
	tool.run = fakeSystemCtlRun(stdout, "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res systemctlStatusResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Service != "nginx.service" || res.ActiveState != "active" || res.MainPID != 1234 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestSystemCtlStatusToolExecuteMissingService(t *testing.T) {
	tool := NewSystemCtlStatusTool()
	tool.run = fakeSystemCtlRun("", "Unit nginx.service could not be found.\n", 1)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "service not found") {
		t.Fatalf("expected service not found error, got: %v", err)
	}
}

func TestSystemCtlStatusToolExecuteNotInstalled(t *testing.T) {
	tool := NewSystemCtlStatusTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}

func TestSystemCtlStatusToolExecuteNonZeroExit(t *testing.T) {
	tool := NewSystemCtlStatusTool()
	tool.run = fakeSystemCtlRun("", "Failed to connect to bus\n", 1)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl show failed") {
		t.Fatalf("expected systemctl failure error, got: %v", err)
	}
}

func TestSystemCtlStatusToolExecuteMalformed(t *testing.T) {
	tool := NewSystemCtlStatusTool()
	tool.run = fakeSystemCtlRun("Id=nginx.service\ngarbage line\n", "", 0)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid property line") {
		t.Fatalf("expected malformed output error, got: %v", err)
	}
}
