package agent

import (
	"context"
	"errors"
)

const ToolNoop = "noop"

var ErrToolNotImplemented = errors.New("tool not implemented")

// Executor runs a command's tool against its payload and returns the result.
type Executor interface {
	Execute(ctx context.Context, tool string, payload []byte) ([]byte, error)
}

// StubExecutor implements a stub tool set. It never executes shell commands.
type StubExecutor struct{}

func NewStubExecutor() *StubExecutor {
	return &StubExecutor{}
}

func (e *StubExecutor) Execute(_ context.Context, tool string, _ []byte) ([]byte, error) {
	if tool != ToolNoop {
		return nil, ErrToolNotImplemented
	}
	return []byte(`{"status":"ok"}`), nil
}
