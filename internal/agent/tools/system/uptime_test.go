package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
)

func TestUptimeToolName(t *testing.T) {
	tool := NewUptimeTool()
	if tool.Name() != ToolSystemUptime {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

// TestUptimeToolExecute invokes the real /usr/bin/uptime and must pass on both
// macOS and Linux. Assertions avoid OS-specific stdout wording (Linux uses
// "load average:", macOS "load averages:").
func TestUptimeToolExecute(t *testing.T) {
	tool := NewUptimeTool()
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Fatalf("expected non-empty stdout, got: %q", res.Stdout)
	}
}

// TestUptimeToolExecuteSynthetic verifies the tool returns the injected
// runner's result verbatim, without depending on any host OS.
func TestUptimeToolExecuteSynthetic(t *testing.T) {
	const stdout = " 17:39:32 up 4 days,  1:52, 3 users, load average: 1.23 1.20 1.18\n"
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return json.Marshal(agent.CommandResult{
			Stdout:   stdout,
			ExitCode: 0,
		})
	}
	tool := &UptimeTool{run: run}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
	if res.Stdout != stdout {
		t.Fatalf("expected stdout %q, got: %q", stdout, res.Stdout)
	}
}

// TestUptimeToolExecuteFailure verifies a non-zero exit code from the runner
// is surfaced in the command result.
func TestUptimeToolExecuteFailure(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return json.Marshal(agent.CommandResult{
			Stdout:   "",
			Stderr:   "uptime: failed to read /proc/uptime\n",
			ExitCode: 1,
		})
	}
	tool := &UptimeTool{run: run}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got: %d", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "uptime") {
		t.Fatalf("expected stderr, got: %q", res.Stderr)
	}
}

// TestUptimeToolExecuteRunnerError verifies a runner failure (e.g. a timeout)
// propagates as an error rather than a command result.
func TestUptimeToolExecuteRunnerError(t *testing.T) {
	sentinel := errors.New("tool timed out")
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, sentinel
	}
	tool := &UptimeTool{run: run}

	if _, err := tool.Execute(context.Background(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("expected runner error to propagate, got: %v", err)
	}
}

func TestRunCommandCapturesStderrAndExitCode(t *testing.T) {
	result, err := agent.RunCommand(context.Background(), "sh", "-c", "echo err >&2; exit 3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Stderr != "err\n" {
		t.Fatalf("unexpected stderr: %q", res.Stderr)
	}
	if res.ExitCode != 3 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := agent.RunCommand(ctx, "sh", "-c", "sleep 5")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}
