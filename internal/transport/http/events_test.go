package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agentsign"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
	"github.com/tsee9iii/opspilot/internal/notify"
)

// syncRecorder wraps an httptest recorder and surfaces every Flush on a
// channel, so a test can wait deterministically for the handler to have
// written and flushed an event without polling or sleeping.
type syncRecorder struct {
	*httptest.ResponseRecorder
	flushCh chan struct{}
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{ResponseRecorder: httptest.NewRecorder(), flushCh: make(chan struct{}, 32)}
}

func (r *syncRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.flushCh <- struct{}{}
}

func waitFlush(t *testing.T, r *syncRecorder) {
	t.Helper()
	select {
	case <-r.flushCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE flush")
	}
}

func assertFlushCount(t *testing.T, r *syncRecorder, want int) {
	t.Helper()
	got := 0
	for {
		select {
		case <-r.flushCh:
			got++
		default:
			if got != want {
				t.Fatalf("expected %d flushes, got %d", want, got)
			}
			return
		}
	}
}

// eventsRouterFixture builds a router with a real SSE events handler and a stub
// agent store containing the given agent.
func eventsRouterFixture(t *testing.T, agents map[uuid.UUID]*domainagent.Agent, n *notify.Notifier) http.Handler {
	t.Helper()
	return NewRouter(
		RouterDeps{
			Agents:        &stubAgentStore{agents: agents},
			OperatorToken: "op-token",
			Logger:        zap.NewNop(),
			Events:        NewAgentEventHandler(n),
		},
		NewAgentHandler(nil, nil, nil),
		NewCommandHandler(nil, nil, nil, nil, nil),
		NewCapabilityHandler(nil),
		NewHealthHandler(nil, nil),
		NewAlertHandler(nil, nil),
	)
}

func TestEventsRouteRequiresSignature(t *testing.T) {
	h := eventsRouterFixture(t, map[uuid.UUID]*domainagent.Agent{}, notify.New())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without signature, got %d", rec.Code)
	}
	if want := "invalid_signature"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestEventsRouteRejectsUnregisteredAgent(t *testing.T) {
	unknown := uuid.New()
	key, _ := agentsign.NewSigningKey()
	h := eventsRouterFixture(t, map[uuid.UUID]*domainagent.Agent{}, notify.New())

	req := signedRequest(t, http.MethodGet, "/api/v1/agents/events", unknown.String(), key, agentsign.Timestamp(), "ev-nonce-1", "")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unregistered agent, got %d", rec.Code)
	}
	if want := "invalid_credentials"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

// TestEventsRouteServesSSEForAuthenticatedAgent drives the endpoint through
// the full router: HMAC auth, SSE headers, the initial connected event, a
// wake-up event triggered by Notify, and a clean exit on request cancellation.
func TestEventsRouteServesSSEForAuthenticatedAgent(t *testing.T) {
	agentID := uuid.New()
	key, _ := agentsign.NewSigningKey()
	n := notify.New()
	defer n.Close()
	h := eventsRouterFixture(t, map[uuid.UUID]*domainagent.Agent{
		agentID: {ID: agentID, SigningKey: key},
	}, n)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := signedRequest(t, http.MethodGet, "/api/v1/agents/events", agentID.String(), key, agentsign.Timestamp(), "ev-nonce-2", "")
	req = req.WithContext(ctx)

	rec := newSyncRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	// The handler flushes once for the empty headers and once for the
	// connected event before blocking.
	waitFlush(t, rec)
	waitFlush(t, rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("unexpected Content-Type: %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("unexpected Cache-Control: %q", cc)
	}
	if c := rec.Header().Get("Connection"); c != "keep-alive" {
		t.Fatalf("unexpected Connection: %q", c)
	}
	if x := rec.Header().Get("X-Accel-Buffering"); x != "no" {
		t.Fatalf("unexpected X-Accel-Buffering: %q", x)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: connected") || !strings.Contains(body, `"status":"connected"`) {
		t.Fatalf("missing connected event in body: %q", body)
	}

	n.Notify(agentID.String())
	waitFlush(t, rec)
	body = rec.Body.String()
	if !strings.Contains(body, "event: wakeup") {
		t.Fatalf("missing wakeup event in body: %q", body)
	}
	if !strings.Contains(body, `"agent_id":"`+agentID.String()+`"`) || !strings.Contains(body, `"reason":"command_available"`) {
		t.Fatalf("wakeup payload malformed: %q", body)
	}
	if strings.Contains(body, "command_id") {
		t.Fatalf("SSE must not leak command ids: %q", body)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit on request cancellation")
	}
}

// TestEventsHandlerReplacedStreamEnds proves a newer stream for the same agent
// cleanly terminates the older handler.
func TestEventsHandlerReplacedStreamEnds(t *testing.T) {
	agentID := uuid.New()
	n := notify.New()
	defer n.Close()
	h := NewAgentEventHandler(n)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/events", nil)
	req.Header.Set(agentsign.HeaderAgentID, agentID.String())

	rec := newSyncRecorder()
	done := make(chan struct{})
	go func() {
		h.Events(rec, req)
		close(done)
	}()
	waitFlush(t, rec)
	waitFlush(t, rec)

	replacement := n.Subscribe(agentID.String())
	defer replacement.Unsubscribe()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("old SSE handler did not exit after stream replacement")
	}
}

// TestEventsHandlerExitsOnRequestCancel proves cancelling the request context
// ends the stream.
func TestEventsHandlerExitsOnRequestCancel(t *testing.T) {
	agentID := uuid.New()
	n := notify.New()
	defer n.Close()
	h := NewAgentEventHandler(n)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/events", nil).WithContext(ctx)
	req.Header.Set(agentsign.HeaderAgentID, agentID.String())

	rec := newSyncRecorder()
	done := make(chan struct{})
	go func() {
		h.Events(rec, req)
		close(done)
	}()
	waitFlush(t, rec)
	waitFlush(t, rec)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit on request cancellation")
	}
}

// TestEventsHandlerIgnoresOtherAgents proves a wake-up for a different agent
// never reaches this stream.
func TestEventsHandlerIgnoresOtherAgents(t *testing.T) {
	agentID := uuid.New()
	n := notify.New()
	defer n.Close()
	h := NewAgentEventHandler(n)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/events", nil).WithContext(ctx)
	req.Header.Set(agentsign.HeaderAgentID, agentID.String())

	rec := newSyncRecorder()
	done := make(chan struct{})
	go func() {
		h.Events(rec, req)
		close(done)
	}()
	waitFlush(t, rec)
	waitFlush(t, rec)

	n.Notify("other-agent")
	n.Notify(agentID.String())

	// Exactly one wakeup event (for this agent) must be flushed.
	waitFlush(t, rec)
	assertFlushCount(t, rec, 0)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit on request cancellation")
	}
}
