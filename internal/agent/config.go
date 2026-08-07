// Package agent implements the agent process runtime.
package agent

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tsee9iii/opspilot/internal/agent/project"
)

type Config struct {
	path string `yaml:"-"`
	// profiles is the loaded project loader, built during LoadConfig.
	profiles *project.Loader `yaml:"-"`

	CentralURL        string     `yaml:"central_url"`
	RegistrationToken string     `yaml:"registration_token"`
	Secret            string     `yaml:"secret"`
	SigningKey        string     `yaml:"signing_key"`
	Version           string     `yaml:"version"`
	Server            ServerInfo `yaml:"server"`
	AgentID           string     `yaml:"agent_id"`
	// PollInterval is the fallback command-poll interval in seconds. With SSE
	// enabled (the default) it is a recovery mechanism for disconnections and
	// startup, so the default is a conservative 30s; when SSE is disabled it is
	// the only delivery path and should be lowered. Zero uses the default.
	PollInterval int `yaml:"poll_interval"`
	// SSEEnabled is a pointer so an unset key defaults to true (SSE wake-ups
	// on). Set `sse_enabled: false` to disable the SSE listener and rely on
	// fallback polling only.
	SSEEnabled *bool `yaml:"sse_enabled"`
	// HealthReportInterval is how often the agent submits a full health report
	// to central. Zero uses the default of 60s.
	HealthReportInterval int                   `yaml:"health_report_interval"`
	ExecutionPolicy      ExecutionPolicyConfig `yaml:"execution_policy"`
	ProjectConfigs       []project.Config      `yaml:"projects"`
	// AllowInsecureCentral permits an http:// central_url even when the agent
	// reports a production environment. It exists only for local development
	// against a TLS-terminated proxy and defaults to deny.
	AllowInsecureCentral bool             `yaml:"allow_insecure_central"`
	Filesystem           FilesystemConfig `yaml:"filesystem"`
	HTTPCheck            HTTPCheckConfig  `yaml:"http_check"`
}

type FilesystemConfig struct {
	// AllowAbsolutePaths lets file.read / filesystem.list accept absolute
	// filesystem paths. Defaults to deny; enable only when every tool caller
	// is trusted (Hermes is not reachable by untrusted parties).
	AllowAbsolutePaths bool `yaml:"allow_absolute_paths"`
}

type HTTPCheckConfig struct {
	// AllowEndpoints is an exact-URL allowlist for http.check (e.g. local
	// health endpoints). Configuring any of these allowlists restricts the
	// tool to exactly those destinations.
	AllowEndpoints []string `yaml:"allow_endpoints"`
	// AllowHosts is a hostname allowlist for http.check.
	AllowHosts []string `yaml:"allow_hosts"`
	// AllowCIDRs is a CIDR allowlist for http.check (public or private).
	AllowCIDRs []string `yaml:"allow_cidrs"`
	// AllowPrivate opts into loopback, link-local and RFC1918 private ranges
	// for http.check. Defaults to deny.
	AllowPrivate bool `yaml:"allow_private"`
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

	if err := cfg.validateTransportSecurity(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateTransportSecurity enforces the production TLS requirement: in a
// production environment the agent must talk to central over HTTPS. The
// development-only escape hatch (AllowInsecureCentral) is never a default.
func (c *Config) validateTransportSecurity() error {
	if c.Server.Environment != "production" || c.AllowInsecureCentral {
		return nil
	}
	u, err := url.Parse(c.CentralURL)
	if err != nil || u.Scheme != "https" {
		return errors.New("agent config: central_url must use https:// in production; set allow_insecure_central: true only for local development behind TLS")
	}
	return nil
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

// IsSSEEnabled reports whether the SSE wake-up listener should run. It defaults
// to true when `sse_enabled` is unset.
func (c *Config) IsSSEEnabled() bool {
	if c.SSEEnabled == nil {
		return true
	}
	return *c.SSEEnabled
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
