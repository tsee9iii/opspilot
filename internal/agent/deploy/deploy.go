// Package deploy implements deployment strategies selected by a project's
// deploy.type configuration. The workflow only orchestrates; a strategy
// performs the actual deployment. Adding a new deployment method is a single
// new Strategy implementation registered with the Registry — no workflow,
// executor, or command changes are required.
package deploy

import (
	"context"

	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// DeployContext carries everything a strategy needs to deploy a project. It is
// the single input to a strategy, so future needs (environment variables,
// timeouts, hooks, rollback metadata) can extend it without changing strategy
// signatures.
type DeployContext struct {
	Project      project.Project
	WorkingDir   string
	DeployConfig project.DeployConfig
	HealthURL    string
}

// Strategy deploys a project. Implementations are selected by Type through the
// Registry and never dispatch on anything else. Validate is invoked before
// Deploy and reports configuration problems as structured ToolErrors.
type Strategy interface {
	Type() string
	Validate(dc DeployContext) error
	Deploy(ctx context.Context, dc DeployContext) error
}

// Registry maps deploy.type values to their strategy implementations. Lookup is
// a plain map access returning (Strategy, bool): no reflection, no type
// assertions, and no switch statements.
type Registry struct {
	strategies map[string]Strategy
}

// NewRegistry returns an empty strategy registry.
func NewRegistry() *Registry {
	return &Registry{strategies: map[string]Strategy{}}
}

// Register adds or replaces the strategy for its Type.
func (r *Registry) Register(s Strategy) {
	if s == nil || s.Type() == "" {
		return
	}
	r.strategies[s.Type()] = s
}

// Get returns the strategy registered for typ.
func (r *Registry) Get(typ string) (Strategy, bool) {
	s, ok := r.strategies[typ]
	return s, ok
}
