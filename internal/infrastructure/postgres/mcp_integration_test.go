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

	// Execution tools require operator confirmation: register workflow.diagnose
	// and workflow.deploy as confirmation-required capabilities so created
	// commands stay pending and the tools return immediately.
	for _, toolName := range []string{"workflow.diagnose", "workflow.deploy"} {
		if err := capabilityRepo.Upsert(ctx, agent, appcapability.Capability{
			ToolName: toolName, Version: "1.0.0", Description: "workflow",
			ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "required",
		}); err != nil {
			t.Fatalf("register capability: %v", err)
		}
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
		Pinger:     pool,
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
		assertStructuredContent(t, res)
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
		assertStructuredContent(t, res)
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
		assertStructuredContent(t, res)
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
		assertStructuredContent(t, res)
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

	t.Run("ping reports health against the real database", func(t *testing.T) {
		res := call("ping", map[string]any{})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		assertStructuredContent(t, res)
		var got struct {
			Service        string `json:"service"`
			Version        string `json:"version"`
			Protocol       string `json:"protocol"`
			CentralVersion string `json:"central_version"`
			Database       string `json:"database"`
			UptimeSeconds  int64  `json:"uptime_seconds"`
		}
		if err := json.Unmarshal([]byte(res.Text), &got); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if got.Service != "opspilot-mcp" || got.Database != "connected" {
			t.Fatalf("unexpected health: %+v", got)
		}
		if got.Version == "" || got.Protocol == "" || got.CentralVersion == "" || got.UptimeSeconds < 0 {
			t.Fatalf("incomplete health: %+v", got)
		}
	})

	t.Run("workflow_diagnose requires approval before execution", func(t *testing.T) {
		res := call("workflow_diagnose", map[string]any{"agent_id": agent.String()})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		assertStructuredContent(t, res)
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

	t.Run("workflow_deploy requires approval before execution", func(t *testing.T) {
		res := call("workflow_deploy", map[string]any{"agent_id": agent.String(), "project": "merchant-api"})
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Text)
		}
		assertStructuredContent(t, res)
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
	})

	t.Run("every advertised tool returns structuredContent", func(t *testing.T) {
		list, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/list",
		})
		if err != nil {
			t.Fatalf("marshal tools/list: %v", err)
		}
		var out bytes.Buffer
		if err := mcp.NewServer(toolSet, bytes.NewReader(append(list, '\n')), &out).Run(ctx); err != nil {
			t.Fatalf("run tools/list: %v", err)
		}
		var listing struct {
			Result struct {
				Tools []struct {
					Name         string `json:"name"`
					OutputSchema struct {
						Required []string `json:"required"`
					} `json:"outputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out.Bytes(), &listing); err != nil {
			t.Fatalf("decode tools/list: %v", err)
		}
		if len(listing.Result.Tools) == 0 {
			t.Fatal("tools/list returned no tools")
		}

		argsByTool := map[string]map[string]any{
			"ping":              {},
			"list_servers":      {},
			"list_agents":       {},
			"list_commands":     {},
			"get_command":       {"command_id": commandID.String()},
			"workflow_diagnose": {"agent_id": agent.String()},
			"workflow_deploy":   {"agent_id": agent.String(), "project": "merchant-api"},
		}
		for _, tool := range listing.Result.Tools {
			if tool.OutputSchema.Required == nil {
				t.Fatalf("tool %s does not advertise outputSchema", tool.Name)
			}
			args, ok := argsByTool[tool.Name]
			if !ok {
				t.Fatalf("tool %s missing from test arguments", tool.Name)
			}
			assertStructuredContent(t, call(tool.Name, args))
		}
	})

	t.Run("tool errors carry text only, never structuredContent", func(t *testing.T) {
		res := call("get_command", map[string]any{"command_id": uuid.New().String()})
		if !res.IsError {
			t.Fatalf("expected error, got: %s", res.Text)
		}
		if len(res.StructuredContent) != 0 {
			t.Fatalf("error results must not carry structuredContent: %s", res.StructuredContent)
		}
	})
}

type callOutput struct {
	Text              string
	StructuredContent json.RawMessage
	IsError           bool
}

func decodeCallOutput(t *testing.T, data []byte) callOutput {
	t.Helper()
	var msg struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent json.RawMessage `json:"structuredContent"`
			IsError           bool            `json:"isError"`
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
	return callOutput{
		Text:              msg.Result.Content[0].Text,
		StructuredContent: msg.Result.StructuredContent,
		IsError:           msg.Result.IsError,
	}
}

// assertStructuredContent verifies the MCP protocol guarantee: every successful
// tool call that advertises outputSchema also returns structuredContent, and it
// carries the same object serialized into content.
func assertStructuredContent(t *testing.T, res callOutput) {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success: %s", res.Text)
	}
	if len(res.StructuredContent) == 0 {
		t.Fatalf("successful tool result must return structuredContent, got %q", res.Text)
	}
	var content, structured any
	if err := json.Unmarshal([]byte(res.Text), &content); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(res.StructuredContent, &structured); err != nil {
		t.Fatalf("structuredContent is not valid JSON: %v", err)
	}
	contentJSON, _ := json.Marshal(content)
	structuredJSON, _ := json.Marshal(structured)
	if string(contentJSON) != string(structuredJSON) {
		t.Fatalf("structuredContent must equal content:\ncontent:          %s\nstructuredContent: %s", contentJSON, structuredJSON)
	}
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
