package tools

import (
	"testing"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	apphealth "github.com/tsee9iii/opspilot/internal/application/health"
	"github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/mcp"
	"github.com/tsee9iii/opspilot/pkg/config"
)

func deps() Dependencies {
	return Dependencies{
		Servers:    inventory.NewListServersUseCase(&fakeServerRepo{}),
		Agents:     inventory.NewListAgentsUseCase(&fakeAgentRepo{}),
		Commands:   inventory.NewListCommandsUseCase(&fakeCommandRepo{}),
		GetCommand: appcommand.NewGetCommandUseCase(&dispatchRepo{}),
		Dispatch:   newDispatch(&dispatchRepo{}),
		Health:     apphealth.NewGetUseCase(&fakeHealthRepo{}),
		Alerts:     appalert.NewListUseCase(&fakeAlertRepo{}),
		GetAlert:   appalert.NewGetUseCase(&fakeAlertRepo{}),
	}
}

// inventoryToolNames are the pure central-read tools every mode exposes.
var inventoryToolNames = []string{
	"ping",
	"list_servers",
	"list_agents",
	"list_commands",
	"get_command",
	"get_agent_health",
	"list_agent_health",
	"list_unhealthy_agents",
	"list_alerts",
	"get_alert",
}

// investigationToolNames are the read-only remote-investigation tools exposed
// in the investigate tier. Each dispatches a bounded, read-only agent command;
// none modifies state.
var investigationToolNames = []string{
	"pm2_list",
	"pm2_logs",
	"docker_list",
	"docker_logs",
	"journal_logs",
	"git_status",
	"git_current_commit",
	"git_branch",
}

// diagnosticToolNames are the investigate-tier tools that dispatch read-only
// inspection to agents.
var diagnosticToolNames = append([]string{
	"file_read",
	"filesystem_list",
	"docker_inspect",
	"workflow_diagnose",
}, investigationToolNames...)

// TestBuildInventoryMode pins the most restrictive tier: pure central reads
// only, no tools that contact agents or mutate anything.
func TestBuildInventoryMode(t *testing.T) {
	for _, mode := range []string{config.MCPModeInventory, ""} {
		ts := Build(depsWithMode(mode))
		seen := toolNames(ts)
		for _, must := range inventoryToolNames {
			if !seen[must] {
				t.Fatalf("mode %q must expose %s", mode, must)
			}
		}
		for _, banned := range append(diagnosticToolNames, "workflow_deploy") {
			if seen[banned] {
				t.Fatalf("mode %q must not expose %s", mode, banned)
			}
			if _, ok := ts.Get(banned); ok {
				t.Fatalf("mode %q must not make %s callable", mode, banned)
			}
		}
	}
}

// TestBuildInvestigateMode adds safe diagnostics but never the mutating deploy
// tool.
func TestBuildInvestigateMode(t *testing.T) {
	ts := Build(depsWithMode(config.MCPModeInvestigate))
	seen := toolNames(ts)
	for _, must := range append(inventoryToolNames, diagnosticToolNames...) {
		if !seen[must] {
			t.Fatalf("investigate mode must expose %s", must)
		}
	}
	if seen["workflow_deploy"] {
		t.Fatal("investigate mode must not expose workflow_deploy")
	}
	if _, ok := ts.Get("workflow_deploy"); ok {
		t.Fatal("investigate mode must not make workflow_deploy callable")
	}
}

// TestBuildOperateMode is the only tier that exposes the mutating deploy tool.
func TestBuildOperateMode(t *testing.T) {
	ts := Build(depsWithMode(config.MCPModeOperate))
	seen := toolNames(ts)
	for _, must := range append(inventoryToolNames, diagnosticToolNames...) {
		if !seen[must] {
			t.Fatalf("operate mode must expose %s", must)
		}
	}
	if !seen["workflow_deploy"] {
		t.Fatal("operate mode must expose workflow_deploy")
	}
}

