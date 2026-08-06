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
}

// Build assembles the milestone MCP tool set.
//
// TODO(workflow): add workflow_rollback, workflow_backup and workflow_upgrade
// tools here once their workflows exist. They are intentionally NOT part of
// this milestone; do not register them before their use cases are implemented.
func Build(deps Dependencies) *mcp.ToolSet {
	diagnose := NewWorkflowDiagnoseTool(deps.Dispatch)
	deploy := NewWorkflowDeployTool(deps.Dispatch)
	read := NewFileReadTool(deps.Dispatch)
	diagnose.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	deploy.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)
	read.SetDefaultTimeoutSeconds(deps.DefaultTimeoutSeconds)

	return mcp.NewToolSet(
		NewPingTool(deps.Pinger),
		NewListServersTool(deps.Servers),
		NewListAgentsTool(deps.Agents),
		NewListCommandsTool(deps.Commands),
		NewGetCommandTool(deps.GetCommand),
		diagnose,
		deploy,
		read,
	)
}
