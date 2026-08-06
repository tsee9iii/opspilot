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
// references for legacy projects. Projects with a deploy strategy are
// validated structurally against their strategy parameters instead. Tool
// parameter schemas are not validated here.
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
// validation. `path` and `repository` are aliases for the project's absolute
// path; `health.url` and `health_url` are aliases for the health URL. Legacy
// projects must reference restart and logs tools; projects with a deploy
// strategy must carry the parameters their strategy requires.
func buildProject(cfg Config) (Project, error) {
	if cfg.Name == "" {
		return Project{}, errors.New("project name is required")
	}
	repoPath := cfg.Path
	if repoPath == "" {
		repoPath = cfg.Repository
	}
	if !filepath.IsAbs(repoPath) {
		return Project{}, fmt.Errorf("repository must be an absolute path: %s", repoPath)
	}

	healthURL := cfg.HealthURL
	if cfg.Health != nil && cfg.Health.URL != nil {
		healthURL = cfg.Health.URL
	}
	if healthURL != nil {
		if err := validateHealthURL(*healthURL); err != nil {
			return Project{}, err
		}
	}
	if cfg.Deploy != nil {
		if err := validateDeployConfig(cfg.Name, cfg.Deploy); err != nil {
			return Project{}, err
		}
	}

	p := Project{
		Name:       cfg.Name,
		Repository: repoPath,
		HealthURL:  healthURL,
		Deploy:     cfg.Deploy,
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
	if cfg.Deploy == nil {
		if _, ok := p.Tools["restart"]; !ok {
			return Project{}, errors.New("missing restart tool")
		}
		if _, ok := p.Tools["logs"]; !ok {
			return Project{}, errors.New("missing logs tool")
		}
	}
	return p, nil
}

// validateDeployConfig checks the structural requirements of a deploy
// strategy. Only the parameters of the known strategies are validated; an
// unknown type is left to the strategy registry so new strategies require no
// loader changes.
func validateDeployConfig(name string, d *DeployConfig) error {
	switch d.Type {
	case "":
		return fmt.Errorf("project %s: deploy.type is required", name)
	case StrategyDockerCompose:
		if d.ComposeFile == "" {
			return fmt.Errorf("project %s: deploy.compose_file is required for type docker-compose", name)
		}
	case StrategyPM2:
		if d.Process == "" {
			return fmt.Errorf("project %s: deploy.process is required for type pm2", name)
		}
	case StrategyScript:
		if d.Script == "" {
			return fmt.Errorf("project %s: deploy.script is required for type script", name)
		}
	}
	return nil
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
