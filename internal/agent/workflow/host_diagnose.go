package workflow

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// DiagnoseWorkflowVersion is the version of the host diagnostic workflow
// report shape. It is bumped when the report contract evolves so clients can
// adapt without breaking.
const DiagnoseWorkflowVersion = "1.0.0"

// DiagnoseReport is the JSON report produced by the host diagnostic workflow.
type DiagnoseReport struct {
	Workflow     string         `json:"workflow"`
	Version      string         `json:"version"`
	AgentVersion string         `json:"agent_version"`
	Hostname     string         `json:"hostname"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	CompletedAt  time.Time      `json:"completed_at"`
	DurationMS   int64          `json:"duration_ms"`
	Steps        []DiagnoseStep `json:"steps"`
}

// DiagnoseStep captures the outcome of one host diagnostic step. Stdout and
// Stderr carry the raw command output for tools that emit CommandResult-shaped
// results; structured tools contribute their JSON output as Stdout. On failure
// Stderr carries the step error; when the step failed with a structured tool
// error, ErrorCode, Message and Suggestion carry its machine-readable details.
type DiagnoseStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

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

// reportFromResult converts the workflow executor's Result into the Diagnose
// report shape. The workflow status is "completed" when at least one step
// completed; it is "failed" only when no step completed at all.
func reportFromResult(res Result, agentVersion string) DiagnoseReport {
	status := "completed"
	if !res.Success {
		status = "failed"
	}
	hostname, _ := os.Hostname()
	steps := make([]DiagnoseStep, 0, len(res.Steps))
	for _, sr := range res.Steps {
		steps = append(steps, diagnoseStepFromResult(sr))
	}
	return DiagnoseReport{
		Workflow:     res.Workflow,
		Version:      DiagnoseWorkflowVersion,
		AgentVersion: agentVersion,
		Hostname:     hostname,
		Status:       status,
		StartedAt:    res.StartedAt,
		CompletedAt:  res.FinishedAt,
		DurationMS:   res.FinishedAt.Sub(res.StartedAt).Milliseconds(),
		Steps:        steps,
	}
}

func diagnoseStepFromResult(sr StepResult) DiagnoseStep {
	step := DiagnoseStep{
		Name:       sr.Name,
		Status:     string(sr.Status),
		DurationMS: sr.FinishedAt.Sub(sr.StartedAt).Milliseconds(),
	}
	switch sr.Status {
	case StepFailed:
		step.Stderr = sr.Error
		step.ErrorCode = sr.ErrorCode
		step.Message = sr.Message
		step.Suggestion = sr.Suggestion
	case StepCompleted:
		step.Stdout, step.Stderr = splitCommandOutput(sr.Result)
	}
	return step
}

// splitCommandOutput returns the raw stdout/stderr of a completed step. Tools
// that emit CommandResult-shaped output contribute their stdout and stderr
// directly; structured tools contribute their JSON output as stdout so no
// result data is dropped.
func splitCommandOutput(result json.RawMessage) (string, string) {
	if len(result) == 0 {
		return "", ""
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(result, &keys); err != nil {
		return "", ""
	}
	if _, ok := keys["stdout"]; !ok {
		return string(result), ""
	}
	var cr agent.CommandResult
	if err := json.Unmarshal(result, &cr); err != nil {
		return "", ""
	}
	return cr.Stdout, cr.Stderr
}
