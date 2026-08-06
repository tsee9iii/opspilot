package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/agentsign"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

// AgentStore fetches an agent's stored signing key for signature verification.
type AgentStore interface {
	GetAgentByID(ctx context.Context, id uuid.UUID) (*domainagent.Agent, error)
}

// AgentAuth authenticates agent requests via HMAC request signing. The request
// must carry the agent id, a Unix timestamp, a per-request nonce and an
// HMAC-SHA256 signature over those fields plus the method, path and body.
//
// Rejected with 401:
//   - missing or malformed headers (invalid_signature)
//   - a timestamp outside the freshness window (expired_timestamp)
//   - an unknown agent (invalid_credentials)
//   - a signature that does not match the stored signing key (invalid_signature)
//   - a nonce already seen within the window (replay_detected)
func AgentAuth(agents AgentStore, window time.Duration) func(http.Handler) http.Handler {
	cache := newReplayCache()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			agentID := r.Header.Get(agentsign.HeaderAgentID)
			ts := r.Header.Get(agentsign.HeaderAgentTimestamp)
			nonce := r.Header.Get(agentsign.HeaderAgentNonce)
			sig := r.Header.Get(agentsign.HeaderAgentSignature)
			if agentID == "" || ts == "" || nonce == "" || sig == "" {
				writeError(w, http.StatusUnauthorized, "invalid_signature", "missing required authentication headers")
				return
			}

			if err := agentsign.CheckTimestamp(ts, window); err != nil {
				writeError(w, http.StatusUnauthorized, "expired_timestamp", "request timestamp is outside the allowed window")
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			id, err := uuid.Parse(agentID)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_signature", "invalid agent id")
				return
			}
			ag, err := agents.GetAgentByID(r.Context(), id)
			if err != nil || ag == nil {
				writeError(w, http.StatusUnauthorized, "invalid_credentials", "agent not found")
				return
			}

			canonical := agentsign.Canonical(agentID, ts, nonce, r.Method, r.URL.Path, string(body))
			if !agentsign.Verify(ag.SigningKey, sig, canonical) {
				writeError(w, http.StatusUnauthorized, "invalid_signature", "invalid request signature")
				return
			}

			if !cache.check(agentID, nonce, window) {
				writeError(w, http.StatusUnauthorized, "replay_detected", "request replay detected")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// replayCache tracks recently seen (agent id, nonce) pairs in memory. Entries
// older than the freshness window are pruned lazily on each check, so the
// cache never outlives the window and needs no background maintenance.
type replayCache struct {
	mu   sync.Mutex
	seen map[string]int64
}

func newReplayCache() *replayCache {
	return &replayCache{seen: make(map[string]int64)}
}

// check reports whether the nonce is fresh for this agent and records it. A
// duplicate within the window is a replay.
func (c *replayCache) check(agentID, nonce string, window time.Duration) bool {
	key := agentID + ":" + nonce
	now := time.Now().Unix()
	cutoff := now - int64(window/time.Second)

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, ts := range c.seen {
		if ts < cutoff {
			delete(c.seen, k)
		}
	}

	if _, ok := c.seen[key]; ok {
		return false
	}
	c.seen[key] = now
	return true
}
