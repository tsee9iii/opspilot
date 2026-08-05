package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

type availTool struct {
	name   string
	ok     bool
	reason string
}

func (t *availTool) Name() string { return t.name }

func (t *availTool) Version() string { return "1.0.0" }

func (t *availTool) Description() string { return "availability test tool" }

func (t *availTool) ParameterSchema() string { return EmptyParameterSchema }

func (t *availTool) ConfirmationLevel() ConfirmationLevel { return ConfirmationNone }

func (t *availTool) Availability(_ context.Context) (bool, string) { return t.ok, t.reason }

func (t *availTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	return []byte(`{}`), nil
}

func TestRegisterCapabilitiesIncludesAvailability(t *testing.T) {
	var captured syncCapabilitiesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reg := NewRegistry()
	reg.Register(&availTool{name: "ok.tool", ok: true})
	reg.Register(&availTool{name: "bad.tool", ok: false, reason: "docker is not installed"})

	a := New(&Config{CentralURL: srv.URL, AgentID: "a1"}, zap.NewNop(), NewRegistryExecutor(reg, ExecutionPolicy{Enabled: true}), reg)
	if err := a.registerCapabilities(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(captured.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(captured.Capabilities))
	}
	byName := make(map[string]capabilityInfo)
	for _, c := range captured.Capabilities {
		byName[c.ToolName] = c
	}

	okEntry, present := byName["ok.tool"]
	if !present {
		t.Fatal("ok.tool not found in payload")
	}
	if !okEntry.Available || okEntry.UnavailableReason != "" {
		t.Fatalf("ok.tool: expected available with empty reason, got %+v", okEntry)
	}

	bad := byName["bad.tool"]
	if bad.Available || bad.UnavailableReason != "docker is not installed" {
		t.Fatalf("bad.tool: expected unavailable with reason, got %+v", bad)
	}

	for _, c := range captured.Capabilities {
		if c.ToolName == "" || c.Version == "" || c.Description == "" {
			t.Fatalf("capability missing metadata: %+v", c)
		}
	}
}
