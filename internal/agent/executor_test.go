package agent

import (
	"context"
	"errors"
	"testing"
)

func TestStubExecutorNoop(t *testing.T) {
	exec := NewStubExecutor()
	result, err := exec.Execute(context.Background(), ToolNoop, []byte(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `{"status":"ok"}` {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestStubExecutorUnsupportedTool(t *testing.T) {
	exec := NewStubExecutor()
	_, err := exec.Execute(context.Background(), "shell", nil)
	if !errors.Is(err, ErrToolNotImplemented) {
		t.Fatalf("expected ErrToolNotImplemented, got: %v", err)
	}
}
