package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/mcp"
	"github.com/tsee9iii/opspilot/pkg/config"
)

// runMCPServer feeds newline-delimited JSON-RPC requests through a real MCP
// server and returns the parsed response objects.
func runMCPServer(t *testing.T, ts *mcp.ToolSet, requests string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	srv := mcp.NewServer(ts, strings.NewReader(requests), &out)
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("mcp server run: %v", err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("response is not valid JSON (%q): %v", line, err)
		}
		responses = append(responses, m)
	}
	return responses
}

// mcpCallText returns the text content of a tools/call response.
func mcpCallText(t *testing.T, resp map[string]any) string {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	content := result["content"].([]any)
	item := content[0].(map[string]any)
	return item["text"].(string)
}

// TestInvestigationToolsetAdvertisement verifies the exact toolset in every MCP
// mode: inventory excludes every remote-investigation tool, investigate and
// operate both include all of them, and no mutating tool sneaks in.
func TestInvestigationToolsetAdvertisement(t *testing.T) {
	advertised := func(ts *mcp.ToolSet) map[string]bool {
		return toolNames(ts)
	}

	inv := advertised(Build(depsWithMode(config.MCPModeInventory)))
	for _, name := range investigationToolNames {
		if inv[name] {
			t.Fatalf("inventory mode must not advertise %s", name)
		}
	}

	for _, mode := range []string{config.MCPModeInvestigate, config.MCPModeOperate} {
		seen := advertised(Build(depsWithMode(mode)))
		for _, name := range investigationToolNames {
			if !seen[name] {
				t.Fatalf("mode %q must advertise %s", mode, name)
			}
		}
	}
}

// TestInvestigationMCPDispatch seeds an agent's capabilities through the fake
// command repository and drives the real MCP server, verifying a successful
// dispatched result for PM2 and one for Docker.
func TestInvestigationMCPDispatch(t *testing.T) {
	cases := []struct {
		name      string
		wireTool  string
		result    []byte
		arguments map[string]any
	}{
		{
			name:      "pm2_list",
			wireTool:  "pm2.list",
			result:    []byte(`[{"name":"api","status":"online","pid":4242,"cpu_percent":1.2,"memory_bytes":104857600,"uptime":3600}]`),
			arguments: map[string]any{"agent_id": uuid.New().String()},
		},
		{
			name:      "docker_list",
			wireTool:  "docker.ps",
			result:    []byte(`{"containers":[{"id":"abc123","name":"merchant-api","image":"merchant-api:latest","state":"running","status":"Up 2 minutes","ports":"0.0.0.0:8080->8080/tcp"}]}`),
			arguments: map[string]any{"agent_id": uuid.New().String()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			commandID := uuid.New()
			repo := &dispatchRepo{
				createRes: createdResponse(commandID),
				getRes: appcommand.GetCommandResponse{
					ID:                 commandID,
					ConfirmationStatus: appcommand.ConfirmationApproved,
					Status:             appcommand.StatusCompleted,
					Result:             tc.result,
				},
			}
			deps := depsWithMode(config.MCPModeInvestigate)
			deps.Dispatch = newDispatch(repo)
			ts := Build(deps)

			argBytes, _ := json.Marshal(tc.arguments)
			req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"%s","arguments":%s}}`+"\n", tc.name, argBytes)
			resps := runMCPServer(t, ts, req)
			if len(resps) != 1 {
				t.Fatalf("expected 1 response, got %d", len(resps))
			}
			if _, isErr := resps[0]["error"]; isErr {
				t.Fatalf("tools/call must not fail: %v", resps[0])
			}

			var env investigationOutput
			if err := json.Unmarshal([]byte(mcpCallText(t, resps[0])), &env); err != nil {
				t.Fatalf("decode tool output: %v", err)
			}
			if env.Status != "completed" {
				t.Fatalf("status = %q, want completed", env.Status)
			}
			if !jsonEquivalent(env.Result, tc.result) {
				t.Fatalf("result = %s, want %s", env.Result, tc.result)
			}
			if len(repo.created) != 1 || repo.created[0].Tool != tc.wireTool {
				t.Fatalf("expected %s dispatched once, got %+v", tc.wireTool, repo.created)
			}
			if repo.created[0].Source != appcommand.SourceMCP {
				t.Fatalf("expected mcp source, got %q", repo.created[0].Source)
			}
		})
	}
}

// TestInvestigationMCPDispatchMissingCapability verifies the fail-closed
// capability path surfaces as a useful, machine-readable tool error over MCP
// rather than a vague internal failure.
func TestInvestigationMCPDispatchMissingCapability(t *testing.T) {
	// No capabilities seeded: the confirmation resolver fails closed and the
	// command is never created.
	repo := &dispatchRepo{}
	deps := depsWithMode(config.MCPModeOperate)
	deps.Dispatch = newDispatchResolver(repo, errConfirmationResolver{})
	ts := Build(deps)

	argBytes, _ := json.Marshal(map[string]any{"agent_id": uuid.New().String()})
	req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pm2_list","arguments":%s}}`+"\n", argBytes)
	resps := runMCPServer(t, ts, req)
	text := mcpCallText(t, resps[0])
	var te mcp.ToolError
	if err := json.Unmarshal([]byte(text), &te); err != nil {
		t.Fatalf("expected machine-readable tool error, got: %s", text)
	}
	if te.Code != "capability_not_found" {
		t.Fatalf("expected capability_not_found, got %+v", te)
	}
	if len(repo.created) != 0 {
		t.Fatalf("a command must not be created when the capability is missing")
	}
}
