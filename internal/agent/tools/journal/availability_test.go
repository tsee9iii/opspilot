package journal

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
)

func TestJournalLogsToolAvailability(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		tool := NewJournalLogsTool()
		var binary string
		tool.run = func(_ context.Context, bin string, _ ...string) ([]byte, error) {
			binary = bin
			out, _ := json.Marshal(agent.CommandResult{ExitCode: 0})
			return out, nil
		}
		ok, reason := tool.Availability(context.Background())
		if !ok || reason != "" {
			t.Fatalf("expected available, got ok=%v reason=%q", ok, reason)
		}
		if binary != "journalctl" {
			t.Fatalf("expected check of journalctl binary, got %q", binary)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		tool := NewJournalLogsTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "journalctl is not installed" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("not runnable", func(t *testing.T) {
		tool := NewJournalLogsTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			out, _ := json.Marshal(agent.CommandResult{ExitCode: 1})
			return out, nil
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "journalctl is not runnable" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})
}
