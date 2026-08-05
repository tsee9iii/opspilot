package pm2

import (
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

func TestPM2RestartToolName(t *testing.T) {
	tool := NewPM2RestartTool()
	if tool.Name() != ToolPM2Restart {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestPM2RestartParameterSchema(t *testing.T) {
	tool := NewPM2RestartTool()
	var schema struct {
		Type      string   `json:"type"`
		Required  []string `json:"required"`
		Processes map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("unexpected schema type: %s", schema.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "process" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	process, ok := schema.Processes["process"]
	if !ok || process.Type != "string" {
		t.Fatalf("unexpected process property: %+v", process)
	}
}

func TestParsePM2RestartRequest(t *testing.T) {
	process, err := parsePM2RestartRequest([]byte(`{"process":"backend"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if process != "backend" {
		t.Fatalf("unexpected process: %s", process)
	}
}

func TestParsePM2RestartRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"process":""}`,
		`not json`,
	}
	for _, c := range cases {
		if _, err := parsePM2RestartRequest([]byte(c)); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}

func TestPM2RestartToolExecute(t *testing.T) {
	jlistOut, err := json.Marshal(agent.CommandResult{Stdout: `[{"name":"backend","pid":1}]`, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal jlist: %v", err)
	}
	restartOut, err := json.Marshal(agent.CommandResult{Stdout: "Restarting...", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal restart: %v", err)
	}

	var calls [][]string
	tool := NewPM2RestartTool()
	tool.run = func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{cmd}, args...))
		switch len(calls) {
		case 1:
			return jlistOut, nil
		case 2:
			return restartOut, nil
		default:
			t.Fatalf("unexpected number of calls: %d", len(calls))
			return nil, nil
		}
	}

	result, err := tool.Execute(context.Background(), []byte(`{"process":"backend"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res pm2RestartResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Process != "backend" || res.Status != "restarted" {
		t.Fatalf("unexpected result: %+v", res)
	}

	want := [][]string{
		{"pm2", "jlist"},
		{"pm2", "restart", "backend"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestPM2RestartToolProcessNotFound(t *testing.T) {
	jlistOut, err := json.Marshal(agent.CommandResult{Stdout: `[]`, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal jlist: %v", err)
	}

	tool := NewPM2RestartTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return jlistOut, nil
	}

	_, err = tool.Execute(context.Background(), []byte(`{"process":"missing"}`))
	if err == nil || !strings.Contains(err.Error(), "process not found") {
		t.Fatalf("expected process-not-found error, got: %v", err)
	}
}

func TestPM2RestartToolPM2NotInstalled(t *testing.T) {
	tool := NewPM2RestartTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, &exec.Error{Name: "pm2", Err: exec.ErrNotFound}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"process":"backend"}`))
	if err == nil || !strings.Contains(err.Error(), "pm2 is not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestPM2RestartToolRestartFailure(t *testing.T) {
	jlistOut, err := json.Marshal(agent.CommandResult{Stdout: `[{"name":"backend","pid":1}]`, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal jlist: %v", err)
	}
	failOut, err := json.Marshal(agent.CommandResult{Stderr: "boom\n", ExitCode: 1})
	if err != nil {
		t.Fatalf("marshal fail: %v", err)
	}

	tool := NewPM2RestartTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "jlist" {
			return jlistOut, nil
		}
		return failOut, nil
	}

	_, err = tool.Execute(context.Background(), []byte(`{"process":"backend"}`))
	if err == nil || !strings.Contains(err.Error(), "restart failed") {
		t.Fatalf("expected restart failure error, got: %v", err)
	}
}
