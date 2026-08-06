// Package agentsign implements the HMAC request-signing protocol shared by the
// agent (signer) and central (verifier). It is intentionally dependency-free:
// both binaries import it, so the canonical string and header names live in one
// place.
//
// Every agent request is authenticated with four headers: agent id, Unix
// timestamp, a per-request nonce and an HMAC-SHA256 signature over
//
//	agent_id "\n" timestamp "\n" nonce "\n" method "\n" path "\n" body
//
// The signature is keyed with the agent's per-agent signing key issued at
// registration. Binding the method, path and body prevents captured signatures
// from being replayed against a different request.
package agentsign

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Header names carried on every signed agent request.
const (
	HeaderAgentID        = "X-Agent-Id"
	HeaderAgentTimestamp = "X-Agent-Timestamp"
	HeaderAgentNonce     = "X-Agent-Nonce"
	HeaderAgentSignature = "X-Agent-Signature"
)

// DefaultTimestampWindow is how long a signed request remains valid. A request
// whose timestamp is older than the window (or in the future) is rejected.
const DefaultTimestampWindow = 5 * time.Minute

// SigningKeyLen is the length in bytes of a generated per-agent signing key.
const SigningKeyLen = 32

// Canonical builds the byte-for-byte string a signature is computed over.
func Canonical(agentID, timestamp, nonce, method, path, body string) string {
	return strings.Join([]string{agentID, timestamp, nonce, method, path, body}, "\n")
}

// Sign computes the hex HMAC-SHA256 signature of canonical under key.
func Sign(key, canonical string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature is a valid HMAC of canonical under key.
func Verify(key, signature, canonical string) bool {
	expected := Sign(key, canonical)
	if len(signature) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

var (
	// ErrInvalidTimestamp is returned when a timestamp is missing or malformed.
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	// ErrTimestampExpired is returned when a timestamp falls outside the window.
	ErrTimestampExpired = errors.New("timestamp outside allowed window")
)

// CheckTimestamp validates a Unix-second timestamp against the given window.
// Timestamps in the future are rejected too, so a skewed or captured clock
// cannot produce an eternally valid request.
func CheckTimestamp(value string, window time.Duration) error {
	if value == "" {
		return ErrInvalidTimestamp
	}
	secs, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}
	ts := time.Unix(secs, 0)
	now := time.Now()
	if now.Sub(ts) > window || ts.Sub(now) > window {
		return ErrTimestampExpired
	}
	return nil
}

// Timestamp returns the current Unix time as a seconds string.
func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// NewNonce returns a random per-request nonce as hex.
func NewNonce() (string, error) {
	return randomHex(16)
}

// NewSigningKey returns a random per-agent signing key as hex. It is generated
// by central at registration, returned to the agent and stored for verification.
func NewSigningKey() (string, error) {
	return randomHex(SigningKeyLen)
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
