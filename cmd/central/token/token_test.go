package token

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/domain/registrationtoken"
	"github.com/tsee9iii/opspilot/internal/infrastructure/security"
)

// fakeRepo records calls and returns canned results.
type fakeRepo struct {
	created   *registrationtoken.RegistrationToken
	tokens    []*registrationtoken.RegistrationToken
	revokedID *uuid.UUID
	err       error
}

func (f *fakeRepo) Create(_ context.Context, t registrationtoken.RegistrationToken) error {
	if f.err != nil {
		return f.err
	}
	f.created = &t
	return nil
}

func (f *fakeRepo) List(_ context.Context) ([]*registrationtoken.RegistrationToken, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tokens, nil
}

func (f *fakeRepo) Revoke(_ context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.revokedID = &id
	return nil
}

func newDeps(repo *fakeRepo) (Deps, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return Deps{
		Repo:   repo,
		Hasher: security.NewHMACHasher("test-server-secret"),
		Out:    &out,
		ErrOut: &errOut,
	}, &out, &errOut
}

func envPtr(s string) *string { return &s }

func TestCreateOutputFormatAndHash(t *testing.T) {
	repo := &fakeRepo{}
	deps, out, _ := newDeps(repo)

	if err := runCreate(context.Background(), deps, nil); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	got := out.String()
	if !strings.HasPrefix(got, "Registration Token\n\nops_rt_") {
		t.Fatalf("unexpected output:\n%s", got)
	}

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines (header, blank, token), got %d:\n%s", len(lines), got)
	}
	plain := lines[2]

	// The stored value must be the HMAC of the plain token, never the token.
	if repo.created == nil {
		t.Fatal("create was never called on the repository")
	}
	if repo.created.TokenHash == plain {
		t.Fatal("plain token was stored")
	}
	want := deps.Hasher.Hash(plain)
	if repo.created.TokenHash != want {
		t.Fatalf("stored hash mismatch: got %q want %q", repo.created.TokenHash, want)
	}
	if repo.created.Environment == nil || *repo.created.Environment != "production" {
		t.Fatalf("expected default environment production, got %v", repo.created.Environment)
	}
	if !repo.created.ExpiresAt.After(time.Now()) {
		t.Fatal("expected expires_at in the future")
	}
}

func TestTokenFormat(t *testing.T) {
	tok, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if !strings.HasPrefix(tok, "ops_rt_") {
		t.Fatalf("missing prefix: %q", tok)
	}
	payload := strings.TrimPrefix(tok, "ops_rt_")
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload is not base64url: %v", err)
	}
	if len(raw) < 32 {
		t.Fatalf("expected at least 32 bytes entropy, got %d", len(raw))
	}
	second, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if tok == second {
		t.Fatal("two generated tokens must differ")
	}
}

func TestCreateEnvironmentOption(t *testing.T) {
	repo := &fakeRepo{}
	deps, _, _ := newDeps(repo)

	if err := runCreate(context.Background(), deps, []string{"--environment", "staging"}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}
	if repo.created == nil || repo.created.Environment == nil || *repo.created.Environment != "staging" {
		t.Fatalf("expected environment staging, got %v", repo.created.Environment)
	}
}

func TestCreateExpirationOption(t *testing.T) {
	for _, tc := range []struct {
		arg   string
		delta time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"90s", 90 * time.Second},
		{"", defaultExpiry},
	} {
		repo := &fakeRepo{}
		deps, _, _ := newDeps(repo)
		args := []string{}
		if tc.arg != "" {
			args = []string{"--expires", tc.arg}
		}
		if err := runCreate(context.Background(), deps, args); err != nil {
			t.Fatalf("create with --expires %q: %v", tc.arg, err)
		}
		before := time.Now()
		got := repo.created.ExpiresAt
		// expiry should be within a small tolerance of now+delta
		lo := before.Add(tc.delta - 2*time.Second)
		hi := before.Add(tc.delta + 2*time.Second)
		if got.Before(lo) || got.After(hi) {
			t.Fatalf("--expires %q: expires_at %v not near now+%v", tc.arg, got, tc.delta)
		}
	}
}

func TestCreateInvalidExpiration(t *testing.T) {
	repo := &fakeRepo{}
	deps, _, _ := newDeps(repo)
	if err := runCreate(context.Background(), deps, []string{"--expires", "bogus"}); err == nil {
		t.Fatal("expected error for invalid lifetime")
	}
	if repo.created != nil {
		t.Fatal("no token should be created for invalid lifetime")
	}
}

