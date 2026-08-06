// Package agent implements the agent process runtime.
package agent

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tsee9iii/opspilot/internal/agent/project"
)

type Config struct {
	path string `yaml:"-"`
	// profiles is the loaded project loader, built during LoadConfig.
	profiles *project.Loader `yaml:"-"`

	CentralURL        string                `yaml:"central_url"`
	RegistrationToken string                `yaml:"registration_token"`
	Secret            string                `yaml:"secret"`
	Version           string                `yaml:"version"`
	Server            ServerInfo            `yaml:"server"`
	AgentID           string                `yaml:"agent_id"`
	PollInterval      int                   `yaml:"poll_interval"`
	ExecutionPolicy   ExecutionPolicyConfig `yaml:"execution_policy"`
	ProjectConfigs    []project.Config      `yaml:"projects"`
}

type ExecutionPolicyConfig struct {
	Enabled          *bool    `yaml:"enabled"`
	Timeout          string   `yaml:"timeout"`
	AllowedCommands  []string `yaml:"allowed_commands"`
	DeniedCommands   []string `yaml:"denied_commands"`
	WorkingDirectory string   `yaml:"working_directory"`
}

type ServerInfo struct {
	Hostname    string `yaml:"hostname"`
	Environment string `yaml:"environment"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agent config: read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("agent config: parse %s: %w", path, err)
	}
	cfg.path = path

	profiles, err := project.New(cfg.ProjectConfigs)
	if err != nil {
		return nil, fmt.Errorf("agent config: %w", err)
	}
	cfg.profiles = profiles

	return &cfg, nil
}

// Save persists the config file, including the locally assigned AgentID.
func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("agent config: marshal: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o600); err != nil {
		return fmt.Errorf("agent config: write %s: %w", c.path, err)
	}
	return nil
}

// Projects returns the loaded project profiles, or nil when the config has no
// `projects` section. The returned slice is a copy in configuration order.
func (c *Config) Projects() []project.Project {
	if c.profiles == nil {
		return nil
	}
	return c.profiles.Projects()
}

// Profiles returns the loaded project loader. It is always non-nil after
// LoadConfig, even when no projects are configured.
func (c *Config) Profiles() *project.Loader {
	return c.profiles
}

// FindProject returns the project profile with the given name.
func (c *Config) FindProject(name string) (project.Project, bool) {
	if c.profiles == nil {
		return project.Project{}, false
	}
	return c.profiles.FindProject(name)
}

func (c *Config) ValidateRegistration() error {
	switch {
	case c.CentralURL == "":
		return errors.New("agent config: central_url is required")
	case c.RegistrationToken == "":
		return errors.New("agent config: registration_token is required")
	case c.Secret == "":
		return errors.New("agent config: secret is required")
	case c.Version == "":
		return errors.New("agent config: version is required")
	case c.Server.Hostname == "":
		return errors.New("agent config: server.hostname is required")
	case c.Server.Environment == "":
		return errors.New("agent config: server.environment is required")
	}
	return nil
}

// Policy resolves the execution policy from config. When the policy section is
// absent, it defaults to an enabled, unrestricted policy.
func (c *Config) Policy() ExecutionPolicy {
	enabled := true
	if c.ExecutionPolicy.Enabled != nil {
		enabled = *c.ExecutionPolicy.Enabled
	}

	timeout, err := time.ParseDuration(c.ExecutionPolicy.Timeout)
	if err != nil {
		timeout = 0
	}

	return ExecutionPolicy{
		Enabled:          enabled,
		Timeout:          timeout,
		AllowedCommands:  c.ExecutionPolicy.AllowedCommands,
		DeniedCommands:   c.ExecutionPolicy.DeniedCommands,
		WorkingDirectory: c.ExecutionPolicy.WorkingDirectory,
	}
}
