package workflow

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// Workflow is a named sequence of steps. Project operations (deploy, restart,
// diagnose, rollback) are expressed as workflows.
type Workflow struct {
	Name  string
	Steps []Step
}

// Step is a single workflow step bound to an existing project tool reference.
type Step struct {
	Name string
	Tool project.ToolReference
}

// NewWorkflow builds a workflow with the given name and steps.
func NewWorkflow(name string, steps ...Step) Workflow {
	return Workflow{Name: name, Steps: steps}
}

// BuildDeployWorkflow constructs the deployment workflow for a project:
// always git.pull (with the project's repository), then the project's stored
// restart tool exactly as configured, then http.check against the project's
// health URL when one is configured.
func BuildDeployWorkflow(p project.Project) (Workflow, error) {
	restart, ok := p.Tools["restart"]
	if !ok {
		return Workflow{}, errors.New("workflow: project has no restart tool")
	}

	repoParams, err := json.Marshal(map[string]string{"repository": p.Repository})
	if err != nil {
		return Workflow{}, fmt.Errorf("workflow: marshal git.pull parameters: %w", err)
	}
	steps := []Step{
		{
			Name: "git.pull",
			Tool: project.ToolReference{
				Tool:       "git.pull",
				Parameters: repoParams,
			},
		},
		{Name: "restart", Tool: restart},
	}

	if p.HealthURL != nil {
		urlParams, err := json.Marshal(map[string]string{"url": *p.HealthURL})
		if err != nil {
			return Workflow{}, fmt.Errorf("workflow: marshal http.check parameters: %w", err)
		}
		steps = append(steps, Step{
			Name: "health",
			Tool: project.ToolReference{
				Tool:       "http.check",
				Parameters: urlParams,
			},
		})
	}

	return Workflow{Name: "deploy", Steps: steps}, nil
}

// emptyParams is the payload accepted by tools that take no parameters.
var emptyParams = json.RawMessage("{}")

// BuildDiagnoseWorkflow constructs the diagnostic workflow for a project. It
// always gathers system metrics (system.cpu, system.memory, system.disk,
// system.processes), then adds the platform-specific step for the project's
// restart tool (docker.ps, pm2.list, or systemctl.status — reusing the
// restart tool's exact parameters), the project's logs tool exactly as stored,
// and http.check against the health URL when one is configured.
func BuildDiagnoseWorkflow(p project.Project) (Workflow, error) {
	steps := []Step{
		{Name: "system.cpu", Tool: project.ToolReference{Tool: "system.cpu", Parameters: emptyParams}},
		{Name: "system.memory", Tool: project.ToolReference{Tool: "system.memory", Parameters: emptyParams}},
		{Name: "system.disk", Tool: project.ToolReference{Tool: "system.disk", Parameters: emptyParams}},
		{Name: "system.processes", Tool: project.ToolReference{Tool: "system.processes", Parameters: emptyParams}},
	}

	if restart, ok := p.Tools["restart"]; ok {
		switch restart.Tool {
		case "docker.restart":
			steps = append(steps, Step{Name: "docker.ps", Tool: project.ToolReference{Tool: "docker.ps", Parameters: emptyParams}})
		case "pm2.restart":
			steps = append(steps, Step{Name: "pm2.list", Tool: project.ToolReference{Tool: "pm2.list", Parameters: emptyParams}})
		case "systemctl.restart":
			steps = append(steps, Step{Name: "systemctl.status", Tool: project.ToolReference{Tool: "systemctl.status", Parameters: restart.Parameters}})
		}
	}

	if logs, ok := p.Tools["logs"]; ok {
		steps = append(steps, Step{Name: logs.Tool, Tool: logs})
	}

	if p.HealthURL != nil {
		urlParams, err := json.Marshal(map[string]string{"url": *p.HealthURL})
		if err != nil {
			return Workflow{}, fmt.Errorf("workflow: marshal http.check parameters: %w", err)
		}
		steps = append(steps, Step{Name: "http.check", Tool: project.ToolReference{Tool: "http.check", Parameters: urlParams}})
	}

	return Workflow{Name: "diagnose", Steps: steps}, nil
}
