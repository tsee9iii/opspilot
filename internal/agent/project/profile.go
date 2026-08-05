// Package project provides Project Profiles — the configuration layer that
// describes deployable projects on an agent. This package is configuration
// and discovery only: it never executes tools, runs deployment workflows, or
// invokes the tool registry.
package project

import "encoding/json"

// Project describes a deployable project on the agent.
type Project struct {
	Name       string
	Repository string
	HealthURL  *string
	Tools      map[string]ToolReference
}

// ToolReference binds a project action (e.g. "restart", "logs") to a
// registered tool and the arbitrary JSON parameters to pass to it. Parameters
// are not schema-validated here; that remains the tool registry's
// responsibility.
type ToolReference struct {
	Tool       string
	Parameters json.RawMessage
}
