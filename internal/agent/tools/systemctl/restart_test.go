package systemctl

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const restartShowOutput = `Id=nginx.service
Description=A high performance web server
LoadState=loaded
ActiveState=active
SubState=running
UnitFileState=enabled
MainPID=1234
ExecMainStatus=0
`

func fakeSystemCtlRestartRun(restartErr error, showExit int) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "systemd 252\n", ExitCode: 0})
			return b, nil
		case "show":
			b, _ := json.Marshal(agent.CommandResult{Stdout: restartShowOutput, ExitCode: showExit})
			return b, nil
		default:
			if restartErr != nil {
				return nil, restartErr
			}
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Restarting nginx.service...\n", Stderr: "", ExitCode: 0})
			return b, nil
		}
	}
}

func TestSystemCtlRestartToolMetadata(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	if tool.Name() != ToolSystemCtlRestart {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
}

func TestSystemCtlRestartConfirmationLevel(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	if tool.ConfirmationLevel() != agent.ConfirmationRequired {
		t.Fatalf("expected required confirmation, got: %s", tool.ConfirmationLevel())
	}
}

func TestSystemCtlRestartParameterSchema(t *testing.T) {
	tool := NewSystemCtlRestartTool()
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

func TestParseSystemCtlRestartRequest(t *testing.T) {
	service, err := parseSystemCtlRestartRequest([]byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if service != "nginx.service" {
		t.Fatalf("unexpected service: %s", service)
	}
}

func TestParseSystemCtlRestartRequestInvalid(t *testing.T) {
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
		if _, err := parseSystemCtlRestartRequest(tc.payload); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestSystemCtlRestartToolExecuteSuccess(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	tool.run = fakeSystemCtlRestartRun(nil, 0)

	result, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res systemctlRestartResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Service != "nginx.service" || res.Status != "restarted" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestSystemCtlRestartToolExecuteServiceExists(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	tool.run = fakeSystemCtlRestartRun(nil, 0)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err != nil {
		t.Fatalf("expected success for existing service, got: %v", err)
	}
}

func TestSystemCtlRestartToolExecuteServiceNotFound(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "systemd 252\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stderr: "Unit nginx.service could not be found.\n", ExitCode: 1})
			return b, nil
		}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "service not found") {
		t.Fatalf("expected service not found error, got: %v", err)
	}
}

func TestSystemCtlRestartToolExecuteNotInstalled(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}

func TestSystemCtlRestartToolExecuteRestartFailure(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "systemd 252\n", ExitCode: 0})
			return b, nil
		case "show":
			b, _ := json.Marshal(agent.CommandResult{Stdout: restartShowOutput, ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stderr: "Failed to restart nginx.service: Unit not loaded\n", ExitCode: 1})
			return b, nil
		}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil || !strings.Contains(err.Error(), "systemctl restart failed") {
		t.Fatalf("expected restart failure error, got: %v", err)
	}
}

func TestSystemCtlRestartToolExecuteExecutionFailure(t *testing.T) {
	tool := NewSystemCtlRestartTool()
	tool.run = fakeSystemCtlRestartRun(exec.ErrNotFound, 0)

	_, err := tool.Execute(context.Background(), []byte(`{"service":"nginx.service"}`))
	if err == nil {
		t.Fatal("expected execution failure error")
	}
}
