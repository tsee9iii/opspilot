package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/tsee9iii/opspilot/pkg/version"
)

const (
	// DefaultServerName identifies the MCP server to clients.
	DefaultServerName = "opspilot"
	// DefaultServerVersion is reported in initialize. The value lives in
	// pkg/version so every process reports the same MCP build version.
	DefaultServerVersion = version.MCP
	// protocolVersion is the MCP protocol revision this server speaks.
	protocolVersion = "2025-03-26"
)

// Server is a Model Context Protocol server over newline-delimited JSON-RPC on
// an arbitrary reader/writer pair (os.Stdin/os.Stdout in production, pipes or
// buffers in tests).
type Server struct {
	name      string
	version   string
	tools     *ToolSet
	in        io.Reader
	out       io.Writer
	log       *slog.Logger
	startedAt time.Time
	pinger    Pinger
}

func NewServer(tools *ToolSet, in io.Reader, out io.Writer) *Server {
	return NewServerWithIdentity(DefaultServerName, DefaultServerVersion, tools, in, out)
}

func NewServerWithIdentity(name, version string, tools *ToolSet, in io.Reader, out io.Writer) *Server {
	return &Server{
		name:      name,
		version:   version,
		tools:     tools,
		in:        in,
		out:       out,
		log:       slog.Default(),
		startedAt: time.Now(),
	}
}

// SetPinger wires the database probe used by the ping health endpoint. When no
// pinger is set, ping reports the database as disconnected.
func (s *Server) SetPinger(p Pinger) {
	s.pinger = p
}

// SetLogger overrides the logger used for request errors.
func (s *Server) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// Run serves requests until the input reaches EOF or ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	reader := bufio.NewReader(s.in)
	writer := bufio.NewWriter(s.out)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			resp := s.handle(ctx, line)
			if resp != nil {
				if _, werr := writer.Write(resp); werr != nil {
					return fmt.Errorf("mcp: write response: %w", werr)
				}
				if werr := writer.WriteByte('\n'); werr != nil {
					return fmt.Errorf("mcp: write response: %w", werr)
				}
				if werr := writer.Flush(); werr != nil {
					return fmt.Errorf("mcp: flush response: %w", werr)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("mcp: read request: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil
		default:
		}
	}
}

// handle processes a single JSON-RPC message. Notifications return nil.
func (s *Server) handle(ctx context.Context, line []byte) []byte {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return marshal(newErrorResponse(nil, rpcParseError, "parse error", nil))
	}
	if req.JSONRPC != "2.0" {
		return marshal(newErrorResponse(req.ID, rpcInvalidRequest, "invalid request: jsonrpc must be 2.0", nil))
	}

	// Notifications are never answered.
	if !req.hasID() {
		return nil
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "ping":
		return s.handlePing(ctx, req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "":
		return marshal(newErrorResponse(req.ID, rpcInvalidRequest, "invalid request: method is required", nil))
	default:
		return marshal(newErrorResponse(req.ID, rpcMethodNotFound, "method not found", nil))
	}
}

func (s *Server) handlePing(ctx context.Context, req rpcRequest) []byte {
	result, _ := json.Marshal(BuildHealth(ctx, s.pinger, s.startedAt))
	return marshal(newResponse(req.ID, result))
}

func (s *Server) handleInitialize(req rpcRequest) []byte {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &params)
	}
	version := protocolVersion
	if params.ProtocolVersion != "" {
		version = params.ProtocolVersion
	}
	result, _ := json.Marshal(map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{"name": s.name, "version": s.version},
	})
	return marshal(newResponse(req.ID, result))
}

func (s *Server) handleToolsList(req rpcRequest) []byte {
	result, _ := json.Marshal(map[string]any{"tools": s.tools.Definitions()})
	return marshal(newResponse(req.ID, result))
}

func (s *Server) handleToolsCall(ctx context.Context, req rpcRequest) []byte {
	var params callParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return marshal(newErrorResponse(req.ID, rpcInvalidParams, "invalid params", nil))
		}
	}
	if params.Name == "" {
		return marshal(newErrorResponse(req.ID, rpcInvalidParams, "invalid params: name is required", nil))
	}

	tool, ok := s.tools.Get(params.Name)
	if !ok {
		return marshal(newErrorResponse(req.ID, rpcInvalidParams, "unknown tool: "+params.Name, nil))
	}

	out, err := tool.Call(ctx, params.Arguments)
	if err != nil {
		result, _ := json.Marshal(callResult{
			Content: []contentItem{{Type: "text", Text: string(marshalToolError(err))}},
			IsError: true,
		})
		return marshal(newResponse(req.ID, result))
	}
	// out is the tool's already-marshaled result. It is reused verbatim for
	// structuredContent and as the human-readable content text, so the result
	// object is serialized exactly once.
	if len(out) == 0 {
		out = json.RawMessage(`{}`)
	}
	result, _ := json.Marshal(callResult{
		Content:           []contentItem{{Type: "text", Text: string(out)}},
		StructuredContent: out,
	})
	return marshal(newResponse(req.ID, result))
}

func marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
