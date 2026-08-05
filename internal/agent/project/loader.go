package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
)

// Loader is the loaded, validated set of project profiles for an agent.
type Loader struct {
	projects []Project
}

// New loads and validates project profiles from their YAML configuration.
// Validation covers unique names, absolute repository paths, valid health URLs
// (when provided), and the presence of the "restart" and "logs" tool
// references. Tool parameter schemas are not validated here.
func New(cfgs []Config) (*Loader, error) {
	l := &Loader{projects: make([]Project, 0, len(cfgs))}
	seen := make(map[string]struct{}, len(cfgs))
	for _, cfg := range cfgs {
		p, err := buildProject(cfg)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[p.Name]; dup {
			return nil, fmt.Errorf("duplicate project name: %s", p.Name)
		}
		seen[p.Name] = struct{}{}
		l.projects = append(l.projects, p)
	}
	return l, nil
}

// Projects returns the loaded project profiles in configuration order.
func (l *Loader) Projects() []Project {
	out := make([]Project, len(l.projects))
	copy(out, l.projects)
	return out
}

// FindProject returns the profile with the given name.
func (l *Loader) FindProject(name string) (Project, bool) {
	for _, p := range l.projects {
		if p.Name == name {
			return p, true
		}
	}
	return Project{}, false
}

// buildProject converts a YAML config into a Project, applying per-project
// validation.
func buildProject(cfg Config) (Project, error) {
	if cfg.Name == "" {
		return Project{}, errors.New("project name is required")
	}
	if !filepath.IsAbs(cfg.Repository) {
		return Project{}, fmt.Errorf("repository must be an absolute path: %s", cfg.Repository)
	}
	if cfg.HealthURL != nil {
		if err := validateHealthURL(*cfg.HealthURL); err != nil {
			return Project{}, err
		}
	}

	p := Project{
		Name:       cfg.Name,
		Repository: cfg.Repository,
		HealthURL:  cfg.HealthURL,
		Tools:      make(map[string]ToolReference, len(cfg.Tools)),
	}
	for key, tc := range cfg.Tools {
		if tc.Tool == "" {
			return Project{}, fmt.Errorf("tool %q has no tool name", key)
		}
		params := json.RawMessage("{}")
		if len(tc.Params) > 0 {
			b, err := json.Marshal(tc.Params)
			if err != nil {
				return Project{}, fmt.Errorf("tool %q parameters: %w", key, err)
			}
			params = b
		}
		p.Tools[key] = ToolReference{Tool: tc.Tool, Parameters: params}
	}
	if _, ok := p.Tools["restart"]; !ok {
		return Project{}, errors.New("missing restart tool")
	}
	if _, ok := p.Tools["logs"]; !ok {
		return Project{}, errors.New("missing logs tool")
	}
	return p, nil
}

// validateHealthURL accepts only absolute http:// or https:// URLs.
func validateHealthURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid health URL %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid health URL %q: only http:// and https:// URLs are allowed", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid health URL %q: missing host", raw)
	}
	return nil
}
