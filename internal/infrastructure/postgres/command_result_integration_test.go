package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
)

// TestCommandResultRetrieval verifies GetCommand returns the stored command
// state with the result passed through as stored (opaque JSON). The payload and
// result columns are jsonb, so JSON is compared semantically rather than
// byte-for-byte.
func TestCommandResultRetrieval(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	agentID := seedAgent(t, ctx, pool)
	commandRepo := NewCommandRepository(pool)

	created, err := commandRepo.CreateCommand(ctx, appcommand.CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{"interval":5}`),
		ConfirmationStatus: appcommand.ConfirmationApproved,
	})
	if err != nil {
		t.Fatalf("create command: %v", err)
	}

	t.Run("returns pending command without result", func(t *testing.T) {
		got, err := commandRepo.GetCommand(ctx, appcommand.GetCommandRequest{CommandID: created.CommandID})
		if err != nil {
			t.Fatalf("get command: %v", err)
		}
		if got.ID.String() != created.CommandID {
			t.Fatalf("expected id %s, got %s", created.CommandID, got.ID)
		}
		if got.AgentID != agentID {
			t.Fatalf("expected agent %s, got %s", agentID, got.AgentID)
		}
		if got.Status != appcommand.StatusPending || got.Tool != "system.uptime" {
			t.Fatalf("unexpected state: %+v", got)
		}
		assertJSONEqual(t, got.Parameters, []byte(`{"interval":5}`))
		if got.LeasedAt != nil || got.CompletedAt != nil {
			t.Fatalf("expected nil timestamps for pending command, got %+v", got)
		}
	})
	t.Run("returns result exactly as stored", func(t *testing.T) {
		if _, err := commandRepo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()}); err != nil {
			t.Fatalf("lease command: %v", err)
		}
		if _, err := commandRepo.StartCommand(ctx, appcommand.StartCommandRequest{
			AgentID: agentID.String(), CommandID: created.CommandID,
		}); err != nil {
			t.Fatalf("start command: %v", err)
		}
		result := []byte(`{"uptime_seconds":42,"output":["line1","line2"]}`)
		if _, err := commandRepo.CompleteCommand(ctx, appcommand.CompleteCommandRequest{
			AgentID: agentID.String(), CommandID: created.CommandID, Result: result,
		}); err != nil {
			t.Fatalf("complete command: %v", err)
		}

		got, err := commandRepo.GetCommand(ctx, appcommand.GetCommandRequest{CommandID: created.CommandID})
		if err != nil {
			t.Fatalf("get command: %v", err)
		}
		if got.Status != appcommand.StatusCompleted {
			t.Fatalf("expected completed, got %q", got.Status)
		}
		assertJSONEqual(t, got.Result, result)
		if got.CompletedAt == nil {
			t.Fatalf("expected completed_at to be set")
		}
	})

	t.Run("failed command exposes error", func(t *testing.T) {
		created2, err := commandRepo.CreateCommand(ctx, appcommand.CreateCommandRequest{
			AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
			ConfirmationStatus: appcommand.ConfirmationApproved,
		})
		if err != nil {
			t.Fatalf("create command: %v", err)
		}
		if _, err := commandRepo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()}); err != nil {
			t.Fatalf("lease command: %v", err)
		}
		if _, err := commandRepo.StartCommand(ctx, appcommand.StartCommandRequest{
			AgentID: agentID.String(), CommandID: created2.CommandID,
		}); err != nil {
			t.Fatalf("start command: %v", err)
		}
		if _, err := commandRepo.FailCommand(ctx, appcommand.FailCommandRequest{
			AgentID: agentID.String(), CommandID: created2.CommandID, Error: "timeout",
		}); err != nil {
			t.Fatalf("fail command: %v", err)
		}

		got, err := commandRepo.GetCommand(ctx, appcommand.GetCommandRequest{CommandID: created2.CommandID})
		if err != nil {
			t.Fatalf("get command: %v", err)
		}
		if got.Status != appcommand.StatusFailed || got.Error != "timeout" {
			t.Fatalf("expected failed + timeout, got %+v", got)
		}
	})

	t.Run("unknown command is not found", func(t *testing.T) {
		_, err := commandRepo.GetCommand(ctx, appcommand.GetCommandRequest{CommandID: uuid.New().String()})
		if !errors.Is(err, appcommand.ErrCommandNotFound) {
			t.Fatalf("expected ErrCommandNotFound, got: %v", err)
		}
	})
}

// assertJSONEqual fails unless got and want encode to the same JSON document.
func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gotV, wantV any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("got is not valid JSON: %s (%v)", got, err)
	}
	if err := json.Unmarshal(want, &wantV); err != nil {
		t.Fatalf("want is not valid JSON: %s (%v)", want, err)
	}
	if !reflect.DeepEqual(gotV, wantV) {
		t.Fatalf("expected JSON %s, got %s", want, got)
	}
}
