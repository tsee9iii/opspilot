package tools

import (
	"github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

// Dependencies are the application use cases the tool set adapts. The MCP
// layer never builds its own use cases; the composition root wires them.
type Dependencies struct {
	Servers               *inventory.ListServersUseCase
	Agents                *inventory.ListAgentsUseCase
	Commands              *inventory.ListCommandsUseCase
	GetCommand            *command.GetCommandUseCase
	Dispatch              *dispatch.DispatchUseCase
	Pinger                mcp.Pinger
	DefaultTimeoutSeconds int
	// ReadOnly (default true) strips remote-execution and deployment tools
	// (workflow_deploy, workflow_diagnose) from the tool set. The MCP process
	// is read-only by default so Hermes cannot trigger mutating or diagnostic
	// execution on agents without an explicit operator opt-in.
	ReadOnly bool
}

// Build assembles the milestone MCP tool set.
//
// TODO(workflow): add workflow_rollback, workflow_backup and workflow_upgrade
// tools here once their workflows exist. They are intentionally NOT part of
// this milestone; do not register them before their use cases are implemented.
func Build(deps Dependencies) *mcp.ToolSet {
	read := NewFileReadTool(deps.Dispatch)
	list := NewFilesystemListTool(deps.Dispatch)
	inspect := NewDockerInspectTool(deps.Dispatch)
	read.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	list.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	inspect.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)

	tools := []mcp.Tool{
		NewPingTool(deps.Pinger),
		NewListServersTool(deps.Servers),
		NewListAgentsTool(deps.Agents),
		NewListCommandsTool(deps.Commands),
		NewGetCommandTool(deps.GetCommand),
		read,
		list,
		inspect,
	}

	if !deps.ReadOnly {
		diagnose := NewWorkflowDiagnoseTool(deps.Dispatch)
		deploy := NewWorkflowDeployTool(deps.Dispatch)
		diagnose.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
		deploy.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
		tools = append(tools, diagnose, deploy)
	}

	return mcp.NewToolSet(tools...)
}
