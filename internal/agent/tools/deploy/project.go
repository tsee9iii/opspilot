// Package deploy exposes the strategy-based deployment workflow as registered
// tools, so central can dispatch deployments through the normal command
// execution path like any other command.
package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/agent"
	agentdeploy "github.com/tsee9iii/opspilot/internal/agent/deploy"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

const (
	ToolDeployProject  = "deploy.project"
	toolProjectVersion = "1.0.0"
	toolProjectDesc    = "Deploy a project using its configured deploy strategy"
	toolProjectSchema  = `{"type":"object","required":["project"],"properties":{"project":{"type":"string"}}}`
)

type deployProjectRequest struct {
	Project string `json:"project"`
}

// DeployProjectTool resolves a project's deploy strategy from the registry and
// executes it. It never switches on strategy types; the registry performs the
// lookup by project.deploy.type.
type DeployProjectTool struct {
	loader   *project.Loader
	registry *agentdeploy.Registry
}

func NewDeployProjectTool(loader *project.Loader, registry *agentdeploy.Registry) *DeployProjectTool {
	return &DeployProjectTool{loader: loader, registry: registry}
}

func (t *DeployProjectTool) Name() string {
	return ToolDeployProject
}

func (t *DeployProjectTool) Version() string {
	return toolProjectVersion
}

func (t *DeployProjectTool) Description() string {
	return toolProjectDesc
}

func (t *DeployProjectTool) ParameterSchema() string {
	return toolProjectSchema
}

func (t *DeployProjectTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationRequired
}

func (t *DeployProjectTool) Availability(context.Context) (bool, string) {
	return true, ""
}

// Execute deploys the named project through its registered strategy. A missing
// project, a project without a deploy strategy, or an unsupported deploy type
// are all surfaced as structured agent.ToolErrors.
func (t *DeployProjectTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := parseProjectRequest(payload)
	if err != nil {
		return nil, err
	}
	if t.loader == nil {
		return nil, errors.New("deploy.project: project loader is not configured")
	}
	if t.registry == nil {
		return nil, errors.New("deploy.project: strategy registry is not configured")
	}

	p, ok := t.loader.FindProject(req.Project)
	if !ok {
		return nil, &agent.ToolError{
			Code:       "project_not_found",
			Message:    fmt.Sprintf("project %q is not configured on this agent", req.Project),
			Suggestion: "Add the project to the agent config's projects section and restart the agent.",
		}
	}
	if p.Deploy == nil || p.Deploy.Type == "" {
		return nil, &agent.ToolError{
			Code:       "no_deploy_strategy",
			Message:    fmt.Sprintf("project %q has no deploy strategy configured", req.Project),
			Suggestion: "Add a deploy.type (docker-compose, pm2, or script) to the project's config.",
		}
	}

	strategy, ok := t.registry.Get(p.Deploy.Type)
	if !ok {
		return nil, &agent.ToolError{
			Code:       "unsupported_deploy_strategy",
			Message:    fmt.Sprintf("no deploy strategy for type %q", p.Deploy.Type),
			Suggestion: "Register a strategy for this deploy type or configure a supported type.",
		}
	}

	dc := agentdeploy.DeployContext{
		Project:      p,
		WorkingDir:   p.Repository,
		DeployConfig: *p.Deploy,
	}
	if p.HealthURL != nil {
		dc.HealthURL = *p.HealthURL
	}

	if err := strategy.Validate(dc); err != nil {
		return nil, fmt.Errorf("%s: %w", ToolDeployProject, err)
	}
	if err := strategy.Deploy(ctx, dc); err != nil {
		return nil, fmt.Errorf("%s: %w", ToolDeployProject, err)
	}
	return json.Marshal(map[string]string{
		"project":  p.Name,
		"strategy": strategy.Type(),
		"status":   "deployed",
	})
}

func parseProjectRequest(payload []byte) (deployProjectRequest, error) {
	if len(payload) == 0 {
		return deployProjectRequest{}, errors.New("deploy.project: payload is required")
	}
	var req deployProjectRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, fmt.Errorf("deploy.project: invalid payload: %w", err)
	}
	if req.Project == "" {
		return req, errors.New("deploy.project: project is required")
	}
	return req, nil
}
