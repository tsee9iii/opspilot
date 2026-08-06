package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
)

// TestLeaseExpiryReclaimsStaleLease verifies the lazy expiry path: a lease that
// outlived the TTL is returned to pending (and re-leased) on the next lease
// call instead of being claimed by a background scheduler.
func TestLeaseExpiryReclaimsStaleLease(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	agentID := seedAgent(t, ctx, pool)
	repo := NewCommandRepository(pool)

	first := createApprovedCommand(t, ctx, repo, agentID)
	createApprovedCommand(t, ctx, repo, agentID)

	if got, err := repo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{
		AgentID: agentID.String(), LeaseTTL: time.Hour,
	}); err != nil {
		t.Fatalf("lease first: %v", err)
	} else if got.CommandID != first {
		t.Fatalf("expected %s leased first, got %s", first, got.CommandID)
	}

	// Backdate the lease beyond the TTL, simulating a stalled agent, then lease
	// again with a short TTL: the stale lease must be reclaimed and re-issued.
	backdateLease(t, ctx, pool, first, time.Hour)

	got, err := repo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{
		AgentID: agentID.String(), LeaseTTL: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("lease after expiry: %v", err)
	}
	if got.CommandID != first {
		t.Fatalf("expected stale lease on %s to be reclaimed, got %s", first, got.CommandID)
	}
}

// TestLeaseWithoutTTLSkipsExpiry verifies a zero TTL leaves stale leases alone:
// the next lease claims the other pending command instead of reclaiming.
func TestLeaseWithoutTTLSkipsExpiry(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	agentID := seedAgent(t, ctx, pool)
	repo := NewCommandRepository(pool)

	first := createApprovedCommand(t, ctx, repo, agentID)
	second := createApprovedCommand(t, ctx, repo, agentID)

	if _, err := repo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()}); err != nil {
		t.Fatalf("lease first: %v", err)
	}
	backdateLease(t, ctx, pool, first, time.Hour)

	got, err := repo.LeaseNextCommand(ctx, appcommand.LeaseCommandRequest{AgentID: agentID.String()})
	if err != nil {
		t.Fatalf("lease second: %v", err)
	}
	if got.CommandID != second {
		t.Fatalf("expected %s leased when expiry is disabled, got %s", second, got.CommandID)
	}
}

func createApprovedCommand(t *testing.T, ctx context.Context, repo *CommandRepository, agentID uuid.UUID) string {
	t.Helper()
	created, err := repo.CreateCommand(ctx, appcommand.CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
		ConfirmationStatus: appcommand.ConfirmationApproved,
	})
	if err != nil {
		t.Fatalf("create command: %v", err)
	}
	return created.CommandID
}

func backdateLease(t *testing.T, ctx context.Context, pool *pgxpool.Pool, commandID string, d time.Duration) {
	t.Helper()
	id, err := uuid.Parse(commandID)
	if err != nil {
		t.Fatalf("parse command id: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE commands SET leased_at = now() - ($1 * interval '1 second') WHERE id = $2`, d.Seconds(), id); err != nil {
		t.Fatalf("backdate lease: %v", err)
	}
}
