package tools

import (
	"testing"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

// TestBuildDefinesMilestoneTools pins the exact set of MCP tools exposed to
// clients. Future workflow tools (workflow_rollback, workflow_backup,
// workflow_upgrade) are intentionally absent until their workflows exist.
func TestBuildDefinesMilestoneTools(t *testing.T) {
	ts := Build(Dependencies{
		Servers:    inventory.NewListServersUseCase(&fakeServerRepo{}),
		Agents:     inventory.NewListAgentsUseCase(&fakeAgentRepo{}),
		Commands:   inventory.NewListCommandsUseCase(&fakeCommandRepo{}),
		GetCommand: appcommand.NewGetCommandUseCase(&dispatchRepo{}),
		Dispatch:   newDispatch(&dispatchRepo{}),
	})

	want := []string{
		"ping",
		"list_servers",
		"list_agents",
		"list_commands",
		"get_command",
		"workflow_diagnose",
		"workflow_deploy",
		"file_read",
		"filesystem_list",
		"docker_inspect",
	}
	wantCategory := map[string]string{
		"ping":              CategorySystem,
		"list_servers":      CategoryInventory,
		"list_agents":       CategoryInventory,
		"list_commands":     CategoryInventory,
		"get_command":       CategoryInventory,
		"workflow_diagnose": CategoryDiagnostics,
		"workflow_deploy":   CategoryDeployment,
		"file_read":         CategoryInvestigation,
		"filesystem_list":   CategoryInvestigation,
		"docker_inspect":    CategoryInvestigation,
	}
	validCategory := map[string]bool{
		CategoryInventory:     true,
		CategoryWorkflow:      true,
		CategoryDeployment:    true,
		CategoryDiagnostics:   true,
		CategorySystem:        true,
		CategoryInvestigation: true,
	}

	defs := ts.Definitions()
	if len(defs) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(defs), defs)
	}
	seen := map[string]bool{}
	for _, def := range defs {
		seen[def.Name] = true
		if def.Description == "" || len(def.InputSchema) == 0 || len(def.OutputSchema) == 0 {
			t.Fatalf("tool %s missing metadata", def.Name)
		}
		if !validCategory[def.Category] {
			t.Fatalf("tool %s has invalid category %q", def.Name, def.Category)
		}
		if wantCategory[def.Name] != def.Category {
			t.Fatalf("tool %s category = %q, want %q", def.Name, def.Category, wantCategory[def.Name])
		}
	}
	for _, name := range want {
		if !seen[name] {
			t.Fatalf("tool %s not registered", name)
		}
	}
	for _, name := range []string{"workflow_rollback", "workflow_backup", "workflow_upgrade"} {
		if seen[name] {
			t.Fatalf("future tool %s must not be implemented yet", name)
		}
	}
}

func TestWorkflowDispatchToolConstants(t *testing.T) {
	if dispatch.WorkflowDiagnoseTool != "workflow.diagnose" ||
		dispatch.WorkflowDeployTool != "workflow.deploy" ||
		dispatch.FileReadTool != "file.read" ||
		dispatch.FilesystemListTool != "filesystem.list" ||
		dispatch.DockerInspectTool != "docker.inspect" {
		t.Fatalf("unexpected wire constants: %q %q %q %q %q", dispatch.WorkflowDiagnoseTool, dispatch.WorkflowDeployTool, dispatch.FileReadTool, dispatch.FilesystemListTool, dispatch.DockerInspectTool)
	}
}

func toolNames(ts *mcp.ToolSet) map[string]bool {
	seen := map[string]bool{}
	for _, def := range ts.Definitions() {
		seen[def.Name] = true
	}
	return seen
}

// TestBuildReadOnlyExcludesExecutionTools pins the safe default: a read-only
// MCP process exposes inventory and read-only investigation tools but never
// registers deployment or diagnostic execution tools.
func TestBuildReadOnlyExcludesExecutionTools(t *testing.T) {
	ts := Build(Dependencies{
		Servers:    inventory.NewListServersUseCase(&fakeServerRepo{}),
		Agents:     inventory.NewListAgentsUseCase(&fakeAgentRepo{}),
		Commands:   inventory.NewListCommandsUseCase(&fakeCommandRepo{}),
		GetCommand: appcommand.NewGetCommandUseCase(&dispatchRepo{}),
		Dispatch:   newDispatch(&dispatchRepo{}),
		ReadOnly:   true,
	})

	seen := toolNames(ts)
	for _, must := range []string{"ping", "list_servers", "list_agents", "list_commands", "get_command", "file_read", "filesystem_list", "docker_inspect"} {
		if !seen[must] {
			t.Fatalf("read-only toolset must include %s", must)
		}
	}
	for _, banned := range []string{"workflow_deploy", "workflow_diagnose"} {
		if seen[banned] {
			t.Fatalf("read-only toolset must not register execution tool %s", banned)
		}
		if _, ok := ts.Get(banned); ok {
			t.Fatalf("execution tool %s must not be callable in read-only mode", banned)
		}
	}
}

// TestBuildOptInEnablesExecutionTools proves the explicit opt-out (ReadOnly
// false) registers the deployment and diagnostic execution tools again.
func TestBuildOptInEnablesExecutionTools(t *testing.T) {
	ts := Build(Dependencies{
		Servers:    inventory.NewListServersUseCase(&fakeServerRepo{}),
		Agents:     inventory.NewListAgentsUseCase(&fakeAgentRepo{}),
		Commands:   inventory.NewListCommandsUseCase(&fakeCommandRepo{}),
		GetCommand: appcommand.NewGetCommandUseCase(&dispatchRepo{}),
		Dispatch:   newDispatch(&dispatchRepo{}),
		ReadOnly:   false,
	})

	seen := toolNames(ts)
	for _, want := range []string{"workflow_deploy", "workflow_diagnose", "file_read", "filesystem_list", "docker_inspect"} {
		if !seen[want] {
			t.Fatalf("opt-in toolset must include %s", want)
		}
	}
}
