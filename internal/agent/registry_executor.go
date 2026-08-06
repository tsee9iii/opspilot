package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// RegistryExecutor runs tools via the tool registry and never switches on
// tool names: an unregistered name is simply not found.
type RegistryExecutor struct {
	registry *Registry
	policy   ExecutionPolicy
}

func NewRegistryExecutor(registry *Registry, policy ExecutionPolicy) *RegistryExecutor {
	return &RegistryExecutor{registry: registry, policy: policy}
}

func (e *RegistryExecutor) Execute(ctx context.Context, toolName string, payload []byte) ([]byte, error) {
	tool, ok := e.registry.Find(toolName)
	if !ok {
		log.Printf("debug: registry.Find miss tool=%q registry_size=%d", toolName, len(e.registry.List()))
		return nil, ErrToolNotImplemented
	}

	if err := e.policy.Allow(toolName); err != nil {
		return nil, err
	}

	if err := validatePayload([]byte(tool.ParameterSchema()), payload); err != nil {
		return nil, fmt.Errorf("tool %s: invalid payload: %w", toolName, err)
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if e.policy.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, e.policy.Timeout)
		defer cancel()
	}

	result, err := tool.Execute(execCtx, payload)
	if err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("tool %s: timed out", toolName)
		}
		return nil, fmt.Errorf("tool %s: %w", toolName, err)
	}
	return result, nil
}
