package pm2

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opspilot/opspilot/internal/agent"
)

func TestPM2ListToolName(t *testing.T) {
	tool := NewPM2ListTool()
	if tool.Name() != ToolPM2List {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestParsePM2List(t *testing.T) {
	data := `[
	  {"pid":1234,"name":"api","pm2_env":{"status":"online","pm_uptime":1700000000000},"monit":{"memory":104857600,"cpu":1.5}},
	  {"pid":5678,"name":"worker","pm2_env":{"status":"stopped","pm_uptime":0},"monit":{"memory":0,"cpu":0}}
	]`
	procs, err := parsePM2List([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("expected 2 processes, got: %d", len(procs))
	}
	if procs[0].Name != "api" || procs[0].Pid != 1234 || procs[0].Monit.CPU != 1.5 || procs[0].Monit.Memory != 104857600 {
		t.Fatalf("unexpected first process: %+v", procs[0])
	}
	if procs[0].PM2Env.Status != "online" || procs[0].PM2Env.PMUptime != 1700000000000 {
		t.Fatalf("unexpected first pm2_env: %+v", procs[0].PM2Env)
	}
}

func TestParsePM2ListInvalid(t *testing.T) {
	if _, err := parsePM2List([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid pm2 output")
	}
}

func TestMapPM2Processes(t *testing.T) {
	now := time.UnixMilli(1700001000000) // 1000s after the fixture pm_uptime
	raw := []pm2RawProcess{
		{
			Name: "api",
			Pid:  1234,
			Monit: struct {
				CPU    float64 `json:"cpu"`
				Memory int64   `json:"memory"`
			}{CPU: 2.25, Memory: 1024},
			PM2Env: struct {
				Status   string `json:"status"`
				PMUptime int64  `json:"pm_uptime"`
			}{Status: "online", PMUptime: 1700000000000},
		},
		{
			Name: "idle",
			Pid:  2,
			Monit: struct {
				CPU    float64 `json:"cpu"`
				Memory int64   `json:"memory"`
			}{},
			PM2Env: struct {
				Status   string `json:"status"`
				PMUptime int64  `json:"pm_uptime"`
			}{Status: "stopped", PMUptime: 0},
		},
	}

	results := mapPM2Processes(raw, now)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got: %d", len(results))
	}
	r := results[0]
	if r.Name != "api" || r.Status != "online" || r.PID != 1234 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.CPUPercent != 2.25 || r.MemoryBytes != 1024 {
		t.Fatalf("unexpected usage: %+v", r)
	}
	if r.Uptime != 1000 {
		t.Fatalf("unexpected uptime: %d", r.Uptime)
	}
	if results[1].Uptime != 0 {
		t.Fatalf("expected zero uptime, got: %d", results[1].Uptime)
	}
}

func TestMapPM2ProcessesClampsFutureUptime(t *testing.T) {
	now := time.UnixMilli(1000)
	raw := []pm2RawProcess{{
		Name: "x",
		PM2Env: struct {
			Status   string `json:"status"`
			PMUptime int64  `json:"pm_uptime"`
		}{PMUptime: 5000},
	}}
	results := mapPM2Processes(raw, now)
	if results[0].Uptime != 0 {
		t.Fatalf("expected clamped uptime, got: %d", results[0].Uptime)
	}
}

func TestPM2ListToolExecute(t *testing.T) {
	stdout := `[{"pid":9,"name":"web","pm2_env":{"status":"online","pm_uptime":1700000000000},"monit":{"memory":4194304,"cpu":0.5}}]`
	out, err := json.Marshal(agent.CommandResult{Stdout: stdout, ExitCode: 0})
	if err != nil {
		t.Fatalf("marshal command result: %v", err)
	}

	tool := NewPM2ListTool()
	tool.now = func() time.Time { return time.UnixMilli(1700001000000) }
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, nil
	}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []pm2ListResult
	if err := json.Unmarshal(result, &results); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got: %d", len(results))
	}
	r := results[0]
	if r.Name != "web" || r.Status != "online" || r.PID != 9 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.CPUPercent != 0.5 || r.MemoryBytes != 4194304 || r.Uptime != 1000 {
		t.Fatalf("unexpected usage/uptime: %+v", r)
	}
}

func TestPM2ListToolExecuteFailsOnNonZeroExit(t *testing.T) {
	out, err := json.Marshal(agent.CommandResult{Stderr: "boom\n", ExitCode: 1})
	if err != nil {
		t.Fatalf("marshal command result: %v", err)
	}

	tool := NewPM2ListTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return out, nil
	}

	_, err = tool.Execute(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "pm2 jlist failed") {
		t.Fatalf("expected pm2 failure error, got: %v", err)
	}
}
