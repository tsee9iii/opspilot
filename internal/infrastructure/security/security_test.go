package security

import (
	"context"
	"testing"
)

func TestArgon2idRoundTrip(t *testing.T) {
	h := NewArgon2idHasher()
	ctx := context.Background()

	enc, err := h.Hash(ctx, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := h.Verify(ctx, enc, "super-secret")
	if err != nil || !ok {
		t.Fatalf("expected match: ok=%v err=%v", ok, err)
	}
	ok, err = h.Verify(ctx, enc, "wrong")
	if err != nil || ok {
		t.Fatalf("expected mismatch: ok=%v err=%v", ok, err)
	}
}

func TestHMACHasher(t *testing.T) {
	h := NewHMACHasher("pepper")
	a := h.Hash("token-1")
	b := h.Hash("token-1")
	c := h.Hash("token-2")
	if a == "" || a != b {
		t.Fatalf("deterministic hash expected: %q %q", a, b)
	}
	if a == c {
		t.Fatalf("different tokens must differ: %q %q", a, c)
	}
}
