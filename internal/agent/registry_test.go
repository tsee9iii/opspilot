package agent_test

import (
	"context"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/tools/system"
)

func TestRegistryRegisterFindList(t *testing.T) {
	r := agent.NewRegistry()
	r.Register(system.NewUptimeTool())

	tool, ok := r.Find(system.ToolSystemUptime)
	if !ok {
		t.Fatal("expected tool to be found")
	}
	if tool.Name() != system.ToolSystemUptime {
		t.Fatalf("unexpected name: %s", tool.Name())
	}

	if _, ok := r.Find("nonexistent"); ok {
		t.Fatal("expected nonexistent tool to be missing")
	}

	names := r.List()
	if len(names) != 1 || names[0] != system.ToolSystemUptime {
		t.Fatalf("unexpected list: %v", names)
	}
}

func TestRegistryRegisterOverwrites(t *testing.T) {
	r := agent.NewRegistry()
	r.Register(system.NewUptimeTool())
	r.Register(&fakeTool{name: system.ToolSystemUptime})
	if len(r.List()) != 1 {
		t.Fatalf("expected single registration, got: %v", r.List())
	}
}

type fakeTool struct {
	name string
}

func (t *fakeTool) Name() string { return t.name }

func (t *fakeTool) Version() string { return "0.0.1" }

func (t *fakeTool) Description() string { return "fake tool" }

func (t *fakeTool) ParameterSchema() string { return agent.EmptyParameterSchema }

func (t *fakeTool) ConfirmationLevel() agent.ConfirmationLevel { return agent.ConfirmationNone }

func (t *fakeTool) Availability(_ context.Context) (bool, string) { return true, "" }

func (t *fakeTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	return []byte(`{"ok":true}`), nil
}
