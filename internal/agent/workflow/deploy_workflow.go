package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// DeployWorkflowVersion is the version of the strategy-based deploy workflow
// report shape. It is bumped when the report contract evolves so clients can
// adapt without breaking.
const DeployWorkflowVersion = "1.0.0"

// BuildStrategyDeployWorkflow constructs the strategy-based deployment
// workflow for a project: it checks for a clean working tree (git.status),
// pulls the configured branch (git.pull), deploys through the project's
// strategy (deploy.project), and finally health-checks (http.check) when a
// health URL is configured. The workflow never switches on the project's
// deploy type; strategy dispatch is the deploy.project step's responsibility.
func BuildStrategyDeployWorkflow(p project.Project) (Workflow, error) {
	repoParams, err := json.Marshal(map[string]string{"repository": p.Repository})
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow: marshal git parameters: %w", err)
	}
	projectParams, err := json.Marshal(map[string]string{"project": p.Name})
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow: marshal deploy.project parameters: %w", err)
	}
	steps := []Step{
		{Name: "git.status", Tool: project.ToolReference{Tool: "git.status", Parameters: repoParams}},
		{Name: "git.pull", Tool: project.ToolReference{Tool: "git.pull", Parameters: repoParams}},
		{Name: "deploy.project", Tool: project.ToolReference{Tool: "deploy.project", Parameters: projectParams}},
	}

	if p.HealthURL != nil {
		urlParams, err := json.Marshal(map[string]any{
			"url":             *p.HealthURL,
			"timeout_seconds": 30,
		})
		if err != nil {
			return Workflow{}, fmt.Errorf("workflow: marshal http.check parameters: %w", err)
		}
		steps = append(steps, Step{
			Name: "health",
			Tool: project.ToolReference{Tool: "http.check", Parameters: urlParams},
		})
	}

	return NewWorkflow("deploy", steps...), nil
}

// RunStrategyDeploy executes the strategy-based deploy workflow through the
// injected executor — the RegistryExecutor in production. It refuses a dirty
// working tree before pulling (AbortWhen), stops on any failing step, and
// fails when the health check reports an unhealthy service (FailWhen).
func RunStrategyDeploy(ctx context.Context, executor agent.Executor, p project.Project, agentVersion string) Report {
	wf, err := BuildStrategyDeployWorkflow(p)
	if err != nil {
		return failedDeployReport(p.Name, agentVersion, err)
	}

	res := NewExecutor(executor).
		StopOnFailure(true).
		AbortWhen(func(sr StepResult) bool {
			return sr.Name == "git.status" && gitStatusDirty(sr.Result)
		}).
		FailWhen(func(sr StepResult) (bool, string) {
			if sr.Name != "health" || sr.Status != StepCompleted {
				return false, ""
			}
			if httpCheckHealthy(sr.Result) {
				return false, ""
			}
			return true, "health check failed: service is unhealthy"
		}).
		Execute(ctx, p, wf)

	report := reportFrom(res, DeployWorkflowVersion, agentVersion)
	report.Strategy = strategyOf(p)
	return report
}

// strategyOf reports the project's configured deploy strategy, empty when
// absent. It is metadata only — the workflow never dispatches on it.
func strategyOf(p project.Project) string {
	if p.Deploy == nil {
		return ""
	}
	return p.Deploy.Type
}

// gitStatusDirty reports whether a completed git.status step found uncommitted
// changes.
func gitStatusDirty(result json.RawMessage) bool {
	var status struct {
		Dirty bool `json:"dirty"`
	}
	if len(result) == 0 || json.Unmarshal(result, &status) != nil {
		return false
	}
	return status.Dirty
}

// httpCheckHealthy reports whether a completed http.check step found the
// service healthy.
func httpCheckHealthy(result json.RawMessage) bool {
	var check struct {
		Healthy bool `json:"healthy"`
	}
	if len(result) == 0 || json.Unmarshal(result, &check) != nil {
		return false
	}
	return check.Healthy
}

// failedDeployReport is returned when the deploy workflow itself cannot be
// assembled — unreachable in practice since parameter marshaling cannot fail —
// so callers always receive a report rather than an error.
func failedDeployReport(projectName, agentVersion string, cause error) Report {
	now := time.Now()
	hostname, _ := os.Hostname()
	return Report{
		Workflow:     "deploy",
		Version:      DeployWorkflowVersion,
		AgentVersion: agentVersion,
		Hostname:     hostname,
		Status:       "failed",
		StartedAt:    now,
		CompletedAt:  now,
		DurationMS:   0,
		Steps: []ReportStep{
			{Name: "deploy", Status: "failed", Stderr: cause.Error()},
		},
	}
}
