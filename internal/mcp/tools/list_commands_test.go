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

func TestListCommandsToolSuccess(t *testing.T) {
	id := uuid.New()
	created := time.Now().UTC()
	repo := &fakeCommandRepo{commands: []inventory.CommandSummary{
		{ID: id, AgentID: uuid.New(), Tool: "system.uptime", Status: "completed", CreatedAt: created},
	}}
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(repo))

	out, err := tool.Call(context.Background(), map[string]any{"limit": 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got struct {
		Commands []commandSummaryResult `json:"commands"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(got.Commands))
	}
	c := got.Commands[0]
	if c.ID != id.String() || c.Tool != "system.uptime" || c.Status != "completed" || c.CreatedAt != created.Format(time.RFC3339) {
		t.Fatalf("unexpected command: %+v", c)
	}
}

func TestListCommandsToolNoPayloadOrResult(t *testing.T) {
	repo := &fakeCommandRepo{commands: []inventory.CommandSummary{{ID: uuid.New(), Tool: "x", Status: "completed", CreatedAt: time.Now()}}}
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(repo))
	out, err := tool.Call(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(raw["commands"], &commands); err != nil {
		t.Fatalf("decode commands: %v", err)
	}
	for key := range commands[0] {
		switch key {
		case "payload", "result", "parameters":
			t.Fatalf("command summary leaked %s", key)
		}
	}
}

func TestListCommandsToolForwardsFilters(t *testing.T) {
	repo := &fakeCommandRepo{}
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(repo))
	agentID := uuid.New()

	if _, err := tool.Call(context.Background(), map[string]any{
		"status":   "failed",
		"agent_id": agentID.String(),
		"limit":    5,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got.Status != "failed" || repo.got.AgentID == nil || *repo.got.AgentID != agentID || repo.got.Limit != 5 {
		t.Fatalf("filters not forwarded: %+v", repo.got)
	}
}

func TestListCommandsToolDefaultsLimit(t *testing.T) {
	repo := &fakeCommandRepo{}
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(repo))
	if _, err := tool.Call(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got.Limit != inventory.DefaultLimit {
		t.Fatalf("expected default limit, got %d", repo.got.Limit)
	}
}

func TestListCommandsToolInvalidAgentID(t *testing.T) {
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(&fakeCommandRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestListCommandsToolInvalidLimitType(t *testing.T) {
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(&fakeCommandRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"limit": "fifty"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestListCommandsToolMapsInvalidLimitDomainError(t *testing.T) {
	repo := &fakeCommandRepo{err: inventory.ErrInvalidLimit}
	tool := NewListCommandsTool(inventory.NewListCommandsUseCase(repo))
	_, err := tool.Call(context.Background(), map[string]any{"limit": 9999})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_limit" {
		t.Fatalf("expected invalid_limit, got: %v", err)
	}
}
