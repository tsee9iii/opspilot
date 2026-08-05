package docker

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

const restartPSLine = `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":"0.0.0.0:8080->80/tcp"}`

func fakeDockerRestartRun(restartErr error) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Docker version 26.1.1\n", ExitCode: 0})
			return b, nil
		case "ps":
			b, _ := json.Marshal(agent.CommandResult{Stdout: restartPSLine, ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stdout: "web\n", Stderr: "", ExitCode: 0})
			if restartErr != nil {
				return nil, restartErr
			}
			return b, nil
		}
	}
}

func TestDockerRestartToolMetadata(t *testing.T) {
	tool := NewDockerRestartTool()
	if tool.Name() != ToolDockerRestart {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
}

func TestDockerRestartConfirmationLevel(t *testing.T) {
	tool := NewDockerRestartTool()
	if tool.ConfirmationLevel() != agent.ConfirmationRequired {
		t.Fatalf("expected required confirmation, got: %s", tool.ConfirmationLevel())
	}
}

func TestDockerRestartParameterSchema(t *testing.T) {
	tool := NewDockerRestartTool()
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
	if len(schema.Required) != 1 || schema.Required[0] != "container" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	container, ok := schema.Properties["container"]
	if !ok {
		t.Fatal("missing container property")
	}
	if container.Type != "string" {
		t.Fatalf("unexpected container type: %s", container.Type)
	}
}

func TestParseDockerRestartRequest(t *testing.T) {
	container, err := parseDockerRestartRequest([]byte(`{"container":"web"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if container != "web" {
		t.Fatalf("unexpected container: %s", container)
	}
}

func TestParseDockerRestartRequestInvalid(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"missing container", []byte(`{}`)},
		{"empty container", []byte(`{"container":""}`)},
		{"malformed json", []byte(`{`)},
	}
	for _, tc := range cases {
		if _, err := parseDockerRestartRequest(tc.payload); err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
	}
}

func TestDockerRestartToolExecuteSuccess(t *testing.T) {
	tool := NewDockerRestartTool()
	tool.run = fakeDockerRestartRun(nil)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerRestartResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Container != "web" || res.Status != "restarted" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestDockerRestartToolExecuteContainerExists(t *testing.T) {
	tool := NewDockerRestartTool()
	tool.run = fakeDockerRestartRun(nil)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err != nil {
		t.Fatalf("expected success for existing container, got: %v", err)
	}
}

func TestDockerRestartToolExecuteContainerNotFound(t *testing.T) {
	tool := NewDockerRestartTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Docker version 26.1.1\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stdout: "", ExitCode: 0})
			return b, nil
		}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"container":"ghost"}`))
	if err == nil || !strings.Contains(err.Error(), "container not found") {
		t.Fatalf("expected container not found error, got: %v", err)
	}
}

func TestDockerRestartToolExecuteDockerNotInstalled(t *testing.T) {
	tool := NewDockerRestartTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err == nil || !strings.Contains(err.Error(), "docker is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}

func TestDockerRestartToolExecuteRestartFailure(t *testing.T) {
	tool := NewDockerRestartTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Docker version 26.1.1\n", ExitCode: 0})
			return b, nil
		case "ps":
			b, _ := json.Marshal(agent.CommandResult{Stdout: restartPSLine, ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stderr: "Error response from daemon: boom\n", ExitCode: 1})
			return b, nil
		}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err == nil || !strings.Contains(err.Error(), "docker restart failed") {
		t.Fatalf("expected restart failure error, got: %v", err)
	}
}

func TestDockerRestartToolExecuteExecutionFailure(t *testing.T) {
	tool := NewDockerRestartTool()
	tool.run = fakeDockerRestartRun(exec.ErrNotFound)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	if err == nil {
		t.Fatal("expected execution failure error")
	}
}
