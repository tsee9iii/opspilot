package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
)

type spyTool struct {
	name     string
	schema   string
	executed bool
	mu       sync.Mutex
}

func (t *spyTool) Name() string { return t.name }

func (t *spyTool) Version() string { return "0.0.1" }

func (t *spyTool) Description() string { return "spy tool" }

func (t *spyTool) ParameterSchema() string { return t.schema }

func (t *spyTool) ConfirmationLevel() ConfirmationLevel { return ConfirmationNone }

func (t *spyTool) Availability(_ context.Context) (bool, string) { return true, "" }

func (t *spyTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.executed = true
	return []byte(`{"ok":true}`), nil
}

func (t *spyTool) wasExecuted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.executed
}

func TestPollOnceValidationFailureFailsCommand(t *testing.T) {
	tool := &spyTool{
		name:   "spy",
		schema: `{"type":"object","properties":{"lines":{"type":"integer"}},"required":["lines"]}`,
	}

	var mu sync.Mutex
	var started, failed bool
	var failErr string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/commands/lease":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"command_id":"c1","tool":"spy","payload":{"lines":"50"}}`))
		case "/api/v1/commands/start":
			mu.Lock()
			started = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case "/api/v1/commands/fail":
			var req reportRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			failed = true
			failErr = req.Error
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	reg.Register(tool)

	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true}), reg)

	if err := a.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !started {
		t.Fatal("start was not called")
	}
	if !failed {
		t.Fatal("fail was not called")
	}
	if !strings.Contains(failErr, `property "lines" must be integer`) {
		t.Fatalf("unexpected fail error: %q", failErr)
	}
	if tool.wasExecuted() {
		t.Fatal("tool executed despite invalid payload")
	}
}
