package agent

import "context"

const ToolSystemUptime = "system.uptime"

// UptimeTool reports system load via /usr/bin/uptime.
type UptimeTool struct{}

func NewUptimeTool() *UptimeTool {
	return &UptimeTool{}
}

func (t *UptimeTool) Name() string {
	return ToolSystemUptime
}

func (t *UptimeTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	return runCommand(ctx, "/usr/bin/uptime")
}
