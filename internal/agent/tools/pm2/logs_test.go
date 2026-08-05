package pm2

import (
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

func TestPM2LogsToolName(t *testing.T) {
	tool := NewPM2LogsTool()
	if tool.Name() != ToolPM2Logs {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestPM2LogsParameterSchema(t *testing.T) {
	tool := NewPM2LogsTool()
	var schema struct {
		Type       string   `json:"type"`
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type    string `json:"type"`
			Default int    `json:"default"`
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
	lines, ok := schema.Properties["lines"]
	if !ok || lines.Type != "integer" || lines.Default != 100 {
		t.Fatalf("unexpected lines property: %+v", lines)
	}
}

func TestParsePM2LogsRequest(t *testing.T) {
	process, lines, err := parsePM2LogsRequest([]byte(`{"process":"backend"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if process != "backend" || lines != 100 {
		t.Fatalf("unexpected parse: %s %d", process, lines)
	}
}

func TestParsePM2LogsRequestLines(t *testing.T) {
	process, lines, err := parsePM2LogsRequest([]byte(`{"process":"api","lines":250}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if process != "api" || lines != 250 {
		t.Fatalf("unexpected parse: %s %d", process, lines)
	}
}

func TestParsePM2LogsRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"lines":5}`,
		`{"process":"","lines":5}`,
		`{"process":"x","lines":0}`,
		`{"process":"x","lines":-3}`,
		`{"process":"x","lines":1001}`,
		`not json`,
	}
	for _, c := range cases {
		if _, _, err := parsePM2LogsRequest([]byte(c)); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}

func TestPM2LogsToolExecute(t *testing.T) {
	jlistOut, err := json.Marshal(agent.CommandResult{Stdout: `[{"name":"backend","pid":1}]`, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal jlist: %v", err)
	}
	stdoutLogs, err := json.Marshal(agent.CommandResult{Stdout: "line out 1\nline out 2\n", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal stdout logs: %v", err)
	}
	stderrLogs, err := json.Marshal(agent.CommandResult{Stdout: "line err 1\n", ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal stderr logs: %v", err)
	}

	var calls [][]string
	tool := NewPM2LogsTool()
	tool.run = func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{cmd}, args...))
		switch len(calls) {
		case 1:
			return jlistOut, nil
		case 2:
			return stdoutLogs, nil
		case 3:
			return stderrLogs, nil
		default:
			t.Fatalf("unexpected number of calls: %d", len(calls))
			return nil, nil
		}
	}

	result, err := tool.Execute(context.Background(), []byte(`{"process":"backend","lines":50}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res pm2LogsResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Process != "backend" || res.Lines != 50 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Stdout != "line out 1\nline out 2\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
	if res.Stderr != "line err 1\n" {
		t.Fatalf("unexpected stderr: %q", res.Stderr)
	}

	want := [][]string{
		{"pm2", "jlist"},
		{"pm2", "logs", "backend", "--lines", "50", "--nostream", "--raw", "--out"},
		{"pm2", "logs", "backend", "--lines", "50", "--nostream", "--raw", "--err"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected calls: %+v", calls)
	}
}

func TestPM2LogsToolProcessNotFound(t *testing.T) {
	jlistOut, err := json.Marshal(agent.CommandResult{Stdout: `[]`, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal jlist: %v", err)
	}

	tool := NewPM2LogsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return jlistOut, nil
	}

	_, err = tool.Execute(context.Background(), []byte(`{"process":"missing"}`))
	if err == nil || !strings.Contains(err.Error(), "process not found") {
		t.Fatalf("expected process-not-found error, got: %v", err)
	}
}

func TestPM2LogsToolPM2NotInstalled(t *testing.T) {
	tool := NewPM2LogsTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, &exec.Error{Name: "pm2", Err: exec.ErrNotFound}
	}

	_, err := tool.Execute(context.Background(), []byte(`{"process":"backend"}`))
	if err == nil || !strings.Contains(err.Error(), "pm2 is not installed") {
		t.Fatalf("expected not-installed error, got: %v", err)
	}
}

func TestPM2LogsToolNonZeroExit(t *testing.T) {
	jlistOut, err := json.Marshal(agent.CommandResult{Stdout: `[{"name":"backend","pid":1}]`, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal jlist: %v", err)
	}
	failOut, err := json.Marshal(agent.CommandResult{Stderr: "boom\n", ExitCode: 1})
	if err != nil {
		t.Fatalf("marshal fail: %v", err)
	}

	tool := NewPM2LogsTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "jlist" {
			return jlistOut, nil
		}
		return failOut, nil
	}

	_, err = tool.Execute(context.Background(), []byte(`{"process":"backend"}`))
	if err == nil || !strings.Contains(err.Error(), "pm2 logs failed") {
		t.Fatalf("expected logs failure error, got: %v", err)
	}
}
