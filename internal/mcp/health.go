package mcp

import (
	"context"
	"time"

	"github.com/tsee9iii/opspilot/pkg/version"
)

const (
	// ServiceName identifies the MCP service in health responses.
	ServiceName = "opspilot-mcp"
	// DatabaseConnected reports a reachable backing database.
	DatabaseConnected = "connected"
	// DatabaseDisconnected reports an unreachable backing database.
	DatabaseDisconnected = "disconnected"
)

// healthPingTimeout bounds the database probe so a health check never blocks
// the protocol loop longer than necessary.
const healthPingTimeout = 2 * time.Second

// Pinger reports whether the backing database is reachable. pgxpool.Pool
// satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health is the machine-readable ping response. It is read-only and contains
// no secrets.
type Health struct {
	Service        string `json:"service"`
	Version        string `json:"version"`
	Protocol       string `json:"protocol"`
	CentralVersion string `json:"central_version"`
	Database       string `json:"database"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
}

// BuildHealth assembles the ping payload from the process's identity. Version
// strings come from pkg/version; they are never duplicated here.
func BuildHealth(ctx context.Context, pinger Pinger, startedAt time.Time) Health {
	database := DatabaseDisconnected
	if pinger != nil {
		pingCtx, cancel := context.WithTimeout(ctx, healthPingTimeout)
		defer cancel()
		if pinger.Ping(pingCtx) == nil {
			database = DatabaseConnected
		}
	}
	return Health{
		Service:        ServiceName,
		Version:        version.MCP,
		Protocol:       protocolVersion,
		CentralVersion: version.Central,
		Database:       database,
		UptimeSeconds:  int64(time.Since(startedAt).Seconds()),
	}
}
