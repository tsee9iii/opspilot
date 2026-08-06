package workflow

import (
	"context"
	"encoding/json"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// DiagnoseWorkflowVersion is the version of the host diagnostic workflow
// report shape. It is bumped when the report contract evolves so clients can
// adapt without breaking.
const DiagnoseWorkflowVersion = "1.0.0"

// DiagnoseReport is the JSON report produced by the host diagnostic workflow.
// It is an alias for the shared Report shape so clients can keep using the
// diagnostic-specific name.
type DiagnoseReport = Report

// DiagnoseStep captures the outcome of one workflow step. It is an alias for
// the shared ReportStep shape.
type DiagnoseStep = ReportStep

// BuildHostDiagnoseWorkflow builds the host-level diagnostic workflow: it runs
// the registered system and container capabilities in order and, when a
// service name is given, appends systemctl.status reusing that name as the
// required service parameter.
func BuildHostDiagnoseWorkflow(service string) Workflow {
	steps := []Step{
		{Name: "system.uptime", Tool: project.ToolReference{Tool: "system.uptime", Parameters: emptyParams}},
		{Name: "system.cpu", Tool: project.ToolReference{Tool: "system.cpu", Parameters: emptyParams}},
		{Name: "system.memory", Tool: project.ToolReference{Tool: "system.memory", Parameters: emptyParams}},
		{Name: "system.disk", Tool: project.ToolReference{Tool: "system.disk", Parameters: emptyParams}},
		{Name: "docker.ps", Tool: project.ToolReference{Tool: "docker.ps", Parameters: emptyParams}},
	}
	if service != "" {
		params, err := json.Marshal(map[string]string{"service": service})
		if err == nil {
			steps = append(steps, Step{
				Name: "systemctl.status",
				Tool: project.ToolReference{Tool: "systemctl.status", Parameters: params},
			})
		}
	}
	return NewWorkflow("diagnose", steps...)
}

// RunHostDiagnose executes the host diagnostic workflow through the injected
// executor — the RegistryExecutor in production — so every step goes through
// the normal execution pipeline. Steps run sequentially and never stop on
// failure: each step outcome is recorded in the returned report.
func RunHostDiagnose(ctx context.Context, executor agent.Executor, service, agentVersion string) DiagnoseReport {
	res := NewExecutor(executor).StopOnFailure(false).Execute(ctx, project.Project{}, BuildHostDiagnoseWorkflow(service))
	return reportFromResult(res, agentVersion)
}
