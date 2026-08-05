package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestShellExecutorCapturesStdout(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true})
	result, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res shellExecResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Stdout != "hi\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
}

func TestShellExecutorCapturesStderrAndExitCode(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true})
	result, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"echo err >&2; exit 3"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res shellExecResult
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

func TestShellExecutorUnknownTool(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true})
	_, err := exec.Execute(context.Background(), "noop", nil)
	if !errors.Is(err, ErrToolNotImplemented) {
		t.Fatalf("expected ErrToolNotImplemented, got: %v", err)
	}
}

func TestShellExecutorMissingCommand(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true})
	_, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestShellExecutorCanceledContext(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := exec.Execute(ctx, ToolShellExec, []byte(`{"command":"sleep 30"}`))
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestShellExecutorDeniedCommand(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true, DeniedCommands: []string{"rm"}})
	_, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"rm -rf /tmp/x"}`))
	if !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("expected ErrCommandDenied, got: %v", err)
	}
}

func TestShellExecutorNotAllowedCommand(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true, AllowedCommands: []string{"uptime"}})
	_, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"ls /tmp"}`))
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed, got: %v", err)
	}
}

func TestShellExecutorDisabledPolicy(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: false})
	_, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"uptime"}`))
	if !errors.Is(err, ErrPolicyDisabled) {
		t.Fatalf("expected ErrPolicyDisabled, got: %v", err)
	}
}

func TestShellExecutorTimeout(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true, Timeout: 100 * time.Millisecond})
	_, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"sleep 5"}`))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestShellExecutorWorkingDirectory(t *testing.T) {
	exec := NewShellExecutor(ExecutionPolicy{Enabled: true, WorkingDirectory: "/tmp"})
	result, err := exec.Execute(context.Background(), ToolShellExec, []byte(`{"command":"pwd"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res shellExecResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Stdout != "/tmp\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}
