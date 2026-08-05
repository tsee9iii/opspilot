// Package agent implements the agent process runtime.
package agent

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	path string `yaml:"-"`

	CentralURL        string     `yaml:"central_url"`
	RegistrationToken string     `yaml:"registration_token"`
	Secret            string     `yaml:"secret"`
	Version           string     `yaml:"version"`
	Server            ServerInfo `yaml:"server"`
	AgentID           string     `yaml:"agent_id"`
	PollInterval      int        `yaml:"poll_interval"`
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
