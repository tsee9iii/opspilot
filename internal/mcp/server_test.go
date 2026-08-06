package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeTool struct {
	name string
	call func(context.Context, map[string]any) (json.RawMessage, error)
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "fake tool " + f.name }
func (f *fakeTool) Category() string    { return "system" }
func (f *fakeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (f *fakeTool) OutputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (f *fakeTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	return f.call(ctx, args)
}

// fakePinger reports a reachable database.
type fakePinger struct{}

func (fakePinger) Ping(context.Context) error { return nil }

// runServer feeds newline-delimited JSON-RPC requests and returns the parsed
// response objects.
func runServer(t *testing.T, toolSet *ToolSet, requests string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	srv := NewServer(toolSet, strings.NewReader(requests), &out)
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("server run: %v", err)
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return nil
	}
	var responses []map[string]any
	for _, line := range strings.Split(text, "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("response is not valid JSON (%q): %v", line, err)
		}
		responses = append(responses, m)
	}
	return responses
}

func TestServerInitialize(t *testing.T) {
	responses := runServer(t, NewToolSet(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"hermes","version":"1"}}}`+"\n")
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	resp := responses[0]
	if resp["id"] != float64(1) {
		t.Fatalf("unexpected id: %v", resp["id"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object: %v", resp)
	}
	if result["protocolVersion"] != "2025-03-26" {
		t.Fatalf("unexpected protocol version: %v", result["protocolVersion"])
	}
	serverInfo := result["serverInfo"].(map[string]any)
	if serverInfo["name"] != DefaultServerName {
		t.Fatalf("unexpected server name: %v", serverInfo["name"])
	}
}

func TestServerInitializeEchoesRequestedVersion(t *testing.T) {
	responses := runServer(t, NewToolSet(), `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`+"\n")
	result := responses[0]["result"].(map[string]any)
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("expected echoed protocol version, got %v", result["protocolVersion"])
	}
}

func TestServerPing(t *testing.T) {
	responses := runServer(t, NewToolSet(), `{"jsonrpc":"2.0","id":3,"method":"ping"}`+"\n")
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	result, ok := responses[0]["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected ping result object: %v", responses[0])
	}
	for _, key := range []string{"service", "version", "protocol", "central_version", "database", "uptime_seconds"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("ping missing %s: %v", key, result)
		}
	}
	if result["service"] != ServiceName {
		t.Fatalf("unexpected service: %v", result["service"])
	}
	if result["database"] != DatabaseDisconnected {
		t.Fatalf("expected disconnected without a pinger, got %v", result["database"])
	}
}

func TestServerPingReportsDatabaseStatus(t *testing.T) {
	var out bytes.Buffer
	srv := NewServer(NewToolSet(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &out)
	srv.SetPinger(fakePinger{})

	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("server run: %v", err)
	}
	var resp struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode ping response: %v", err)
	}
	if resp.Result["database"] != DatabaseConnected {
		t.Fatalf("expected connected, got %v", resp.Result["database"])
	}
}

func TestServerNotificationNoResponse(t *testing.T) {
	responses := runServer(t, NewToolSet(), `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	if len(responses) != 0 {
		t.Fatalf("notifications must not be answered: %v", responses)
	}
}

func TestServerToolsList(t *testing.T) {
	ts := NewToolSet(&fakeTool{name: "list_servers"}, &fakeTool{name: "get_command"})
	responses := runServer(t, ts, `{"jsonrpc":"2.0","id":4,"method":"tools/list"}`+"\n")
	result := responses[0]["result"].(map[string]any)
	list := result["tools"].([]any)
	if len(list) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(list))
	}
	first := list[0].(map[string]any)
	for _, key := range []string{"name", "description", "category", "inputSchema", "outputSchema"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("tool missing %s: %v", key, first)
		}
	}
}

func TestServerToolsCallSuccess(t *testing.T) {
	ok := &fakeTool{name: "list_servers", call: func(context.Context, map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"servers":[]}`), nil
	}}
	responses := runServer(t, NewToolSet(ok),
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_servers","arguments":{}}}`+"\n")

	result := responses[0]["result"].(map[string]any)
	content := result["content"].([]any)
	item := content[0].(map[string]any)
	if item["type"] != "text" || item["text"] != `{"servers":[]}` {
		t.Fatalf("unexpected content: %v", item)
	}
	if result["isError"] != nil {
		t.Fatalf("expected success result: %v", result)
	}
	sc, hasSC := result["structuredContent"].(map[string]any)
	if !hasSC {
		t.Fatalf("expected structuredContent object: %v", result)
	}
	if _, present := sc["servers"]; !present {
		t.Fatalf("structuredContent must carry the same object as content: %v", sc)
	}
}

func TestServerToolsCallToolError(t *testing.T) {
	failing := &fakeTool{name: "get_command", call: func(context.Context, map[string]any) (json.RawMessage, error) {
		return nil, &ToolError{Code: "command_not_found", Message: "The command does not exist.", Suggestion: "Use list_commands."}
	}}
	responses := runServer(t, NewToolSet(failing),
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"get_command","arguments":{"command_id":"x"}}}`+"\n")

	result := responses[0]["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("expected isError: %v", result)
	}
	if _, ok := result["structuredContent"]; ok {
		t.Fatalf("error results must not carry structuredContent: %v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	var te ToolError
	if err := json.Unmarshal([]byte(text), &te); err != nil {
		t.Fatalf("expected machine-readable tool error: %v", err)
	}
	if te.Code != "command_not_found" || te.Suggestion != "Use list_commands." {
		t.Fatalf("unexpected tool error: %+v", te)
	}
}

func TestServerToolsCallUnknownTool(t *testing.T) {
	responses := runServer(t, NewToolSet(),
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"nope","arguments":{}}}`+"\n")
	if responses[0]["error"].(map[string]any)["code"] != float64(rpcInvalidParams) {
		t.Fatalf("expected invalid params error: %v", responses[0])
	}
}

func TestServerUnknownMethod(t *testing.T) {
	responses := runServer(t, NewToolSet(), `{"jsonrpc":"2.0","id":8,"method":"frobnicate"}`+"\n")
	if responses[0]["error"].(map[string]any)["code"] != float64(rpcMethodNotFound) {
		t.Fatalf("expected method not found: %v", responses[0])
	}
}

func TestServerMalformedJSON(t *testing.T) {
	responses := runServer(t, NewToolSet(), "not json\n")
	if responses[0]["error"].(map[string]any)["code"] != float64(rpcParseError) {
		t.Fatalf("expected parse error: %v", responses[0])
	}
}

func TestServerInvalidJSONRPCVersion(t *testing.T) {
	responses := runServer(t, NewToolSet(), `{"jsonrpc":"1.0","id":9,"method":"ping"}`+"\n")
	if responses[0]["error"].(map[string]any)["code"] != float64(rpcInvalidRequest) {
		t.Fatalf("expected invalid request: %v", responses[0])
	}
}

func TestServerEOFStopsCleanly(t *testing.T) {
	var out bytes.Buffer
	srv := NewServer(NewToolSet(), strings.NewReader(""), &out)
	if err := srv.Run(context.Background()); err != nil {
		t.Fatalf("expected clean shutdown on EOF: %v", err)
	}
}
