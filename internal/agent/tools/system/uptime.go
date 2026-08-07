package system

import (
	"context"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolSystemUptime      = "system.uptime"
	toolUptimeVersion     = "1.0.0"
	toolUptimeDescription = "Report system uptime and load averages"
)

// UptimeTool reports system load via /usr/bin/uptime.
// The command runner is injectable so result behavior can be tested without
// depending on the host OS.
type UptimeTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewUptimeTool() *UptimeTool {
	return &UptimeTool{run: agent.RunCommand}
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

func (t *UptimeTool) ParameterSchema() string {
	return agent.EmptyParameterSchema
}

func (t *UptimeTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *UptimeTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategorySystem,
		Domain:               "linux",
		Tags:                 []string{"uptime", "load", "host"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationInstant,
		SinceVersion:         toolUptimeVersion,
	}
}

func (t *UptimeTool) Availability(_ context.Context) (bool, string) {
	return platformSupported()
}

func (t *UptimeTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	return t.run(ctx, "/usr/bin/uptime")
}
