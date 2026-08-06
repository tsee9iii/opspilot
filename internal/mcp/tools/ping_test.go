package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tsee9iii/opspilot/internal/mcp"
	"github.com/tsee9iii/opspilot/pkg/version"
)

type fakePinger struct {
	err error
}

func (f *fakePinger) Ping(context.Context) error { return f.err }

func TestPingToolHealthConnected(t *testing.T) {
	tool := NewPingTool(&fakePinger{})
	out, err := tool.Call(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got mcp.Health
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if got.Service != mcp.ServiceName {
		t.Fatalf("unexpected service: %q", got.Service)
	}
	if got.Version != version.MCP {
		t.Fatalf("unexpected version: %q", got.Version)
	}
	if got.Protocol != "2025-03-26" {
		t.Fatalf("unexpected protocol: %q", got.Protocol)
	}
	if got.CentralVersion != version.Central {
		t.Fatalf("unexpected central version: %q", got.CentralVersion)
	}
	if got.Database != mcp.DatabaseConnected {
		t.Fatalf("unexpected database status: %q", got.Database)
	}
	if got.UptimeSeconds < 0 {
		t.Fatalf("uptime must be non-negative, got %d", got.UptimeSeconds)
	}
}

func TestPingToolHealthDisconnected(t *testing.T) {
	for _, pinger := range []mcp.Pinger{nil, &fakePinger{err: errors.New("db down")}} {
		tool := NewPingTool(pinger)
		out, err := tool.Call(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got mcp.Health
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("decode health: %v", err)
		}
		if got.Database != mcp.DatabaseDisconnected {
			t.Fatalf("expected disconnected, got %q", got.Database)
		}
	}
}

func TestPingToolMetadata(t *testing.T) {
	tool := NewPingTool(nil)
	if tool.Name() != "ping" || tool.Description() == "" || tool.Category() != CategorySystem {
		t.Fatalf("unexpected metadata: %s %q %q", tool.Name(), tool.Description(), tool.Category())
	}
	for _, schema := range []json.RawMessage{tool.InputSchema(), tool.OutputSchema()} {
		var raw any
		if err := json.Unmarshal(schema, &raw); err != nil {
			t.Fatalf("invalid schema: %v", err)
		}
	}
}
