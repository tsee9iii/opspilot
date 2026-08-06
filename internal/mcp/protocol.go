package mcp

import (
	"encoding/json"
)

// JSON-RPC error codes.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// rpcRequest is an inbound JSON-RPC message. A request carries an id; a
// notification does not.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// hasID reports whether the message is a request (has an id) rather than a
// notification.
func (r *rpcRequest) hasID() bool {
	return len(r.ID) > 0
}

// rpcResponse is an outbound JSON-RPC response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func newResponse(id json.RawMessage, result json.RawMessage) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func newErrorResponse(id json.RawMessage, code int, message string, data json.RawMessage) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message, Data: data},
	}
}

// callParams is the MCP tools/call request shape.
type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// toolDefinition is the MCP tool listing entry, extended with the stable
// output schema and a category so clients can reason about the returned JSON
// shape and group tools by domain.
//
// TODO: future optional metadata fields, added only when a consumer needs
// them:
//
//	tags                  // free-form searchable tags
//	risk_level            // low | medium | high | destructive
//	requires_confirmation // bool
//	destructive           // bool
//	estimated_duration    // e.g. "30s", "5m"
type toolDefinition struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
}

// contentItem is one element of an MCP tool call result.
type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callResult is the MCP tools/call result envelope.
type callResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
