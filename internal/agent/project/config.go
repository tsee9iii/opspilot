package project

// Config is the YAML representation of a single entry in the agent
// configuration's optional `projects` section.
type Config struct {
	Name       string                `yaml:"name"`
	Repository string                `yaml:"repository"`
	HealthURL  *string               `yaml:"health_url"`
	Tools      map[string]ToolConfig `yaml:"tools"`
}

// ToolConfig is the YAML representation of a project tool reference. The
// `tool` key names the registered tool; every other key is a tool parameter,
// captured as arbitrary JSON when the profile is loaded. The parameters are
// kept as generic values so the section round-trips through the agent
// configuration's Save.
type ToolConfig struct {
	Tool   string         `yaml:"tool"`
	Params map[string]any `yaml:",inline"`
}
