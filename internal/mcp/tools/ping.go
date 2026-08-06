package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	pingName        = "ping"
	pingDescription = "Return MCP service health: version, protocol, database status and uptime."
)

const pingInputSchema = `{"type":"object","properties":{}}`

const pingOutputSchema = `{
  "type": "object",
  "required": ["service", "version", "protocol", "central_version", "database", "uptime_seconds"],
  "properties": {
    "service": {"type": "string"},
    "version": {"type": "string"},
    "protocol": {"type": "string"},
    "central_version": {"type": "string"},
    "database": {"type": "string", "enum": ["connected", "disconnected"]},
    "uptime_seconds": {"type": "integer"}
  }
}`

// PingTool is the MCP health endpoint. It is read-only and shares the same
// payload as the JSON-RPC ping method.
type PingTool struct {
	startedAt time.Time
	pinger    mcp.Pinger
}

func NewPingTool(pinger mcp.Pinger) *PingTool {
	return &PingTool{startedAt: time.Now(), pinger: pinger}
}

func (t *PingTool) Name() string        { return pingName }
func (t *PingTool) Description() string { return pingDescription }
func (t *PingTool) Category() string    { return CategorySystem }
func (t *PingTool) InputSchema() json.RawMessage {
	return json.RawMessage(pingInputSchema)
}
func (t *PingTool) OutputSchema() json.RawMessage {
	return json.RawMessage(pingOutputSchema)
}

func (t *PingTool) Call(ctx context.Context, _ map[string]any) (json.RawMessage, error) {
	return json.Marshal(mcp.BuildHealth(ctx, t.pinger, t.startedAt))
}

var _ mcp.Tool = (*PingTool)(nil)
