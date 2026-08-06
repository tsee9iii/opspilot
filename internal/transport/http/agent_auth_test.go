package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/agentsign"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

type stubAgentStore struct {
	agents map[uuid.UUID]*domainagent.Agent
}

func (s *stubAgentStore) GetAgentByID(_ context.Context, id uuid.UUID) (*domainagent.Agent, error) {
	return s.agents[id], nil
}

// signedRequest builds an httptest request signed with the agentsign protocol.
func signedRequest(t *testing.T, method, path, agentID, key, ts, nonce, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(agentsign.HeaderAgentID, agentID)
	req.Header.Set(agentsign.HeaderAgentTimestamp, ts)
	req.Header.Set(agentsign.HeaderAgentNonce, nonce)
	req.Header.Set(agentsign.HeaderAgentSignature, agentsign.Sign(key, agentsign.Canonical(agentID, ts, nonce, method, path, body)))
	return req
}

func newAuthFixture(t *testing.T, window time.Duration) (uuid.UUID, string, http.Handler) {
	t.Helper()
	agentID := uuid.New()
	key, err := agentsign.NewSigningKey()
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	store := &stubAgentStore{agents: map[uuid.UUID]*domainagent.Agent{
		agentID: {ID: agentID, SigningKey: key},
	}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return agentID, key, chain(next, AgentAuth(store, window))
}

func TestAgentAuthRequiresAllHeaders(t *testing.T) {
	agentID, _, h := newAuthFixture(t, agentsign.DefaultTimestampWindow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", strings.NewReader(`{}`))
	req.Header.Set(agentsign.HeaderAgentID, agentID.String())
	req.Header.Set(agentsign.HeaderAgentTimestamp, agentsign.Timestamp())
	req.Header.Set(agentsign.HeaderAgentNonce, "abc")
	// signature intentionally missing

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if want := "invalid_signature"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestAgentAuthRejectsExpiredTimestamp(t *testing.T) {
	agentID, key, h := newAuthFixture(t, time.Minute)

	old := strconv.FormatInt(time.Now().Add(-2*time.Minute).Unix(), 10)
	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", agentID.String(), key, old, "nonce-1", `{}`)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if want := "expired_timestamp"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestAgentAuthRejectsFutureTimestamp(t *testing.T) {
	agentID, key, h := newAuthFixture(t, time.Minute)

	future := strconv.FormatInt(time.Now().Add(2*time.Minute).Unix(), 10)
	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", agentID.String(), key, future, "nonce-2", `{}`)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAgentAuthRejectsUnknownAgent(t *testing.T) {
	unknown := uuid.New()
	key, _ := agentsign.NewSigningKey()
	store := &stubAgentStore{agents: map[uuid.UUID]*domainagent.Agent{}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := chain(next, AgentAuth(store, agentsign.DefaultTimestampWindow))

	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", unknown.String(), key, agentsign.Timestamp(), "nonce-3", `{}`)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if want := "invalid_credentials"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestAgentAuthRejectsTamperedBody(t *testing.T) {
	agentID, key, h := newAuthFixture(t, agentsign.DefaultTimestampWindow)

	ts := agentsign.Timestamp()
	nonce := "nonce-4"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", strings.NewReader(`{"agent_id":"someone-else"}`))
	req.Header.Set(agentsign.HeaderAgentID, agentID.String())
	req.Header.Set(agentsign.HeaderAgentTimestamp, ts)
	req.Header.Set(agentsign.HeaderAgentNonce, nonce)
	// Signature computed over the intended body.
	req.Header.Set(agentsign.HeaderAgentSignature, agentsign.Sign(key, agentsign.Canonical(agentID.String(), ts, nonce, http.MethodPost, "/api/v1/agents/heartbeat", `{}`)))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if want := "invalid_signature"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestAgentAuthRejectsReplay(t *testing.T) {
	agentID, key, h := newAuthFixture(t, agentsign.DefaultTimestampWindow)

	ts := agentsign.Timestamp()
	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", agentID.String(), key, ts, "replay-nonce", `{}`)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay to be rejected, got %d", rec.Code)
	}
	if want := "replay_detected"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestAgentAuthAcceptsValidRequest(t *testing.T) {
	agentID, key, h := newAuthFixture(t, agentsign.DefaultTimestampWindow)

	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", agentID.String(), key, agentsign.Timestamp(), "nonce-5", `{"agent_id":"`+agentID.String()+`"}`)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp.Error.Code
}
