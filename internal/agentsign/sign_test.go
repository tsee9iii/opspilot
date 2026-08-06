package agentsign

import (
	"strconv"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	key := "k3y"
	canonical := Canonical("a1", "1700000000", "n1", "POST", "/api/v1/agents/heartbeat", `{"agent_id":"a1"}`)
	sig := Sign(key, canonical)
	if sig == "" {
		t.Fatal("empty signature")
	}
	if !Verify(key, sig, canonical) {
		t.Fatal("expected signature to verify")
	}
	if Verify("other-key", sig, canonical) {
		t.Fatal("wrong key must not verify")
	}
	if Verify(key, sig, canonical+"x") {
		t.Fatal("tampered canonical must not verify")
	}
	if Verify(key, "zzzz", canonical) {
		t.Fatal("garbage signature must not verify")
	}
}

func TestCanonicalBindsFields(t *testing.T) {
	a := Canonical("a", "t", "n", "POST", "/p", "b")
	b := Canonical("a", "t", "n", "POST", "/p", "b")
	if a != b {
		t.Fatalf("canonical must be deterministic: %q vs %q", a, b)
	}
	if Canonical("x", "t", "n", "POST", "/p", "b") == a {
		t.Fatal("agent id must be bound into the canonical string")
	}
	if Canonical("a", "x", "n", "POST", "/p", "b") == a {
		t.Fatal("timestamp must be bound into the canonical string")
	}
	if Canonical("a", "t", "x", "POST", "/p", "b") == a {
		t.Fatal("nonce must be bound into the canonical string")
	}
	if Canonical("a", "t", "n", "GET", "/p", "b") == a {
		t.Fatal("method must be bound into the canonical string")
	}
	if Canonical("a", "t", "n", "POST", "/x", "b") == a {
		t.Fatal("path must be bound into the canonical string")
	}
	if Canonical("a", "t", "n", "POST", "/p", "x") == a {
		t.Fatal("body must be bound into the canonical string")
	}
}

func TestCheckTimestamp(t *testing.T) {
	window := time.Minute
	now := time.Now().Unix()

	if err := CheckTimestamp(strconv.FormatInt(now, 10), window); err != nil {
		t.Fatalf("current timestamp should be valid: %v", err)
	}
	if err := CheckTimestamp(strconv.FormatInt(now-61, 10), window); err != ErrTimestampExpired {
		t.Fatalf("old timestamp should be expired, got: %v", err)
	}
	if err := CheckTimestamp(strconv.FormatInt(now+61, 10), window); err != ErrTimestampExpired {
		t.Fatalf("future timestamp should be rejected, got: %v", err)
	}
	if err := CheckTimestamp("", window); err != ErrInvalidTimestamp {
		t.Fatalf("empty timestamp should be invalid, got: %v", err)
	}
	if err := CheckTimestamp("not-a-number", window); err != ErrInvalidTimestamp {
		t.Fatalf("non-numeric timestamp should be invalid, got: %v", err)
	}
}
