package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agentsign"
)

// waitFor polls cond until it returns true or the deadline expires. It keeps
// tests deterministic (no fixed sleeps) and fails fast on the rare hang.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

// sseTestServer is a fake central that serves the agent's endpoints and
// records calls.
type sseTestServer struct {
	mu     sync.Mutex
	srv    *httptest.Server
	events int
	leases int
}

func newSSETestServer() *sseTestServer {
	st := &sseTestServer{}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/events":
			st.mu.Lock()
			st.events++
			st.mu.Unlock()
			flusher, _ := w.(http.Flusher)
			_, _ = fmt.Fprint(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			// Hold the stream open until the client disconnects.
			<-r.Context().Done()
			return
		case "/api/v1/capabilities":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/agents/heartbeat":
			_, _ = fmt.Fprint(w, `{"status":"ok","next_heartbeat":30}`)
		case "/api/v1/commands/lease":
			st.mu.Lock()
			st.leases++
			n := st.leases
			st.mu.Unlock()
			if n > 1 {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"command_id":"c1","tool":"spy","payload":{}}`)
		case "/api/v1/commands/start", "/api/v1/commands/complete", "/api/v1/commands/fail":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	return st
}

func (st *sseTestServer) Close() { st.srv.Close() }

func (st *sseTestServer) eventsCalls() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.events
}

// TestConsumeSSEWakeupSignalsChannel proves a `event: wakeup` from central
// produces a wake signal for the command loop.
func TestConsumeSSEWakeupSignalsChannel(t *testing.T) {
	var srv *httptest.Server
	wake := make(chan struct{}, 1)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "event: wakeup\ndata: {\"agent_id\":\"a1\",\"reason\":\"command_available\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), nil, NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = a.consumeSSE(ctx, wake)
		close(done)
	}()

	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("wakeup event did not signal the wake channel")
	}
	cancel()
	<-done
}

// TestConsumeSSEIgnoresNoise proves connected events, comments and unknown
// events never signal a wake.
func TestConsumeSSEIgnoresNoise(t *testing.T) {
	var srv *httptest.Server
	wake := make(chan struct{}, 1)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w,
			": ping\n\n"+
				"event: connected\ndata: {\"status\":\"connected\"}\n\n"+
				"event: some_other\ndata: {\"x\":1}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), nil, NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = a.consumeSSE(ctx, wake)
		close(done)
	}()

	select {
	case <-wake:
		t.Fatal("noise events must not signal a wake")
	case <-time.After(300 * time.Millisecond):
	}
	cancel()
	<-done
}

// TestWakeTriggersImmediateLeaseAttempt proves a wake-up causes the command
// loop to poll (and execute) immediately instead of waiting for the fallback
// interval, which is set to an hour here so only the wake can trigger it.
func TestWakeTriggersImmediateLeaseAttempt(t *testing.T) {
	var mu sync.Mutex
	var leaseCalls int
	executed := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/commands/lease":
			mu.Lock()
			leaseCalls++
			n := leaseCalls
			mu.Unlock()
			if n == 1 {
				// The initial timer poll finds nothing.
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"command_id":"c1","tool":"spy","payload":{}}`)
		case "/api/v1/commands/start":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/commands/complete":
			var req reportRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			executed <- req.CommandID
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	if err := reg.Register(&spyTool{name: "spy", schema: `{"type":"object"}`}); err != nil {
		t.Fatalf("register: %v", err)
	}
	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true}), reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	go a.pollCommands(ctx, time.Hour, wake)

	// The initial timer poll runs once and finds nothing.
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return leaseCalls >= 1 }, "initial poll never ran")

	signalWake(wake)

	select {
	case id := <-executed:
		if id != "c1" {
			t.Fatalf("expected command c1 to execute, got %q", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wake-up did not trigger an immediate lease and execution")
	}
}

// blockingSpyTool implements a Tool whose execution blocks until release.
type blockingSpyTool struct {
	name    string
	schema  string
	release chan struct{}

	mu         sync.Mutex
	executions int
	active     int
	maxActive  int
}

func (t *blockingSpyTool) Name() string                         { return t.name }
func (t *blockingSpyTool) Version() string                      { return "0.0.1" }
func (t *blockingSpyTool) Description() string                  { return "blocking spy tool" }
func (t *blockingSpyTool) ParameterSchema() string              { return t.schema }
func (t *blockingSpyTool) ConfirmationLevel() ConfirmationLevel { return ConfirmationNone }
func (t *blockingSpyTool) Metadata() ToolMetadata {
	return ToolMetadata{
		Name:              t.Name(),
		Description:       t.Description(),
		Category:          CategorySystem,
		Tags:              []string{"test"},
		Risk:              RiskReadOnly,
		EstimatedDuration: DurationShort,
	}
}
func (t *blockingSpyTool) Availability(context.Context) (bool, string) { return true, "" }

func (t *blockingSpyTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	t.mu.Lock()
	t.executions++
	t.active++
	if t.active > t.maxActive {
		t.maxActive = t.active
	}
	t.mu.Unlock()
	<-t.release
	t.mu.Lock()
	t.active--
	t.mu.Unlock()
	return []byte(`{"ok":true}`), nil
}

// TestDuplicateWakesDoNotRunConcurrentExecutions proves wakes coalesce to one
// pending signal and the command loop never runs two executions at once: a
// wake during an active execution is queued, and the follow-up poll happens
// only after execution completes.
func TestDuplicateWakesDoNotRunConcurrentExecutions(t *testing.T) {
	tool := &blockingSpyTool{name: "spy", schema: `{"type":"object"}`, release: make(chan struct{})}

	var mu sync.Mutex
	var leaseCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/commands/lease":
			mu.Lock()
			leaseCalls++
			n := leaseCalls
			mu.Unlock()
			if n > 1 {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"command_id":"c1","tool":"spy","payload":{}}`)
		case "/api/v1/commands/start", "/api/v1/commands/complete":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	if err := reg.Register(tool); err != nil {
		t.Fatalf("register: %v", err)
	}
	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true}), reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	go a.pollCommands(ctx, time.Hour, wake)

	// First (initial) poll leases c1 and blocks inside execution.
	waitFor(t, func() bool {
		tool.mu.Lock()
		defer tool.mu.Unlock()
		return tool.active == 1
	}, "execution never started")

	// Two wakes during an active execution must coalesce to one pending signal.
	signalWake(wake)
	signalWake(wake)

	close(tool.release)

	// After execution completes the single coalesced wake triggers exactly one
	// follow-up poll.
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return leaseCalls >= 2 }, "follow-up poll never ran")
	time.Sleep(50 * time.Millisecond)

	tool.mu.Lock()
	executions, maxActive := tool.executions, tool.maxActive
	tool.mu.Unlock()
	if executions != 1 {
		t.Fatalf("expected exactly 1 execution, got %d", executions)
	}
	if maxActive != 1 {
		t.Fatalf("expected at most 1 concurrent execution, saw %d", maxActive)
	}
	mu.Lock()
	got := leaseCalls
	mu.Unlock()
	if got != 2 {
		t.Fatalf("expected 2 lease calls (coalesced wakes), got %d", got)
	}
}

// TestSSEListenerReconnectsAfterDisconnect proves the listener reconnects with
// a small injected backoff after the stream drops, and stops cleanly on
// shutdown.
func TestSSEListenerReconnectsAfterDisconnect(t *testing.T) {
	var mu sync.Mutex
	connects := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/events" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		connects++
		mu.Unlock()
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "event: connected\ndata: {}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Simulate central dropping the stream: hijack and close immediately.
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), nil, NewRegistry())
	a.sseInitialBackoff = time.Millisecond
	a.sseMaxBackoff = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	wake := make(chan struct{}, 1)
	go func() {
		a.runSSEListener(ctx, wake)
		close(done)
	}()

	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return connects >= 3 }, "listener did not reconnect")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE listener did not stop on shutdown")
	}
}

// TestSSEBackoffIsBounded proves the reconnect backoff grows from the initial
// value, is jitter-bounded below the maximum, and never exceeds it.
func TestSSEBackoffIsBounded(t *testing.T) {
	const max = 30 * time.Second
	d := 1 * time.Second
	seenMax := false
	for i := 0; i < 20; i++ {
		if d <= 0 || d > max {
			t.Fatalf("backoff %v out of bounds (max %v)", d, max)
		}
		if d >= time.Duration(float64(max)*(1-sseJitterFraction)) {
			seenMax = true
		}
		d = nextSSEBackoff(d, max)
	}
	if !seenMax {
		t.Fatal("backoff never reached the maximum band")
	}
}

// agentWithSpyTool builds a registry with a working "spy" tool and an agent
// wired to it, so command execution succeeds against the fake central.
func agentWithSpyTool(t *testing.T, cfg *Config) *Agent {
	t.Helper()
	reg := NewRegistry()
	if err := reg.Register(&spyTool{name: "spy", schema: `{"type":"object"}`}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return New(cfg, zap.NewNop(), NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true}), reg)
}

// TestSSEDisabledUsesFallbackPolling proves that with sse_enabled=false the
// agent never connects to the events endpoint but the fallback poll still runs.
func TestSSEDisabledUsesFallbackPolling(t *testing.T) {
	st := newSSETestServer()
	defer st.Close()

	disabled := false
	a := agentWithSpyTool(t, &Config{
		CentralURL:   st.srv.URL,
		AgentID:      "a1",
		PollInterval: 1,
		SSEEnabled:   &disabled,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = a.Run(ctx)
		close(done)
	}()

	waitFor(t, func() bool {
		st.mu.Lock()
		defer st.mu.Unlock()
		return st.leases >= 2
	}, "fallback polling never ran")

	if st.eventsCalls() != 0 {
		t.Fatal("SSE listener connected despite sse_enabled=false")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("agent Run did not stop on shutdown")
	}
}

// TestRunStartsAndStopsSSEListener proves the SSE-enabled Run connects to the
// events endpoint and stops the listener cleanly on shutdown.
func TestRunStartsAndStopsSSEListener(t *testing.T) {
	st := newSSETestServer()
	defer st.Close()

	a := agentWithSpyTool(t, &Config{CentralURL: st.srv.URL, AgentID: "a1"})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = a.Run(ctx)
		close(done)
	}()

	waitFor(t, func() bool { return st.eventsCalls() >= 1 }, "SSE listener never connected")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("agent Run did not stop on shutdown")
	}
}

// TestGETRequestSignaturesWork proves the HMAC signing helpers sign GET
// requests with an empty body correctly, as required by the SSE stream.
func TestGETRequestSignaturesWork(t *testing.T) {
	const (
		agentID = "agent-1"
		key     = "test-signing-key"
	)
	verified := make(chan bool, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(agentsign.HeaderAgentID)
		ts := r.Header.Get(agentsign.HeaderAgentTimestamp)
		nonce := r.Header.Get(agentsign.HeaderAgentNonce)
		sig := r.Header.Get(agentsign.HeaderAgentSignature)
		canonical := agentsign.Canonical(id, ts, nonce, r.Method, r.URL.Path, "")
		verified <- id == agentID && sig != "" && agentsign.Verify(key, sig, canonical)
	}))
	defer srv.Close()

	a := New(&Config{CentralURL: srv.URL, AgentID: agentID, SigningKey: key}, zap.NewNop(), nil, NewRegistry())

	resp, err := a.openSSEStream(context.Background())
	if err != nil {
		t.Fatalf("open SSE stream: %v", err)
	}
	resp.Body.Close()

	if !<-verified {
		t.Fatal("GET request signature was invalid")
	}
}
