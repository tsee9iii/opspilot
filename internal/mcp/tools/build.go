package tools

import (
	"fmt"

	"github.com/tsee9iii/opspilot/internal/application/alert"
	"github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/application/health"
	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
	"github.com/tsee9iii/opspilot/pkg/config"
)

// Dependencies are the application use cases the tool set adapts. The MCP
// layer never builds its own use cases; the composition root wires them.
type Dependencies struct {
	Servers               *inventory.ListServersUseCase
	Agents                *inventory.ListAgentsUseCase
	Commands              *inventory.ListCommandsUseCase
	GetCommand            *command.GetCommandUseCase
	Dispatch              *dispatch.DispatchUseCase
	Health                *health.GetUseCase
	Alerts                *alert.ListUseCase
	GetAlert              *alert.GetUseCase
	Pinger                mcp.Pinger
	DefaultTimeoutSeconds int
	// Mode is the MCP capability tier: inventory, investigate or operate. It
	// gates which tools are exposed to the Hermes integration.
	Mode string
}

// Build assembles the MCP tool set for the configured capability mode.
//
// Modes are strictly cumulative:
//
//   - inventory    — pure central/PostgreSQL reads (ping, servers, agents,
//     commands, health, alerts). These never contact agents.
//   - investigate  — inventory plus safe diagnostic tools that dispatch
//     read-only inspection to agents (file_read, filesystem_list,
//     docker_inspect, workflow_diagnose), always policy-enforced.
//   - operate      — investigate plus the mutating deploy tool
//     (workflow_deploy). MCP-created mutations always require operator
//     confirmation and are never self-approved.
//
// TODO(workflow): add workflow_rollback, workflow_backup and workflow_upgrade
// tools here once their workflows exist. They are intentionally NOT part of
// this milestone; do not register them before their use cases are implemented.
func Build(deps Dependencies) *mcp.ToolSet {
	inventoryTools := []mcp.Tool{
		NewPingTool(deps.Pinger),
		NewListServersTool(deps.Servers),
		NewListAgentsTool(deps.Agents),
		NewListCommandsTool(deps.Commands),
		NewGetCommandTool(deps.GetCommand),
		NewGetAgentHealthTool(deps.Health),
		NewListAgentHealthTool(deps.Health),
		NewListUnhealthyAgentsTool(deps.Health),
		NewListAlertsTool(deps.Alerts),
		NewGetAlertTool(deps.GetAlert),
	}

	tools := append([]mcp.Tool{}, inventoryTools...)

	switch deps.Mode {
	case "", config.MCPModeInventory:
		// The most restrictive tier; inventory tools only.
	case config.MCPModeInvestigate:
		tools = append(tools, diagnosticsTools(deps)...)
	case config.MCPModeOperate:
		tools = append(tools, diagnosticsTools(deps)...)
		deploy := NewWorkflowDeployTool(deps.Dispatch)
		deploy.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
		tools = append(tools, deploy)
	default:
		// Unreachable when config validation ran; fail loudly rather than
		// silently expose a tool set for an unknown mode.
		panic(fmt.Sprintf("mcp: unknown mode %q", deps.Mode))
	}

	return mcp.NewToolSet(tools...)
}

// diagnosticsTools returns the investigate-tier tools: safe, read-only
// inspection dispatched to agents, always policy-enforced.
func diagnosticsTools(deps Dependencies) []mcp.Tool {
	read := NewFileReadTool(deps.Dispatch)
	list := NewFilesystemListTool(deps.Dispatch)
	inspect := NewDockerInspectTool(deps.Dispatch)
	diagnose := NewWorkflowDiagnoseTool(deps.Dispatch)
	read.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	list.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	inspect.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	diagnose.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	return []mcp.Tool{read, list, inspect, diagnose}
}
