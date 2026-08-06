package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/tools/system"
)

func TestRegistryExecutorRunsTool(t *testing.T) {
	reg := agent.NewRegistry()
	if err := reg.Register(system.NewUptimeTool()); err != nil {
		t.Fatalf("register: %v", err)
	}

	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{Enabled: true})
	result, err := exec.Execute(context.Background(), system.ToolSystemUptime, nil)
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
	if !strings.Contains(res.Stdout, "up") {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestRegistryExecutorUnknownTool(t *testing.T) {
	reg := agent.NewRegistry()
	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{Enabled: true})
	_, err := exec.Execute(context.Background(), "noop", nil)
	if !errors.Is(err, agent.ErrToolNotImplemented) {
		t.Fatalf("expected ErrToolNotImplemented, got: %v", err)
	}
}

func TestRegistryExecutorPolicyDenied(t *testing.T) {
	reg := agent.NewRegistry()
	if err := reg.Register(system.NewUptimeTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{
		Enabled:        true,
		DeniedCommands: []string{system.ToolSystemUptime},
	})
	_, err := exec.Execute(context.Background(), system.ToolSystemUptime, nil)
	if !errors.Is(err, agent.ErrCommandDenied) {
		t.Fatalf("expected ErrCommandDenied, got: %v", err)
	}
}

func TestRegistryExecutorPolicyNotAllowed(t *testing.T) {
	reg := agent.NewRegistry()
	if err := reg.Register(system.NewUptimeTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{
		Enabled:         true,
		AllowedCommands: []string{"other"},
	})
	_, err := exec.Execute(context.Background(), system.ToolSystemUptime, nil)
	if !errors.Is(err, agent.ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed, got: %v", err)
	}
}

func TestRegistryExecutorDisabledPolicy(t *testing.T) {
	reg := agent.NewRegistry()
	if err := reg.Register(system.NewUptimeTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{Enabled: false})
	_, err := exec.Execute(context.Background(), system.ToolSystemUptime, nil)
	if !errors.Is(err, agent.ErrPolicyDisabled) {
		t.Fatalf("expected ErrPolicyDisabled, got: %v", err)
	}
}

func TestRegistryExecutorTimeout(t *testing.T) {
	reg := agent.NewRegistry()
	if err := reg.Register(&blockingTool{name: "slow"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{Enabled: true, Timeout: 100 * time.Millisecond})

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

func (t *blockingTool) Version() string { return "0.0.1" }

func (t *blockingTool) Description() string { return "blocking tool" }

func (t *blockingTool) ParameterSchema() string { return agent.EmptyParameterSchema }

func (t *blockingTool) ConfirmationLevel() agent.ConfirmationLevel { return agent.ConfirmationNone }

func (t *blockingTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:              t.Name(),
		Description:       t.Description(),
		Category:          agent.CategorySystem,
		Tags:              []string{"test"},
		Risk:              agent.RiskReadOnly,
		EstimatedDuration: agent.DurationShort,
	}
}

func (t *blockingTool) Availability(_ context.Context) (bool, string) { return true, "" }

func (t *blockingTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
