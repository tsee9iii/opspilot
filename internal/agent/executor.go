package agent

import (
	"context"
	"errors"
)

var ErrToolNotImplemented = errors.New("tool not implemented")

// Executor runs a command's tool against its payload and returns the result.
type Executor interface {
	Execute(ctx context.Context, tool string, payload []byte) ([]byte, error)
}
