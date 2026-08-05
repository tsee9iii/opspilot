package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appcapability "github.com/opspilot/opspilot/internal/application/capability"
	appcommand "github.com/opspilot/opspilot/internal/application/command"
)

// TestCommandConfirmation exercises the confirmation enforcement that lives in
// SQL: pending commands are never leased, approval makes them leasable, and
// the approval transition is idempotent. It requires a reachable PostgreSQL
// (OPSPILOT_TEST_DATABASE_URL, defaulting to the docker-compose dev database)
// and skips otherwise.
func TestCommandConfirmation(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	agentID := seedAgent(t, ctx, pool)

	commandRepo := NewCommandRepository(pool)
	capabilityRepo := NewCapabilityRepository(pool)

	// Capability confirmation resolution.
	if lvl, err := capabilityRepo.ConfirmationLevel(ctx, agentID, "system.uptime"); err != nil || lvl != "" {
		t.Fatalf("expected empty level for unknown tool, got %q (err %v)", lvl, err)
	}
	if err := capabilityRepo.Upsert(ctx, agentID, appcapability.Capability{
		ToolName: "pm2.restart", Version: "1.0.0", Description: "restart",
		ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "required",
	}); err != nil {
		t.Fatalf("upsert capability: %v", err)
	}
	if lvl, err := capabilityRepo.ConfirmationLevel(ctx, agentID, "pm2.restart"); err != nil || lvl != "required" {
		t.Fatalf("expected required level, got %q (err %v)", lvl, err)
	}

	t.Run("approved command is leasable immediately", func(t *testing.T) {
		resp, err := commandRepo.CreateCommand(ctx, appcommand.CreateCommandRequest{
			AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
			ConfirmationStatus: appcommand.ConfirmationApproved,
		})
		if err != nil {
			t.Fatalf("create command: %v", err)
		}
		leased, err := commandRepo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()})
		if err != nil {
			t.Fatalf("lease command: %v", err)
		}
		if leased.CommandID != resp.CommandID {
			t.Fatalf("expected leased command %s, got %s", resp.CommandID, leased.CommandID)
		}
	})

	t.Run("pending command is never leased until approved", func(t *testing.T) {
		resp, err := commandRepo.CreateCommand(ctx, appcommand.CreateCommandRequest{
			AgentID: agentID.String(), Tool: "pm2.restart", Payload: []byte(`{"process":"web"}`),
			ConfirmationStatus: appcommand.ConfirmationPending,
		})
		if err != nil {
			t.Fatalf("create command: %v", err)
		}

		if _, err := commandRepo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()}); !errors.Is(err, appcommand.ErrNoPendingCommands) {
			t.Fatalf("expected ErrNoPendingCommands, got: %v", err)
		}

		approved, err := commandRepo.ApproveCommand(ctx, appcommand.ApproveCommandRequest{CommandID: resp.CommandID})
		if err != nil {
			t.Fatalf("approve command: %v", err)
		}
		if approved.Status != appcommand.ConfirmationApproved {
			t.Fatalf("expected approved, got %q", approved.Status)
		}

		var status string
		var confirmed bool
		if err := pool.QueryRow(ctx,
			`SELECT confirmation_status, confirmed_at IS NOT NULL FROM commands WHERE id = $1`,
			uuid.MustParse(resp.CommandID)).Scan(&status, &confirmed); err != nil {
			t.Fatalf("read command: %v", err)
		}
		if status != appcommand.ConfirmationApproved || !confirmed {
			t.Fatalf("expected approved + confirmed_at set, got status %q confirmed=%v", status, confirmed)
		}

		leased, err := commandRepo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()})
		if err != nil {
			t.Fatalf("lease after approve: %v", err)
		}
		if leased.CommandID != resp.CommandID {
			t.Fatalf("expected leased command %s, got %s", resp.CommandID, leased.CommandID)
		}
	})

	t.Run("approval is idempotent", func(t *testing.T) {
		resp, err := commandRepo.CreateCommand(ctx, appcommand.CreateCommandRequest{
			AgentID: agentID.String(), Tool: "pm2.restart", Payload: []byte(`{"process":"web"}`),
			ConfirmationStatus: appcommand.ConfirmationApproved,
		})
		if err != nil {
			t.Fatalf("create command: %v", err)
		}
		first, err := commandRepo.ApproveCommand(ctx, appcommand.ApproveCommandRequest{CommandID: resp.CommandID})
		if err != nil {
			t.Fatalf("first approve: %v", err)
		}
		second, err := commandRepo.ApproveCommand(ctx, appcommand.ApproveCommandRequest{CommandID: resp.CommandID})
		if err != nil {
			t.Fatalf("second approve: %v", err)
		}
		if first.Status != appcommand.ConfirmationApproved || second.Status != appcommand.ConfirmationApproved {
			t.Fatalf("expected approved both times, got %q / %q", first.Status, second.Status)
		}
	})

	t.Run("approving an unknown command is not found", func(t *testing.T) {
		_, err := commandRepo.ApproveCommand(ctx, appcommand.ApproveCommandRequest{CommandID: uuid.New().String()})
		if !errors.Is(err, appcommand.ErrCommandNotFound) {
			t.Fatalf("expected ErrCommandNotFound, got: %v", err)
		}
	})

	t.Run("existing commands default to approved and remain leasable", func(t *testing.T) {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO commands (agent_id, tool_name, payload, status) VALUES ($1, 'system.uptime', '{}', 'pending') RETURNING id`,
			agentID).Scan(&id); err != nil {
			t.Fatalf("insert default-approved command: %v", err)
		}
		var status string
		if err := pool.QueryRow(ctx, `SELECT confirmation_status FROM commands WHERE id = $1`, id).Scan(&status); err != nil {
			t.Fatalf("read default status: %v", err)
		}
		if status != appcommand.ConfirmationApproved {
			t.Fatalf("expected default approved, got %q", status)
		}
		// Earlier subtests may have left approved commands queued; drain the
		// queue until the default-approved command is leased.
		var got bool
		for i := 0; i < 10; i++ {
			leased, err := commandRepo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()})
			if errors.Is(err, appcommand.ErrNoPendingCommands) {
				break
			}
			if err != nil {
				t.Fatalf("lease default-approved command: %v", err)
			}
			if leased.CommandID == id.String() {
				got = true
				break
			}
		}
		if !got {
			t.Fatalf("default-approved command %s was never leased", id)
		}
	})
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("OPSPILOT_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://opspilot:opspilot@localhost:5432/opspilot?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test database unavailable: %v", err)
	}
	return pool
}

func resetSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "sql", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read migration %s: %v", e.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("apply migration %s: %v", e.Name(), err)
		}
	}
}

func seedAgent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var serverID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO servers (name, hostname, environment) VALUES ('test-server', 'localhost', 'test') RETURNING id`,
	).Scan(&serverID); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	agentID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO agents (id, server_id, secret, version) VALUES ($1, $2, 'hash', '1.0')`,
		agentID, serverID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return agentID
}
