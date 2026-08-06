package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	appcapability "github.com/tsee9iii/opspilot/internal/application/capability"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	appdispatch "github.com/tsee9iii/opspilot/internal/application/dispatch"
	appinventory "github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
	"github.com/tsee9iii/opspilot/internal/mcp/tools"
)

// TestMCPToolsEndToEnd drives the MCP server over stdio against the real
// repository stack: tool -> use case -> repository -> database. It verifies
// the milestone guarantees: no HTTP involved, stable machine-readable JSON,
// read-only tools never leak secrets/payloads/results, and execution tools
// keep awaiting approval instead of bypassing the confirmation flow.
func TestMCPToolsEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)

	server := seedServer(t, ctx, pool, "edge-1", "edge-1.example", "prod")
	agent := seedAgentOn(t, ctx, pool, server, "online", mustTimePtr(time.Now()))
	commandID := insertCompletedCommand(t, ctx, pool, agent, "system.uptime")

	// Wire the same composition root as cmd/mcp.
	commandRepo := NewCommandRepository(pool)
	capabilityRepo := NewCapabilityRepository(pool)
	inventoryRepo := NewInventoryRepository(pool)

	// The execution tool requires operator confirmation: register workflow.diagnose
	// as a confirmation-required capability so the created command stays pending.
	if err := capabilityRepo.Upsert(ctx, agent, appcapability.Capability{
		ToolName: "workflow.diagnose", Version: "1.0.0", Description: "diagnose",
		ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "required",
	}); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	createUC := appcommand.NewCreateUseCase(commandRepo, capabilityRepo)
	getUC := appcommand.NewGetCommandUseCase(commandRepo)
	dispatchUC := appdispatch.NewDispatchUseCase(createUC, getUC)

	toolSet := tools.Build(tools.Dependencies{
		Servers:    appinventory.NewListServersUseCase(inventoryRepo),
		Agents:     appinventory.NewListAgentsUseCase(inventoryRepo),
		Commands:   appinventory.NewListCommandsUseCase(inventoryRepo),
		GetCommand: getUC,
		Dispatch:   dispatchUC,
	})

	call := func(name string, args map[string]any) callOutput {
		payload, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": name, "arguments": args},
		})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		payload = append(payload, '\n')

		var out bytes.Buffer
		srv := mcp.NewServer(toolSet, bytes.NewReader(payload), &out)
		if err := srv.Run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}
		return decodeCallOutput(t, out.Bytes())
	}

	t.Run("list_servers over stdio", func(t *testing.T) {
		res := call("list_servers", map[string]any{})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		var got struct {
			Servers []struct {
				ID               string `json:"id"`
				Name             string `json:"name"`
				AgentCount       int64  `json:"agent_count"`
				OnlineAgentCount int64  `json:"online_agent_count"`
				Secret           string `json:"secret,omitempty"`
			} `json:"servers"`
		}
		if err := json.Unmarshal([]byte(res.Text), &got); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if len(got.Servers) != 1 || got.Servers[0].Name != "edge-1" {
			t.Fatalf("unexpected servers: %+v", got.Servers)
		}
		s := got.Servers[0]
		if s.AgentCount != 1 || s.OnlineAgentCount != 1 {
			t.Fatalf("unexpected totals: %+v", s)
		}
		if s.Secret != "" {
			t.Fatal("list_servers must not expose secrets")
		}
	})

	t.Run("list_agents never leaks the agent secret", func(t *testing.T) {
		res := call("list_agents", map[string]any{})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		var got struct {
			Agents []map[string]any `json:"agents"`
		}
		if err := json.Unmarshal([]byte(res.Text), &got); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if len(got.Agents) != 1 {
			t.Fatalf("expected 1 agent, got %d", len(got.Agents))
		}
		if _, ok := got.Agents[0]["secret"]; ok {
			t.Fatal("list_agents must not expose the agent secret")
		}
	})

	t.Run("list_commands is lightweight (no payload/result)", func(t *testing.T) {
		res := call("list_commands", map[string]any{})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		var got struct {
			Commands []map[string]any `json:"commands"`
		}
		if err := json.Unmarshal([]byte(res.Text), &got); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if len(got.Commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(got.Commands))
		}
		row := got.Commands[0]
		for _, leaked := range []string{"payload", "result", "error"} {
			if _, ok := row[leaked]; ok {
				t.Fatalf("list_commands must not include %q", leaked)
			}
		}
	})

	t.Run("get_command passes through a completed result", func(t *testing.T) {
		res := call("get_command", map[string]any{"command_id": commandID.String()})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		var got struct {
			Command map[string]any `json:"command"`
		}
		if err := json.Unmarshal([]byte(res.Text), &got); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if got.Command["id"] != commandID.String() || got.Command["status"] != "completed" {
			t.Fatalf("unexpected command: %+v", got.Command)
		}
	})

	t.Run("workflow_diagnose requires approval before execution", func(t *testing.T) {
		res := call("workflow_diagnose", map[string]any{"agent_id": agent.String()})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		var got struct {
			CommandID string `json:"command_id"`
			Status    string `json:"status"`
		}
		if err := json.Unmarshal([]byte(res.Text), &got); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if got.Status != "awaiting_approval" {
			t.Fatalf("expected awaiting_approval, got %q", got.Status)
		}
		var confirmation string
		if err := pool.QueryRow(ctx,
			`SELECT confirmation_status FROM commands WHERE id = $1`,
			uuid.MustParse(got.CommandID)).Scan(&confirmation); err != nil {
			t.Fatalf("read confirmation status: %v", err)
		}
		if confirmation != appcommand.ConfirmationPending {
			t.Fatalf("expected pending confirmation, got %q", confirmation)
		}
	})
}

type callOutput struct {
	Text    string
	IsError bool
}

func decodeCallOutput(t *testing.T, data []byte) callOutput {
	t.Helper()
	var msg struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if msg.Error != nil {
		t.Fatalf("transport error: %s", data)
	}
	if len(msg.Result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(msg.Result.Content))
	}
	return callOutput{Text: msg.Result.Content[0].Text, IsError: msg.Result.IsError}
}

func insertCompletedCommand(t *testing.T, ctx context.Context, pool *pgxpool.Pool, agentID uuid.UUID, tool string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO commands (agent_id, tool_name, payload, status, result)
		 VALUES ($1, $2, '{"host":"edge-1"}', 'completed', '{"ok":true}')
		 RETURNING id`,
		agentID, tool).Scan(&id); err != nil {
		t.Fatalf("insert completed command: %v", err)
	}
	return id
}
