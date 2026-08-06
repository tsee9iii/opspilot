package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

func TestListAgentsToolSuccess(t *testing.T) {
	id := uuid.New()
	hb := time.Now().UTC()
	repo := &fakeAgentRepo{agents: []inventory.AgentSummary{
		{ID: id, ServerID: uuid.New(), ServerName: "edge-1", Hostname: "edge-1.example", Environment: "prod", Version: "2.1.0", Status: "online", LastHeartbeat: &hb},
	}}
	tool := NewListAgentsTool(inventory.NewListAgentsUseCase(repo))

	out, err := tool.Call(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got struct {
		Agents []agentResult `json:"agents"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(got.Agents))
	}
	a := got.Agents[0]
	if a.ID != id.String() || a.Server != "edge-1" || a.Hostname != "edge-1.example" ||
		a.Environment != "prod" || a.Version != "2.1.0" || a.Status != "online" || a.LastHeartbeat == nil {
		t.Fatalf("unexpected agent: %+v", a)
	}
}

func TestListAgentsToolNeverProjectsSecrets(t *testing.T) {
	repo := &fakeAgentRepo{agents: []inventory.AgentSummary{{ID: uuid.New(), ServerName: "s", Status: "offline"}}}
	tool := NewListAgentsTool(inventory.NewListAgentsUseCase(repo))
	out, err := tool.Call(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) == "" {
		t.Fatal("empty output")
	}
	// The agent summary has no secret field by construction; assert no
	// secret-like key appears in the serialized output.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var agents []map[string]any
	if err := json.Unmarshal(raw["agents"], &agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	for key := range agents[0] {
		switch key {
		case "secret", "secret_hash", "token":
			t.Fatalf("secret field leaked: %s", key)
		}
	}
}

func TestListAgentsToolForwardsFilters(t *testing.T) {
	repo := &fakeAgentRepo{}
	tool := NewListAgentsTool(inventory.NewListAgentsUseCase(repo))
	serverID := uuid.New()

	if _, err := tool.Call(context.Background(), map[string]any{
		"server_id": serverID.String(),
		"status":    "offline",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got.ServerID == nil || *repo.got.ServerID != serverID || repo.got.Status != "offline" {
		t.Fatalf("filters not forwarded: %+v", repo.got)
	}
}

func TestListAgentsToolInvalidServerID(t *testing.T) {
	tool := NewListAgentsTool(inventory.NewListAgentsUseCase(&fakeAgentRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"server_id": "not-a-uuid"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestListAgentsToolInvalidStatusType(t *testing.T) {
	tool := NewListAgentsTool(inventory.NewListAgentsUseCase(&fakeAgentRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"status": 42})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestListAgentsToolMapsInvalidServerIDDomainError(t *testing.T) {
	zero := uuid.Nil
	repo := &fakeAgentRepo{err: inventory.ErrInvalidServerID}
	repo.got = inventory.ListAgentsRequest{ServerID: &zero}
	tool := NewListAgentsTool(inventory.NewListAgentsUseCase(repo))
	_, err := tool.Call(context.Background(), map[string]any{"server_id": uuid.New().String()})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_server_id" {
		t.Fatalf("expected invalid_server_id, got: %v", err)
	}
}
