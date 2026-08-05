package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestUptimeToolName(t *testing.T) {
	tool := NewUptimeTool()
	if tool.Name() != ToolSystemUptime {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestUptimeToolExecute(t *testing.T) {
	tool := NewUptimeTool()
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res commandResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "up") {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestRunCommandCapturesStderrAndExitCode(t *testing.T) {
	result, err := runCommand(context.Background(), "sh", "-c", "echo err >&2; exit 3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res commandResult
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

	_, err := runCommand(ctx, "sh", "-c", "sleep 5")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}
