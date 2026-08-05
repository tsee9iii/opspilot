package postgres

import (
	"context"
	"testing"

	appcapability "github.com/opspilot/opspilot/internal/application/capability"
)

// TestCapabilityAvailabilityPersistence verifies available/unavailable_reason
// are persisted and refreshed on resync, while other metadata is unchanged.
func TestCapabilityAvailabilityPersistence(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	agentID := seedAgent(t, ctx, pool)

	repo := NewCapabilityRepository(pool)

	first := appcapability.Capability{
		ToolName: "docker.ps", Version: "1.0.0", Description: "list containers",
		ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "none",
		Available: true, UnavailableReason: "",
	}
	if err := repo.Upsert(ctx, agentID, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	var available bool
	var reason string
	var version, description string
	if err := pool.QueryRow(ctx,
		`SELECT available, unavailable_reason, version, description FROM capabilities WHERE agent_id = $1 AND tool_name = $2`,
		agentID, "docker.ps").Scan(&available, &reason, &version, &description); err != nil {
		t.Fatalf("read capability: %v", err)
	}
	if !available || reason != "" {
		t.Fatalf("expected available, got available=%v reason=%q", available, reason)
	}
	if version != "1.0.0" || description != "list containers" {
		t.Fatalf("unexpected metadata: version=%q description=%q", version, description)
	}

	// Resync reflects runtime availability changing (docker removed).
	second := first
	second.Available = false
	second.UnavailableReason = "docker is not installed"
	if err := repo.Upsert(ctx, agentID, second); err != nil {
		t.Fatalf("resync upsert: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`SELECT available, unavailable_reason, version, description FROM capabilities WHERE agent_id = $1 AND tool_name = $2`,
		agentID, "docker.ps").Scan(&available, &reason, &version, &description); err != nil {
		t.Fatalf("read resynced capability: %v", err)
	}
	if available || reason != "docker is not installed" {
		t.Fatalf("expected unavailable after resync, got available=%v reason=%q", available, reason)
	}
	if version != "1.0.0" || description != "list containers" {
		t.Fatalf("metadata changed on resync: version=%q description=%q", version, description)
	}
}
