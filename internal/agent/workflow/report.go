package workflow

import (
	"encoding/json"
	"os"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// Report is the JSON report produced by a workflow tool (diagnose, deploy).
type Report struct {
	Workflow     string       `json:"workflow"`
	Version      string       `json:"version"`
	AgentVersion string       `json:"agent_version"`
	Hostname     string       `json:"hostname"`
	Strategy     string       `json:"strategy,omitempty"`
	Status       string       `json:"status"`
	StartedAt    time.Time    `json:"started_at"`
	CompletedAt  time.Time    `json:"completed_at"`
	DurationMS   int64        `json:"duration_ms"`
	Steps        []ReportStep `json:"steps"`
}

// ReportStep captures the outcome of one workflow step. Stdout and Stderr
// carry the raw command output for tools that emit CommandResult-shaped
// results; structured tools contribute their JSON output as Stdout. On failure
// Stderr carries the step error; when the step failed with a structured tool
// error, ErrorCode, Message and Suggestion carry its machine-readable details.
type ReportStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// reportFromResult converts the workflow executor's Result into the report
// shape using the diagnostic workflow version. The workflow status is
// "completed" when at least one step completed; it is "failed" only when no
// step completed at all.
func reportFromResult(res Result, agentVersion string) Report {
	return reportFrom(res, DiagnoseWorkflowVersion, agentVersion)
}

// reportFrom converts the workflow executor's Result into the report shape
// using the given workflow version.
func reportFrom(res Result, version, agentVersion string) Report {
	status := "completed"
	if !res.Success {
		status = "failed"
	}
	hostname, _ := os.Hostname()
	steps := make([]ReportStep, 0, len(res.Steps))
	for _, sr := range res.Steps {
		steps = append(steps, stepFromResult(sr))
	}
	return Report{
		Workflow:     res.Workflow,
		Version:      version,
		AgentVersion: agentVersion,
		Hostname:     hostname,
		Status:       status,
		StartedAt:    res.StartedAt,
		CompletedAt:  res.FinishedAt,
		DurationMS:   res.FinishedAt.Sub(res.StartedAt).Milliseconds(),
		Steps:        steps,
	}
}

func stepFromResult(sr StepResult) ReportStep {
	step := ReportStep{
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
