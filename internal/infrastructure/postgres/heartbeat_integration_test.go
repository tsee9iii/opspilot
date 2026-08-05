package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"

	appagent "github.com/tsee9iii/opspilot/internal/application/agent"
)

// TestUpdateLastHeartbeatStatusTransition verifies the repository heartbeat
// behavior against PostgreSQL: an accepted heartbeat updates
// last_heartbeat/updated_at and transitions offline -> online, while online
// stays online and an unregistered agent is never resurrected.
func TestUpdateLastHeartbeatStatusTransition(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	repo := NewAgentRepository(pool)

	seedStatus := func(t *testing.T, status string) uuid.UUID {
		t.Helper()
		var serverID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO servers (name, hostname, environment) VALUES ('hb-server', $1, 'test') RETURNING id`,
			uuid.NewString()).Scan(&serverID); err != nil {
			t.Fatalf("seed server: %v", err)
		}
		id := uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO agents (id, server_id, secret, version, status) VALUES ($1, $2, 'hash', '1.0', $3)`,
			id, serverID, status); err != nil {
			t.Fatalf("seed agent: %v", err)
		}
		return id
	}

	readAgent := func(t *testing.T, id uuid.UUID) (status string, lastHeartbeat, updatedAt any) {
		t.Helper()
		if err := pool.QueryRow(ctx,
			`SELECT status, last_heartbeat, updated_at FROM agents WHERE id = $1`, id,
		).Scan(&status, &lastHeartbeat, &updatedAt); err != nil {
			t.Fatalf("read agent: %v", err)
		}
		return status, lastHeartbeat, updatedAt
	}

	t.Run("offline to online updates timestamps", func(t *testing.T) {
		id := seedStatus(t, appagent.StatusOffline)
		if err := repo.UpdateLastHeartbeat(ctx, id); err != nil {
			t.Fatalf("update last heartbeat: %v", err)
		}
		status, lastHeartbeat, updatedAt := readAgent(t, id)
		if status != appagent.StatusOnline {
			t.Fatalf("expected status %q, got %q", appagent.StatusOnline, status)
		}
		if lastHeartbeat == nil {
			t.Fatal("expected last_heartbeat to be set")
		}
		if updatedAt == nil {
			t.Fatal("expected updated_at to be set")
		}
	})

	t.Run("online stays online", func(t *testing.T) {
		id := seedStatus(t, appagent.StatusOnline)
		if err := repo.UpdateLastHeartbeat(ctx, id); err != nil {
			t.Fatalf("update last heartbeat: %v", err)
		}
		status, _, _ := readAgent(t, id)
		if status != appagent.StatusOnline {
			t.Fatalf("expected status %q, got %q", appagent.StatusOnline, status)
		}
	})

	t.Run("unregistered stays unregistered", func(t *testing.T) {
		id := seedStatus(t, appagent.StatusUnregistered)
		if err := repo.UpdateLastHeartbeat(ctx, id); err != nil {
			t.Fatalf("update last heartbeat: %v", err)
		}
		status, lastHeartbeat, _ := readAgent(t, id)
		if status != appagent.StatusUnregistered {
			t.Fatalf("expected status %q, got %q", appagent.StatusUnregistered, status)
		}
		if lastHeartbeat != nil {
			t.Fatal("unregistered agent must not have its last_heartbeat updated")
		}
	})
}
