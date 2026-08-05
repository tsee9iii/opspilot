package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
)

// TestRegistrationTokenList verifies List returns all unconsumed tokens, most
// recently created first, mapped with pointers for nullable columns. It requires
// a reachable PostgreSQL (see testPool) and skips otherwise.
func TestRegistrationTokenList(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)
	if _, err := pool.Exec(ctx, `DELETE FROM registration_tokens`); err != nil {
		t.Fatalf("clear tokens: %v", err)
	}

	repo := NewRegistrationTokenRepository(pool)
	env := "production"

	if err := repo.Create(ctx, registrationtoken.RegistrationToken{
		TokenHash:   "hash-oldest",
		Environment: &env,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create oldest: %v", err)
	}
	if err := repo.Create(ctx, registrationtoken.RegistrationToken{
		TokenHash:   "hash-newest",
		Environment: nil,
		ExpiresAt:   time.Now().Add(48 * time.Hour),
	}); err != nil {
		t.Fatalf("create newest: %v", err)
	}

	// Revoke the newest token so List surfaces RevokedAt and confirms revoke
	// does not delete rows.
	var newestID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM registration_tokens WHERE token_hash = 'hash-newest'`).Scan(&newestID); err != nil {
		t.Fatalf("read newest id: %v", err)
	}
	if err := repo.Revoke(ctx, newestID); err != nil {
		t.Fatalf("revoke newest: %v", err)
	}

	tokens, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	// Newest (revoked) first, oldest second.
	first, second := tokens[0], tokens[1]
	if first.TokenHash != "hash-newest" || second.TokenHash != "hash-oldest" {
		t.Fatalf("expected newest-first ordering, got %s then %s", first.TokenHash, second.TokenHash)
	}
	if first.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set on revoked token")
	}
	if second.RevokedAt != nil {
		t.Fatal("expected revoked_at to be nil on active token")
	}
	if first.Environment != nil {
		t.Fatalf("expected nil environment, got %v", first.Environment)
	}
	if second.Environment == nil || *second.Environment != "production" {
		t.Fatalf("expected environment production, got %v", second.Environment)
	}
	if second.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be populated")
	}
}
