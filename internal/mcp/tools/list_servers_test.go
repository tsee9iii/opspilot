package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

func TestListServersToolSuccess(t *testing.T) {
	id := uuid.New()
	repo := &fakeServerRepo{servers: []inventory.ServerSummary{
		{ID: id, Name: "edge-1", Hostname: "edge-1.example", Environment: "prod", Status: "unknown", AgentCount: 2, OnlineAgentCount: 1},
	}}
	tool := NewListServersTool(inventory.NewListServersUseCase(repo))

	out, err := tool.Call(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got struct {
		Servers []serverResult `json:"servers"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got.Servers))
	}
	s := got.Servers[0]
	if s.ID != id.String() || s.Name != "edge-1" || s.Hostname != "edge-1.example" ||
		s.Environment != "prod" || s.Status != "unknown" || s.AgentCount != 2 || s.OnlineAgentCount != 1 {
		t.Fatalf("unexpected server: %+v", s)
	}
}

func TestListServersToolPropagatesError(t *testing.T) {
	tool := NewListServersTool(inventory.NewListServersUseCase(&fakeServerRepo{err: errors.New("db down")}))
	_, err := tool.Call(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	var te *mcp.ToolError
	if errors.As(err, &te) {
		t.Fatalf("generic repo errors surface as internal_error in the server, not as ToolError: %v", err)
	}
}

func TestListServersToolMetadata(t *testing.T) {
	tool := NewListServersTool(inventory.NewListServersUseCase(&fakeServerRepo{}))
	if tool.Name() != "list_servers" || tool.Description() == "" {
		t.Fatalf("unexpected metadata: %s %q", tool.Name(), tool.Description())
	}
	for _, schema := range []json.RawMessage{tool.InputSchema(), tool.OutputSchema()} {
		var raw any
		if err := json.Unmarshal(schema, &raw); err != nil {
			t.Fatalf("invalid schema: %v", err)
		}
	}
}