// TestBuildToolCategories validates the category metadata of every exposed tool.
func TestBuildToolCategories(t *testing.T) {
	ts := Build(depsWithMode(config.MCPModeOperate))
	wantCategory := map[string]string{
		"ping":                  CategorySystem,
		"list_servers":          CategoryInventory,
		"list_agents":           CategoryInventory,
		"list_commands":         CategoryInventory,
		"get_command":           CategoryInventory,
		"get_agent_health":      CategoryInventory,
		"list_agent_health":     CategoryInventory,
		"list_unhealthy_agents": CategoryInventory,
		"list_alerts":           CategoryInventory,
		"get_alert":             CategoryInventory,
		"workflow_diagnose":     CategoryDiagnostics,
		"workflow_deploy":       CategoryDeployment,
		"file_read":             CategoryInvestigation,
		"filesystem_list":       CategoryInvestigation,
		"docker_inspect":        CategoryInvestigation,
		"pm2_list":              CategoryInvestigation,
		"pm2_logs":              CategoryInvestigation,
		"docker_list":           CategoryInvestigation,
		"docker_logs":           CategoryInvestigation,
		"journal_logs":          CategoryInvestigation,
		"git_status":            CategoryInvestigation,
		"git_current_commit":    CategoryInvestigation,
		"git_branch":            CategoryInvestigation,
	}
	validCategory := map[string]bool{
		CategoryInventory:     true,
		CategoryWorkflow:      true,
		CategoryDeployment:    true,
		CategoryDiagnostics:   true,
		CategorySystem:        true,
		CategoryInvestigation: true,
	}

	seen := map[string]bool{}
	for _, def := range ts.Definitions() {
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
	for name := range wantCategory {
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

// TestInvestigationDispatchToolConstants pins the wire contract between the MCP
// investigation tools and the registered agent tool names.
func TestInvestigationDispatchToolConstants(t *testing.T) {
	want := map[string]string{
		"pm2_list":           dispatch.PM2ListTool,
		"pm2_logs":           dispatch.PM2LogsTool,
		"docker_list":        dispatch.DockerPsTool,
		"docker_logs":        dispatch.DockerLogsTool,
		"journal_logs":       dispatch.JournalLogsTool,
		"git_status":         dispatch.GitStatusTool,
		"git_current_commit": dispatch.GitCurrentCommitTool,
		"git_branch":         dispatch.GitBranchTool,
	}
	expectedWire := map[string]string{
		"pm2_list":           "pm2.list",
		"pm2_logs":           "pm2.logs",
		"docker_list":        "docker.ps",
		"docker_logs":        "docker.logs",
		"journal_logs":       "journal.logs",
		"git_status":         "git.status",
		"git_current_commit": "git.current_commit",
		"git_branch":         "git.branch",
	}
	for mcpName, wire := range want {
		if wire != expectedWire[mcpName] {
			t.Fatalf("%s dispatches %q, want %q", mcpName, wire, expectedWire[mcpName])
		}
	}
}

// TestBuildExcludesMutatingTools ensures the investigation tools never map to a
// mutating agent command: no pm2/docker/systemd restart, no git pull, and no
// deploy workflow among the new investigation tools in any mode.
func TestBuildExcludesMutatingTools(t *testing.T) {
	for _, mode := range []string{config.MCPModeInventory, config.MCPModeInvestigate, config.MCPModeOperate} {
		ts := Build(depsWithMode(mode))
		seen := toolNames(ts)
		for _, mutating := range []string{
			"pm2_restart",
			"docker_restart",
			"systemctl_restart",
			"systemctl_status",
			"git_pull",
		} {
			if seen[mutating] {
				t.Fatalf("mode %q must not expose mutating tool %s", mode, mutating)
			}
			if _, ok := ts.Get(mutating); ok {
				t.Fatalf("mode %q must not make %s callable", mode, mutating)
			}
		}
	}
	// The investigation dispatch constants must only reference read-only agent
	// tools; the mutating agent tool names must never appear among them.
	for _, wire := range []string{
		dispatch.PM2ListTool, dispatch.PM2LogsTool, dispatch.DockerPsTool,
		dispatch.DockerLogsTool, dispatch.JournalLogsTool, dispatch.GitStatusTool,
		dispatch.GitCurrentCommitTool, dispatch.GitBranchTool,
	} {
		switch wire {
		case "pm2.restart", "docker.restart", "systemctl.restart", "git.pull":
			t.Fatalf("mutating agent tool %q must not be dispatched by an investigation tool", wire)
		}
	}
}

func toolNames(ts *mcp.ToolSet) map[string]bool {
	seen := map[string]bool{}
	for _, def := range ts.Definitions() {
		seen[def.Name] = true
	}
	return seen
}

func depsWithMode(mode string) Dependencies {
	d := deps()
	d.Mode = mode
	return d
}
