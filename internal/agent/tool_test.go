package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRunCommandCapturesResult(t *testing.T) {
	out, err := RunCommand(context.Background(), "/bin/sh", "-c", "echo hello; echo warn >&2; exit 3")
	if err != nil {
		t.Fatalf("run command: %v", err)
	}

	var res CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Stdout != "hello\n" {
		t.Fatalf("expected stdout %q, got %q", "hello\n", res.Stdout)
	}
	if res.Stderr != "warn\n" {
		t.Fatalf("expected stderr %q, got %q", "warn\n", res.Stderr)
	}
	if res.ExitCode != 3 {
		t.Fatalf("expected exit code 3, got %d", res.ExitCode)
	}
}

func TestRunCommandOutputLimit(t *testing.T) {
	out, err := RunCommand(context.Background(), "/bin/sh", "-c", "yes x | head -c 2000000")
	if err == nil {
		t.Fatalf("expected error for oversized output, got %d bytes", len(out))
	}

	var toolErr *ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected *ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != "output_limit_exceeded" {
		t.Fatalf("expected code output_limit_exceeded, got %q", toolErr.Code)
	}
}

func TestBoundedBufferCapsOutput(t *testing.T) {
	b := &boundedBuffer{}
	payload := make([]byte, 100)
	for range MaxCommandOutputBytes / 10 {
		if _, err := b.Write(payload); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if !b.over {
		t.Fatal("expected overflow flag to be set")
	}
	if b.buf.Len() != MaxCommandOutputBytes {
		t.Fatalf("expected buffer capped at %d, got %d", MaxCommandOutputBytes, b.buf.Len())
	}
}
