package agent

import "context"

const (
	ToolSystemUptime      = "system.uptime"
	toolUptimeVersion     = "1.0.0"
	toolUptimeDescription = "Report system uptime and load averages"
)

// UptimeTool reports system load via /usr/bin/uptime.
type UptimeTool struct{}

func NewUptimeTool() *UptimeTool {
	return &UptimeTool{}
}

func (t *UptimeTool) Name() string {
	return ToolSystemUptime
}

func (t *UptimeTool) Version() string {
	return toolUptimeVersion
}

func (t *UptimeTool) Description() string {
	return toolUptimeDescription
}

func (t *UptimeTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	return runCommand(ctx, "/usr/bin/uptime")
}
