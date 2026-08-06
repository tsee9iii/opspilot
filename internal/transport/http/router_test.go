package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agentsign"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

// newRouterFixture builds the real router wiring with stub middleware
// dependencies and nil-backed handlers. Authenticated endpoints reject before
// a handler runs, so only the auth boundaries are exercised here.
func newRouterFixture(t *testing.T, token string) http.Handler {
	t.Helper()
	return NewRouter(
		RouterDeps{
			Agents:        &stubAgentStore{agents: map[uuid.UUID]*domainagent.Agent{}},
			OperatorToken: token,
			Logger:        zap.NewNop(),
		},
		NewAgentHandler(nil, nil, nil),
		NewCommandHandler(nil, nil, nil, nil, nil),
		NewCapabilityHandler(nil),
	)
}

func TestRouterHealthzIsUnauthenticated(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRouterRegisterIsUnauthenticated(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// No signing headers and no operator token: if the route were behind auth
	// the middleware would answer 401 before the handler. A 400 means the
	// handler itself rejected the empty body, proving the route is open.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (auth bypassed, handler validation), got %d", rec.Code)
	}
}

func TestRouterOperatorRoutesRequireToken(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/commands/00000000-0000-0000-0000-000000000001", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/commands/approve", strings.NewReader(`{}`)),
	}
	for _, req := range requests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401 without token, got %d", req.Method, req.URL.Path, rec.Code)
		}
	}
}

func TestRouterOperatorRoutesAcceptCorrectToken(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/00000000-0000-0000-0000-000000000001", nil)
	req.Header.Set("Authorization", "Bearer op-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// The operator boundary passed; the nil-backed Get handler must not panic
	// to a 500, but more importantly auth must not answer 401.
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("expected token-authenticated request to pass the operator boundary")
	}
}

func TestRouterAgentRoutesRequireSignature(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	requests := []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/agents/unregister", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/commands/lease", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/commands/start", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/commands/complete", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/commands/fail", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/capabilities", strings.NewReader(`{}`)),
	}
	for _, req := range requests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401 without signature, got %d", req.Method, req.URL.Path, rec.Code)
		}
	}
}

func TestRouterAgentRoutesAcceptSignedRequest(t *testing.T) {
	agentID := uuid.New()
	key, _ := agentsign.NewSigningKey()
	store := &stubAgentStore{agents: map[uuid.UUID]*domainagent.Agent{
		agentID: {ID: agentID, SigningKey: key},
	}}
	h := NewRouter(
		RouterDeps{
			Agents:        store,
			OperatorToken: "op-token",
			Logger:        zap.NewNop(),
		},
		NewAgentHandler(nil, nil, nil),
		NewCommandHandler(nil, nil, nil, nil, nil),
		NewCapabilityHandler(nil),
	)

	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", agentID.String(), key, agentsign.Timestamp(), "router-nonce", `{"agent_id":"`+agentID.String()+`"}`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatal("expected signed request to pass the agent auth middleware")
	}
}
