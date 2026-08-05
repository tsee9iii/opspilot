package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegistryExecutorRunsTool(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewUptimeTool())

	exec := NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true})
	result, err := exec.Execute(context.Background(), ToolSystemUptime, nil)
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

func TestRegistryExecutorUnknownTool(t *testing.T) {
	reg := NewRegistry()
	exec := NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true})
	_, err := exec.Execute(context.Background(), "noop", nil)
	if !errors.Is(err, ErrToolNotImplemented) {
		t.Fatalf("expected ErrToolNotImplemented, got: %v", err)
	}
}

func TestRegistryExecutorPolicyDenied(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewUptimeTool())
	exec := NewRegistryExecutor(reg, ExecutionPolicy{
		Enabled:        true,
		DeniedCommands: []string{ToolSystemUptime},
	})
	_, err := exec.Execute(context.Background(), ToolSystemUptime, nil)
	if !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("expected ErrCommandDenied, got: %v", err)
	}
}

func TestRegistryExecutorPolicyNotAllowed(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewUptimeTool())
	exec := NewRegistryExecutor(reg, ExecutionPolicy{
		Enabled:         true,
		AllowedCommands: []string{"other"},
	})
	_, err := exec.Execute(context.Background(), ToolSystemUptime, nil)
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed, got: %v", err)
	}
}

func TestRegistryExecutorDisabledPolicy(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewUptimeTool())
	exec := NewRegistryExecutor(reg, ExecutionPolicy{Enabled: false})
	_, err := exec.Execute(context.Background(), ToolSystemUptime, nil)
	if !errors.Is(err, ErrPolicyDisabled) {
		t.Fatalf("expected ErrPolicyDisabled, got: %v", err)
	}
}

func TestRegistryExecutorTimeout(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&blockingTool{name: "slow"})
	exec := NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true, Timeout: 100 * time.Millisecond})

	start := time.Now()
	_, err := exec.Execute(context.Background(), "slow", nil)
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

type blockingTool struct {
	name string
}

func (t *blockingTool) Name() string { return t.name }

func (t *blockingTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
