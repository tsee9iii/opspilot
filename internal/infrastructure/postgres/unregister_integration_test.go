package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	appagent "github.com/tsee9iii/opspilot/internal/application/agent"
	appcapability "github.com/tsee9iii/opspilot/internal/application/capability"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
)

// TestUnregisterAgent verifies the repository unregister behavior: the agent
// transitions to unregistered, its capabilities are deleted, command history
// is preserved, and the operation is idempotent. Requires PostgreSQL.
func TestUnregisterAgent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	agentID := seedAgent(t, ctx, pool)

	capRepo := NewCapabilityRepository(pool)
	for _, tool := range []string{"system.uptime", "pm2.restart"} {
		if err := capRepo.Upsert(ctx, agentID, appcapability.Capability{
			ToolName: tool, Version: "1.0.0", Description: "tool",
			ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "none",
		}); err != nil {
			t.Fatalf("upsert capability %s: %v", tool, err)
		}
	}

	// Historical command data must survive unregister.
	commandRepo := NewCommandRepository(pool)
	cmd, err := commandRepo.CreateCommand(ctx, appcommand.CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
		ConfirmationStatus: appcommand.ConfirmationApproved,
	})
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	_ = cmd

	agentRepo := NewAgentRepository(pool)

	t.Run("unregister marks status and deletes capabilities", func(t *testing.T) {
		if err := agentRepo.UnregisterAgent(ctx, agentID); err != nil {
			t.Fatalf("unregister: %v", err)
		}

		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM agents WHERE id = $1`, agentID).Scan(&status); err != nil {
			t.Fatalf("read agent status: %v", err)
		}
		if status != appagent.StatusUnregistered {
			t.Fatalf("expected status %q, got %q", appagent.StatusUnregistered, status)
		}

		var capCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM capabilities WHERE agent_id = $1`, agentID).Scan(&capCount); err != nil {
			t.Fatalf("count capabilities: %v", err)
		}
		if capCount != 0 {
			t.Fatalf("expected 0 capabilities, got %d", capCount)
		}

		// Command history preserved.
		var cmdCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM commands WHERE agent_id = $1`, agentID).Scan(&cmdCount); err != nil {
			t.Fatalf("count commands: %v", err)
		}
		if cmdCount != 1 {
			t.Fatalf("expected 1 command preserved, got %d", cmdCount)
		}
	})

	t.Run("unregister is idempotent", func(t *testing.T) {
		if err := agentRepo.UnregisterAgent(ctx, agentID); err != nil {
			t.Fatalf("second unregister should succeed: %v", err)
		}
	})

	t.Run("unregister unknown agent is not found", func(t *testing.T) {
		if err := agentRepo.UnregisterAgent(ctx, uuid.New()); !errors.Is(err, appagent.ErrAgentNotFound) {
			t.Fatalf("expected ErrAgentNotFound, got: %v", err)
		}
	})
}