func TestCreateRepositoryError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	deps, _, _ := newDeps(repo)
	if err := runCreate(context.Background(), deps, nil); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestListOutput(t *testing.T) {
	id := uuid.New()
	created := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	revoked := created.Add(24 * time.Hour)
	repo := &fakeRepo{tokens: []*registrationtoken.RegistrationToken{
		{ID: id, TokenHash: "never-shown", Environment: envPtr("production"), CreatedAt: created, ExpiresAt: expires, RevokedAt: &revoked},
		{ID: uuid.New(), TokenHash: "never-shown-2", Environment: envPtr("staging"), CreatedAt: created.Add(2 * time.Hour), ExpiresAt: expires.Add(2 * time.Hour)},
	}}
	deps, out, _ := newDeps(repo)

	if err := runList(context.Background(), deps, nil); err != nil {
		t.Fatalf("runList: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "Environment", "Created At", "Expires At", "Revoked", "Consumed", "production", "staging", id.String(), "yes", "no"} {
		if !strings.Contains(got, want) {
			t.Fatalf("list output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "never-shown") {
		t.Fatalf("list output must never show token values:\n%s", got)
	}
}

func TestListExpiredTokenStillShown(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	repo := &fakeRepo{tokens: []*registrationtoken.RegistrationToken{
		{ID: uuid.New(), TokenHash: "h", Environment: envPtr("production"), CreatedAt: past, ExpiresAt: past},
	}}
	deps, out, _ := newDeps(repo)

	if err := runList(context.Background(), deps, nil); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(out.String(), "no") {
		t.Fatalf("expected expired (unconsumed) token to be listed:\n%s", out.String())
	}
}

func TestListRepositoryError(t *testing.T) {
	repo := &fakeRepo{err: errors.New("db down")}
	deps, _, _ := newDeps(repo)
	if err := runList(context.Background(), deps, nil); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRevoke(t *testing.T) {
	id := uuid.New()
	repo := &fakeRepo{}
	deps, out, _ := newDeps(repo)

	if err := runRevoke(context.Background(), deps, []string{id.String()}); err != nil {
		t.Fatalf("runRevoke: %v", err)
	}
	if repo.revokedID == nil || *repo.revokedID != id {
		t.Fatalf("expected revoke id %s, got %v", id, repo.revokedID)
	}
	if !strings.Contains(out.String(), "revoked "+id.String()) {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRevokeTwice(t *testing.T) {
	id := uuid.New()
	repo := &fakeRepo{}
	deps, _, _ := newDeps(repo)

	if err := runRevoke(context.Background(), deps, []string{id.String()}); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	// A second revoke is a no-op update and must not error.
	if err := runRevoke(context.Background(), deps, []string{id.String()}); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestRevokeInvalidID(t *testing.T) {
	repo := &fakeRepo{}
	deps, _, _ := newDeps(repo)
	if err := runRevoke(context.Background(), deps, []string{"not-a-uuid"}); err == nil {
		t.Fatal("expected error for invalid token id")
	}
	if repo.revokedID != nil {
		t.Fatal("repository must not be called for invalid id")
	}
}

func TestRevokeMissingID(t *testing.T) {
	repo := &fakeRepo{}
	deps, _, _ := newDeps(repo)
	if err := runRevoke(context.Background(), deps, nil); err == nil {
		t.Fatal("expected error when id is missing")
	}
}

func TestDispatchUnknown(t *testing.T) {
	deps, _, _ := newDeps(&fakeRepo{})
	if err := dispatch(context.Background(), deps, []string{"nope"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestDispatchMissing(t *testing.T) {
	deps, _, _ := newDeps(&fakeRepo{})
	if err := dispatch(context.Background(), deps, nil); err == nil {
		t.Fatal("expected error for missing subcommand")
	}
}

// TestOpenBootstrapFailure verifies a clear error when configuration cannot be
// loaded (bad YAML file).
func TestOpenBootstrapFailure(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("server: [unclosed\n bad: :::"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSPILOT_CONFIG", bad)

	_, _, err := open(context.Background())
	if err == nil {
		t.Fatal("expected config load error")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("expected clear config error, got: %v", err)
	}
}

// TestOpenDatabaseFailure verifies a clear error when the database is
// unreachable.
func TestOpenDatabaseFailure(t *testing.T) {
	t.Setenv("OPSPILOT_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("OPSPILOT_DB_HOST", "127.0.0.1")
	t.Setenv("OPSPILOT_DB_PORT", "1")
	t.Setenv("OPSPILOT_DB_USER", "opspilot")
	t.Setenv("OPSPILOT_DB_PASSWORD", "opspilot")
	t.Setenv("OPSPILOT_DB_NAME", "opspilot")
	t.Setenv("OPSPILOT_DB_SSLMODE", "disable")

	_, _, err := open(context.Background())
	if err == nil {
		t.Fatal("expected database connection error")
	}
	if !strings.Contains(err.Error(), "connect database") {
		t.Fatalf("expected clear database error, got: %v", err)
	}
}
