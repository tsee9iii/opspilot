package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
	"github.com/tsee9iii/opspilot/internal/agent/workflow"
)

const (
	ToolDeploy  = "workflow.deploy"
	toolVersion = "1.0.0"
	toolDesc    = "Deploy a project through its configured strategy and verify health"
	toolSchema  = `{"type":"object","required":["project"],"properties":{"project":{"type":"string"}}}`
)

type deployRequest struct {
	Project string `json:"project"`
}

// DeployTool runs the strategy-based deployment workflow: validate the
// project, refuse a dirty working tree, pull, deploy via the project's
// strategy, then health-check. Deploying requires confirmation, enforced by
// central through the tool's confirmation level.
type DeployTool struct {
	executor     agent.Executor
	loader       *project.Loader
	agentVersion string
}

func NewDeployTool(executor agent.Executor, loader *project.Loader, agentVersion string) *DeployTool {
	return &DeployTool{executor: executor, loader: loader, agentVersion: agentVersion}
}

func (t *DeployTool) Name() string {
	return ToolDeploy
}

func (t *DeployTool) Version() string {
	return toolVersion
}

func (t *DeployTool) Description() string {
	return toolDesc
}

func (t *DeployTool) ParameterSchema() string {
	return toolSchema
}

func (t *DeployTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationRequired
}

func (t *DeployTool) Availability(context.Context) (bool, string) {
	return true, ""
}

// Execute builds and runs the strategy-based deploy workflow. A missing
// project is surfaced as a structured agent.ToolError; every other outcome is
// returned as a report with the same shape as the diagnostic report.
func (t *DeployTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := parseDeployRequest(payload)
	if err != nil {
		return nil, err
	}
	if t.loader == nil {
		return nil, errors.New("workflow.deploy: project loader is not configured")
	}

	p, ok := t.loader.FindProject(req.Project)
	if !ok {
		return nil, &agent.ToolError{
			Code:       "project_not_found",
			Message:    fmt.Sprintf("project %q is not configured on this agent", req.Project),
			Suggestion: "Add the project to the agent config's projects section and restart the agent.",
		}
	}

	report := workflow.RunStrategyDeploy(ctx, t.executor, p, t.agentVersion)
	out, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal report: %w", ToolDeploy, err)
	}
	return out, nil
}

func parseDeployRequest(payload []byte) (deployRequest, error) {
	if len(payload) == 0 {
		return deployRequest{}, errors.New("workflow.deploy: payload is required")
	}
	var req deployRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, fmt.Errorf("workflow.deploy: invalid payload: %w", err)
	}
	if req.Project == "" {
		return req, errors.New("workflow.deploy: project is required")
	}
	return req, nil
}
