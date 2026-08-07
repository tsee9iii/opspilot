package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agentsign"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

// createRouterFixture builds a router whose command handler is backed by a real
// CreateUseCase with stub persistence, so a successfully authenticated request
// provably reaches the handler.
func createRouterFixture(t *testing.T, token string) http.Handler {
	t.Helper()
	return NewRouter(
		RouterDeps{
			Agents:        &stubAgentStore{agents: map[uuid.UUID]*domainagent.Agent{}},
			OperatorToken: token,
			Logger:        zap.NewNop(),
		},
		NewAgentHandler(nil, nil, nil),
		NewCommandHandler(appcommand.NewCreateUseCase(&stubCommandRepo{}, &stubConfirmationResolver{}), nil, nil, nil, nil),
		NewCapabilityHandler(nil),
		NewHealthHandler(nil, nil),
		NewAlertHandler(nil, nil),
	)
}

// stubCommandRepo implements command.Repository with a create that always
// rejects with a validation error, so "reached the handler" is observable as a
// 400 rather than a panic or 401.
type stubCommandRepo struct{}

func (stubCommandRepo) CreateCommand(_ context.Context, _ appcommand.CreateCommandRequest) (appcommand.CreateCommandResponse, error) {
	return appcommand.CreateCommandResponse{}, appcommand.ErrInvalidAgentID
}
func (stubCommandRepo) LeaseNextCommand(_ context.Context, _ appcommand.LeaseCommandRequest) (appcommand.LeaseCommandResponse, error) {
	return appcommand.LeaseCommandResponse{}, nil
}
func (stubCommandRepo) StartCommand(_ context.Context, _ appcommand.StartCommandRequest) (appcommand.StartCommandResponse, error) {
	return appcommand.StartCommandResponse{}, nil
}
func (stubCommandRepo) CompleteCommand(_ context.Context, _ appcommand.CompleteCommandRequest) (appcommand.CompleteCommandResponse, error) {
	return appcommand.CompleteCommandResponse{}, nil
}
func (stubCommandRepo) FailCommand(_ context.Context, _ appcommand.FailCommandRequest) (appcommand.FailCommandResponse, error) {
	return appcommand.FailCommandResponse{}, nil
}
func (stubCommandRepo) ApproveCommand(_ context.Context, _ appcommand.ApproveCommandRequest) (appcommand.ApproveCommandResponse, error) {
	return appcommand.ApproveCommandResponse{}, nil
}
func (stubCommandRepo) GetCommand(_ context.Context, _ appcommand.GetCommandRequest) (appcommand.GetCommandResponse, error) {
	return appcommand.GetCommandResponse{}, nil
}

type stubConfirmationResolver struct{}

func (stubConfirmationResolver) ConfirmationLevel(context.Context, uuid.UUID, string) (string, error) {
	return "none", nil
}

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
		NewHealthHandler(nil, nil),
		NewAlertHandler(nil, nil),
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
		httptest.NewRequest(http.MethodGet, "/api/v1/health", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/alerts/00000000-0000-0000-0000-000000000001/acknowledge", strings.NewReader(`{}`)),
	}
	for _, req := range requests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401 without token, got %d", req.Method, req.URL.Path, rec.Code)
		}
	}
}

// TestRouterOperatorRoutesRequireActor proves every authenticated operator
// action must carry an X-Operator-Actor header: actor attribution is enforced
// after bearer auth and never optional.
func TestRouterOperatorRoutesRequireActor(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/commands/00000000-0000-0000-0000-000000000001", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/commands/approve", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/health", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/alerts/00000000-0000-0000-0000-000000000001/acknowledge", strings.NewReader(`{}`)),
	}
	for _, req := range requests {
		req.Header.Set("Authorization", "Bearer op-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: expected 400 without actor header, got %d", req.Method, req.URL.Path, rec.Code)
		}
	}
}

func TestRouterOperatorRoutesAcceptCorrectToken(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/00000000-0000-0000-0000-000000000001", nil)
	req.Header.Set("Authorization", "Bearer op-token")
	req.Header.Set("X-Operator-Actor", "operator@example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// The operator and actor boundaries passed; the nil-backed Get handler must
	// not panic to a 500, but more importantly auth must not answer 401/400.
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusBadRequest {
		t.Fatalf("expected token+actor request to pass the operator boundary, got %d", rec.Code)
	}
}

func TestRouterCreateCommandRequiresOperatorAuth(t *testing.T) {
	h := createRouterFixture(t, "op-token")
	body := strings.NewReader(`{"agent_id":"00000000-0000-0000-0000-000000000001","tool":"system.uptime","payload":{}}`)

	t.Run("no bearer token is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/commands", body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 without token, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "op-token") {
			t.Fatal("auth failure must not leak the operator token")
		}
	})

	t.Run("invalid bearer token is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/commands", body)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for invalid token, got %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "wrong-token") || strings.Contains(rec.Body.String(), "op-token") {
			t.Fatal("auth failure must not leak token values")
		}
	})

	t.Run("valid bearer token reaches the handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/commands", body)
		req.Header.Set("Authorization", "Bearer op-token")
		req.Header.Set("X-Operator-Actor", "operator@example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		// Auth and actor attribution passed and the stub create handler ran
		// (validation rejected the all-zero agent UUID), proving the request
		// crossed the operator boundary instead of being answered by the
		// middleware.
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 from handler (auth passed), got %d", rec.Code)
		}
	})
}

func TestRouterAgentRoutesRequireSignature(t *testing.T) {
	h := newRouterFixture(t, "op-token")

	requests := []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", strings.NewReader(`{}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/agents/health", strings.NewReader(`{}`)),
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
		NewHealthHandler(nil, nil),
		NewAlertHandler(nil, nil),
	)

	req := signedRequest(t, http.MethodPost, "/api/v1/agents/heartbeat", agentID.String(), key, agentsign.Timestamp(), "router-nonce", `{"agent_id":"`+agentID.String()+`"}`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatal("expected signed request to pass the agent auth middleware")
	}
}
