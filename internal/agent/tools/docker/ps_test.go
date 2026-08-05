package docker

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

func TestDockerPsToolName(t *testing.T) {
	tool := NewDockerPsTool()
	if tool.Name() != ToolDockerPs {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
	if tool.ParameterSchema() != agent.EmptyParameterSchema {
		t.Fatalf("unexpected schema: %s", tool.ParameterSchema())
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestParseDockerPS(t *testing.T) {
	stdout := `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":"0.0.0.0:8080->80/tcp"}
{"ID":"def456","Names":["/api","/api2"],"Image":"node:20","State":"running","Status":"Up 2 hours","Ports":""}`
	containers, err := parseDockerPS(stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got: %d", len(containers))
	}

	c := containers[0]
	if c.ID != "abc123" || c.Name != "/web" || c.Image != "nginx:latest" || c.State != "running" || c.Status != "Up 5 minutes" || c.Ports != "0.0.0.0:8080->80/tcp" {
		t.Fatalf("unexpected first container: %+v", c)
	}

	c = containers[1]
	if c.ID != "def456" || c.Name != "/api, /api2" || c.Image != "node:20" || c.State != "running" || c.Status != "Up 2 hours" || c.Ports != "" {
		t.Fatalf("unexpected second container: %+v", c)
	}
}

func TestParseDockerPSInvalidJSON(t *testing.T) {
	if _, err := parseDockerPS("not-json\n"); err == nil {
		t.Fatal("expected error for invalid JSON line")
	}
}

func TestDockerPsToolExecuteEmpty(t *testing.T) {
	out, err := json.Marshal(agent.CommandResult{Stdout: "", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal command result: %v", err)
	}

	tool := NewDockerPsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, nil
	}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp dockerPsResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if resp.Containers == nil || len(resp.Containers) != 0 {
		t.Fatalf("expected empty container list, got: %#v", resp.Containers)
	}
}

func TestDockerPsToolExecuteMultiple(t *testing.T) {
	stdout := `{"ID":"abc123","Names":["/web"],"Image":"nginx:latest","State":"running","Status":"Up 5 minutes","Ports":"0.0.0.0:8080->80/tcp"}
{"ID":"def456","Names":["/db"],"Image":"postgres:16","State":"running","Status":"Up 2 hours","Ports":"5432/tcp"}`
	out, err := json.Marshal(agent.CommandResult{Stdout: stdout, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal command result: %v", err)
	}

	tool := NewDockerPsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, nil
	}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp dockerPsResponse
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(resp.Containers) != 2 {
		t.Fatalf("expected 2 containers, got: %d", len(resp.Containers))
	}
	if resp.Containers[0].ID != "abc123" || resp.Containers[0].Name != "/web" || resp.Containers[0].Image != "nginx:latest" {
		t.Fatalf("unexpected first container: %+v", resp.Containers[0])
	}
	if resp.Containers[0].Ports != "0.0.0.0:8080->80/tcp" {
		t.Fatalf("unexpected first ports: %s", resp.Containers[0].Ports)
	}
	if resp.Containers[1].ID != "def456" || resp.Containers[1].Name != "/db" || resp.Containers[1].Ports != "5432/tcp" {
		t.Fatalf("unexpected second container: %+v", resp.Containers[1])
	}
}

func TestDockerPsToolExecuteInvalidJSON(t *testing.T) {
	out, err := json.Marshal(agent.CommandResult{Stdout: "garbage\n", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal command result: %v", err)
	}

	tool := NewDockerPsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, nil
	}

	_, err = tool.Execute(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected invalid JSON error, got: %v", err)
	}
}

func TestDockerPsToolExecuteNotInstalled(t *testing.T) {
	tool := NewDockerPsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "docker is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}

func TestDockerPsToolExecuteNonZeroExit(t *testing.T) {
	out, err := json.Marshal(agent.CommandResult{Stderr: "Cannot connect to the Docker daemon\n", ExitCode: 1})
	if err != nil {
		t.Fatalf("marshal command result: %v", err)
	}

	tool := NewDockerPsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, nil
	}

	_, err = tool.Execute(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "docker ps failed") {
		t.Fatalf("expected docker failure error, got: %v", err)
	}
}
