package project

// Config is the YAML representation of a single entry in the agent
// configuration's optional `projects` section.
type Config struct {
	Name       string                `yaml:"name"`
	Repository string                `yaml:"repository"`
	Path       string                `yaml:"path,omitempty"`
	HealthURL  *string               `yaml:"health_url"`
	Health     *HealthConfig         `yaml:"health,omitempty"`
	Deploy     *DeployConfig         `yaml:"deploy,omitempty"`
	Tools      map[string]ToolConfig `yaml:"tools"`
}

// HealthConfig is the nested health-check configuration of a project. `url`
// is an alternative to the flat `health_url` field.
type HealthConfig struct {
	URL *string `yaml:"url,omitempty"`
}

// DeployConfig describes how a project is deployed. `type` selects the deploy
// strategy and the matching field carries its configuration: `compose_file`
// for docker-compose, `process` for pm2, and `script` for script deploys.
// Unknown types pass validation and are resolved by the strategy registry at
// deploy time.
type DeployConfig struct {
	Type        string `yaml:"type,omitempty"`
	ComposeFile string `yaml:"compose_file,omitempty"`
	Process     string `yaml:"process,omitempty"`
	Script      string `yaml:"script,omitempty"`
}

// Deploy strategy type identifiers. They live here — rather than in the deploy
// package — because the config loader validates them and the deploy package
// imports this configuration model, keeping a single source of truth without
// an import cycle.
const (
	StrategyDockerCompose = "docker-compose"
	StrategyPM2           = "pm2"
	StrategyScript        = "script"
)

// ToolConfig is the YAML representation of a project tool reference. The
// `tool` key names the registered tool; every other key is a tool parameter,
// captured as arbitrary JSON when the profile is loaded. The parameters are
// kept as generic values so the section round-trips through the agent
// configuration's Save.
type ToolConfig struct {
	Tool   string         `yaml:"tool"`
	Params map[string]any `yaml:",inline"`
}
