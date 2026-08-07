package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Development-only default credentials. They exist purely so a developer can
// boot the stack without any environment; production Validate rejects them.
const (
	DevServerSecret  = "dev-only-secret-change-me"
	DevOperatorToken = "dev-operator-token-change-me"
	DevDBPassword    = "opspilot"
	DevSSLMode       = "disable"
	DevHTTPHost      = "0.0.0.0"
)

type Config struct {
	Env  string
	HTTP struct {
		Host string
		Port int
	}
	Database struct {
		Host     string
		Port     int
		User     string
		Password string
		Name     string
		SSLMode  string
	}
	Logger struct {
		Level string
	}
	Auth struct {
		ServerSecret  string
		OperatorToken string
	}
	Commands struct {
		LeaseTTLSeconds int
	}
	MCP struct {
		// ExecutionTimeoutSeconds is the default timeout for dispatched
		// workflow commands that require no operator confirmation.
		ExecutionTimeoutSeconds int
		// Mode gates which MCP tools are exposed to the Hermes integration.
		//   inventory    - read-only access to central state; no agent contact.
		//   investigate  - inventory plus safe diagnostic tools (file reads,
		//                  filesystem listing, docker inspection, workflow
		//                  diagnosis), always policy-enforced.
		//   operate      - investigate plus mutating tools (deploy). MCP-created
		//                  mutations always require operator confirmation and are
		//                  never self-approved.
		// The default is inventory, the most restrictive tier.
		Mode string
	}
	Alerts struct {
		// Enabled turns the in-process alert evaluator on. Disabled by default.
		Enabled bool
		// IntervalSeconds is how often the evaluator sweeps agents.
		IntervalSeconds int
		AgentOffline    struct {
			Enabled bool
			// Severity is 'warning' or 'critical'.
			Severity string
			// MaxOfflineSeconds is the maximum tolerated gap between heartbeats.
			MaxOfflineSeconds int
		}
		DiskUsage struct {
			Enabled  bool
			Severity string
			// ThresholdPercent is the disk usage percentage that fires the alert.
			ThresholdPercent float64
		}
		HealthReportStale struct {
			Enabled  bool
			Severity string
			// MaxReportAgeSeconds is the maximum tolerated age of a health report.
			MaxReportAgeSeconds int
		}
		ProjectUnhealthy struct {
			Enabled  bool
			Severity string
		}
	}
	Webhook struct {
		// Enabled turns outbound alert webhook delivery on. Disabled by default.
		Enabled bool
		// URL receives signed alert events. HTTPS is required in production.
		URL string
		// Secret signs outbound webhook payloads (HMAC-SHA256).
		Secret string
		// TimeoutSeconds bounds each delivery attempt.
		TimeoutSeconds int
	}
}

// MCP mode values.
const (
	MCPModeInventory   = "inventory"
	MCPModeInvestigate = "investigate"
	MCPModeOperate     = "operate"
)

// validMCPModes are the accepted MCP mode values.
var validMCPModes = map[string]bool{
	MCPModeInventory:   true,
	MCPModeInvestigate: true,
	MCPModeOperate:     true,
}

// Alert rule severity defaults.
const (
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Validate fails closed on unsafe production configuration. In development the
// built-in dev credentials are accepted so the stack boots with zero
// environment; in production every known-default or missing secret is rejected
// and the database must use TLS. Errors reference environment variable names
// only — secret values are never included.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil configuration")
	}

	var errs []error
	if !validMCPModes[c.MCP.Mode] {
		errs = append(errs, fmt.Errorf("config: OPSPILOT_MCP_MODE must be one of %s, %s, %s (got %q)",
			MCPModeInventory, MCPModeInvestigate, MCPModeOperate, c.MCP.Mode))
	}
	if c.MCP.Mode == "" {
		errs = append(errs, errors.New("config: MCP mode must not be empty"))
	}

	if c.Env != "production" {
		return errors.Join(errs...)
	}

	if c.Auth.ServerSecret == "" || c.Auth.ServerSecret == DevServerSecret {
		errs = append(errs, errors.New("config: OPSPILOT_AUTH_SERVER_SECRET must be set to a non-default value in production"))
	}
	if c.Auth.OperatorToken == "" || c.Auth.OperatorToken == DevOperatorToken {
		errs = append(errs, errors.New("config: OPSPILOT_OPERATOR_TOKEN must be set to a non-default value in production"))
	}
	if c.Database.Password == "" || c.Database.Password == DevDBPassword {
		errs = append(errs, errors.New("config: OPSPILOT_DB_PASSWORD must be set to a non-default value in production"))
	}
	if c.Database.SSLMode == "" || c.Database.SSLMode == DevSSLMode {
		errs = append(errs, errors.New("config: OPSPILOT_DB_SSLMODE must not be 'disable' in production"))
	}
	if c.HTTP.Host == "" || c.HTTP.Host == DevHTTPHost {
		errs = append(errs, errors.New("config: OPSPILOT_HTTP_HOST must not bind 0.0.0.0 in production"))
	}
	if c.Webhook.Enabled {
		if c.Webhook.URL == "" {
			errs = append(errs, errors.New("config: OPSPILOT_WEBHOOK_URL must be set when alerts webhooks are enabled"))
		} else if !strings.HasPrefix(c.Webhook.URL, "https://") {
			errs = append(errs, errors.New("config: OPSPILOT_WEBHOOK_URL must use https in production"))
		}
		if c.Webhook.Secret == "" {
			errs = append(errs, errors.New("config: OPSPILOT_WEBHOOK_SECRET must be set when alerts webhooks are enabled"))
		}
	}
	return errors.Join(errs...)
}

// fileConfig is the on-disk YAML representation. YAML keys map onto the Config
// struct; database.database → Config.Database.Name and database.username →
// Config.Database.User.
type fileConfig struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Database string `yaml:"database"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"database"`
	Logger struct {
		Level string `yaml:"level"`
	} `yaml:"logger"`
	Auth struct {
		ServerSecret  string `yaml:"server_secret"`
		OperatorToken string `yaml:"operator_token"`
	} `yaml:"auth"`
	Commands struct {
		LeaseTTLSeconds int `yaml:"lease_ttl_seconds"`
	} `yaml:"commands"`
	MCP struct {
		ExecutionTimeoutSeconds int    `yaml:"execution_timeout_seconds"`
		Mode                    string `yaml:"mode"`
		// ReadOnly is a deprecated alias for mode. read_only=true maps to
		// 'inventory' and read_only=false to 'operate'. An explicit mode always
		// wins; the alias exists so existing configs keep working.
		ReadOnly *bool `yaml:"read_only"`
	} `yaml:"mcp"`
	Alerts struct {
		Enabled         bool `yaml:"enabled"`
		IntervalSeconds int  `yaml:"interval_seconds"`
		AgentOffline    struct {
			Enabled           bool   `yaml:"enabled"`
			Severity          string `yaml:"severity"`
			MaxOfflineSeconds int    `yaml:"max_offline_seconds"`
		} `yaml:"agent_offline"`
		DiskUsage struct {
			Enabled          bool    `yaml:"enabled"`
			Severity         string  `yaml:"severity"`
			ThresholdPercent float64 `yaml:"threshold_percent"`
		} `yaml:"disk_usage"`
		HealthReportStale struct {
			Enabled             bool   `yaml:"enabled"`
			Severity            string `yaml:"severity"`
			MaxReportAgeSeconds int    `yaml:"max_report_age_seconds"`
		} `yaml:"health_report_stale"`
		ProjectUnhealthy struct {
			Enabled  bool   `yaml:"enabled"`
			Severity string `yaml:"severity"`
		} `yaml:"project_unhealthy"`
	} `yaml:"alerts"`
	Webhook struct {
		Enabled        bool   `yaml:"enabled"`
		URL            string `yaml:"url"`
		Secret         string `yaml:"secret"`
		TimeoutSeconds int    `yaml:"timeout_seconds"`
	} `yaml:"webhook"`
}

// defaultConfigPath is used when OPSPILOT_CONFIG is not set.
const defaultConfigPath = "/etc/opspilot/central.yaml"

// Load builds a Config from defaults, then the YAML file (when present), then
// environment variables, which always win. A missing config file is not an
// error: startup continues with defaults plus environment variables.
func Load() (*Config, error) {
	cfg := defaults()

	path := configPath()
	fileCfg, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	if fileCfg != nil {
		applyFile(fileCfg, cfg)
	}

	applyEnv(cfg)

	return cfg, nil
}

// defaults returns a Config populated entirely from built-in defaults.
func defaults() *Config {
	cfg := &Config{}
	cfg.Env = "development"

	cfg.HTTP.Host = DevHTTPHost
	cfg.HTTP.Port = 8080

	cfg.Database.Host = "localhost"
	cfg.Database.Port = 5432
	cfg.Database.User = "opspilot"
	cfg.Database.Password = DevDBPassword
	cfg.Database.Name = "opspilot"
	cfg.Database.SSLMode = DevSSLMode

	cfg.Logger.Level = "info"

	cfg.Auth.ServerSecret = DevServerSecret
	cfg.Auth.OperatorToken = DevOperatorToken

	cfg.Commands.LeaseTTLSeconds = 60

	cfg.MCP.ExecutionTimeoutSeconds = 300
	// Mode defaults to inventory, the most restrictive tier. The deprecated
	// read_only alias may still override it during file/env loading.
	cfg.MCP.Mode = MCPModeInventory

	cfg.Alerts.Enabled = false
	cfg.Alerts.IntervalSeconds = 60
	cfg.Alerts.AgentOffline.Severity = SeverityCritical
	cfg.Alerts.AgentOffline.MaxOfflineSeconds = 300
	cfg.Alerts.DiskUsage.Severity = SeverityWarning
	cfg.Alerts.DiskUsage.ThresholdPercent = 90
	cfg.Alerts.HealthReportStale.Severity = SeverityWarning
	cfg.Alerts.HealthReportStale.MaxReportAgeSeconds = 600
	cfg.Alerts.ProjectUnhealthy.Severity = SeverityCritical

	cfg.Webhook.Enabled = false
	cfg.Webhook.TimeoutSeconds = 5

	return cfg
}

// configPath resolves the YAML file: OPSPILOT_CONFIG when set, otherwise
// /etc/opspilot/central.yaml.
func configPath() string {
	if p := os.Getenv("OPSPILOT_CONFIG"); p != "" {
		return p
	}
	return defaultConfigPath
}

// loadFile reads and parses the YAML file. It returns (nil, nil) when the file
// does not exist.
func loadFile(path string) (*fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &fc, nil
}

// applyFile overlays non-zero file values onto the defaults.
func applyFile(fc *fileConfig, cfg *Config) {
	if fc.Server.Host != "" {
		cfg.HTTP.Host = fc.Server.Host
	}
	if fc.Server.Port != 0 {
		cfg.HTTP.Port = fc.Server.Port
	}

	if fc.Database.Host != "" {
		cfg.Database.Host = fc.Database.Host
	}
	if fc.Database.Port != 0 {
		cfg.Database.Port = fc.Database.Port
	}
	if fc.Database.Database != "" {
		cfg.Database.Name = fc.Database.Database
	}
	if fc.Database.Username != "" {
		cfg.Database.User = fc.Database.Username
	}
	if fc.Database.Password != "" {
		cfg.Database.Password = fc.Database.Password
	}
	if fc.Database.SSLMode != "" {
		cfg.Database.SSLMode = fc.Database.SSLMode
	}

	if fc.Logger.Level != "" {
		cfg.Logger.Level = fc.Logger.Level
	}

	if fc.Auth.ServerSecret != "" {
		cfg.Auth.ServerSecret = fc.Auth.ServerSecret
	}
	if fc.Auth.OperatorToken != "" {
		cfg.Auth.OperatorToken = fc.Auth.OperatorToken
	}

	if fc.Commands.LeaseTTLSeconds != 0 {
		cfg.Commands.LeaseTTLSeconds = fc.Commands.LeaseTTLSeconds
	}

	if fc.MCP.ExecutionTimeoutSeconds != 0 {
		cfg.MCP.ExecutionTimeoutSeconds = fc.MCP.ExecutionTimeoutSeconds
	}
	// An explicit mode always wins over the deprecated read_only alias. A
	// read_only value maps: true -> inventory, false -> operate.
	if fc.MCP.Mode != "" {
		cfg.MCP.Mode = fc.MCP.Mode
	} else if fc.MCP.ReadOnly != nil {
		if *fc.MCP.ReadOnly {
			cfg.MCP.Mode = MCPModeInventory
		} else {
			cfg.MCP.Mode = MCPModeOperate
		}
	}

	if fc.Alerts.Enabled {
		cfg.Alerts.Enabled = true
	}
	if fc.Alerts.IntervalSeconds != 0 {
		cfg.Alerts.IntervalSeconds = fc.Alerts.IntervalSeconds
	}
	if fc.Alerts.AgentOffline.Enabled {
		cfg.Alerts.AgentOffline.Enabled = true
	}
	if fc.Alerts.AgentOffline.Severity != "" {
		cfg.Alerts.AgentOffline.Severity = fc.Alerts.AgentOffline.Severity
	}
	if fc.Alerts.AgentOffline.MaxOfflineSeconds != 0 {
		cfg.Alerts.AgentOffline.MaxOfflineSeconds = fc.Alerts.AgentOffline.MaxOfflineSeconds
	}
	if fc.Alerts.DiskUsage.Enabled {
		cfg.Alerts.DiskUsage.Enabled = true
	}
	if fc.Alerts.DiskUsage.Severity != "" {
		cfg.Alerts.DiskUsage.Severity = fc.Alerts.DiskUsage.Severity
	}
	if fc.Alerts.DiskUsage.ThresholdPercent != 0 {
		cfg.Alerts.DiskUsage.ThresholdPercent = fc.Alerts.DiskUsage.ThresholdPercent
	}
	if fc.Alerts.HealthReportStale.Enabled {
		cfg.Alerts.HealthReportStale.Enabled = true
	}
	if fc.Alerts.HealthReportStale.Severity != "" {
		cfg.Alerts.HealthReportStale.Severity = fc.Alerts.HealthReportStale.Severity
	}
	if fc.Alerts.HealthReportStale.MaxReportAgeSeconds != 0 {
		cfg.Alerts.HealthReportStale.MaxReportAgeSeconds = fc.Alerts.HealthReportStale.MaxReportAgeSeconds
	}
	if fc.Alerts.ProjectUnhealthy.Enabled {
		cfg.Alerts.ProjectUnhealthy.Enabled = true
	}
	if fc.Alerts.ProjectUnhealthy.Severity != "" {
		cfg.Alerts.ProjectUnhealthy.Severity = fc.Alerts.ProjectUnhealthy.Severity
	}

	if fc.Webhook.Enabled {
		cfg.Webhook.Enabled = true
	}
	if fc.Webhook.URL != "" {
		cfg.Webhook.URL = fc.Webhook.URL
	}
	if fc.Webhook.Secret != "" {
		cfg.Webhook.Secret = fc.Webhook.Secret
	}
	if fc.Webhook.TimeoutSeconds != 0 {
		cfg.Webhook.TimeoutSeconds = fc.Webhook.TimeoutSeconds
	}
}

// applyEnv overlays environment variables (highest priority).
func applyEnv(cfg *Config) {
	cfg.Env = getEnv("OPSPILOT_ENV", cfg.Env)

	cfg.HTTP.Host = getEnv("OPSPILOT_HTTP_HOST", cfg.HTTP.Host)
	cfg.HTTP.Port = getEnvInt("OPSPILOT_HTTP_PORT", cfg.HTTP.Port)

	cfg.Database.Host = getEnv("OPSPILOT_DB_HOST", cfg.Database.Host)
	cfg.Database.Port = getEnvInt("OPSPILOT_DB_PORT", cfg.Database.Port)
	cfg.Database.User = getEnv("OPSPILOT_DB_USER", cfg.Database.User)
	cfg.Database.Password = getEnv("OPSPILOT_DB_PASSWORD", cfg.Database.Password)
	cfg.Database.Name = getEnv("OPSPILOT_DB_NAME", cfg.Database.Name)
	cfg.Database.SSLMode = getEnv("OPSPILOT_DB_SSLMODE", cfg.Database.SSLMode)

	cfg.Logger.Level = getEnv("OPSPILOT_LOG_LEVEL", cfg.Logger.Level)

	cfg.Auth.ServerSecret = getEnv("OPSPILOT_AUTH_SERVER_SECRET", cfg.Auth.ServerSecret)
	cfg.Auth.OperatorToken = getEnv("OPSPILOT_OPERATOR_TOKEN", cfg.Auth.OperatorToken)

	cfg.Commands.LeaseTTLSeconds = getEnvInt("OPSPILOT_COMMAND_LEASE_TTL_SECONDS", cfg.Commands.LeaseTTLSeconds)

	cfg.MCP.ExecutionTimeoutSeconds = getEnvInt("OPSPILOT_MCP_EXECUTION_TIMEOUT_SECONDS", cfg.MCP.ExecutionTimeoutSeconds)
	// An explicit OPSPILOT_MCP_MODE always wins over the deprecated
	// OPSPILOT_MCP_READ_ONLY alias (which maps true -> inventory, false ->
	// operate).
	if mode, ok := os.LookupEnv("OPSPILOT_MCP_MODE"); ok && mode != "" {
		cfg.MCP.Mode = mode
	} else if ro, ok := os.LookupEnv("OPSPILOT_MCP_READ_ONLY"); ok && ro != "" {
		if b, err := strconv.ParseBool(ro); err == nil {
			if b {
				cfg.MCP.Mode = MCPModeInventory
			} else {
				cfg.MCP.Mode = MCPModeOperate
			}
		}
	}

	cfg.Alerts.Enabled = getEnvBool("OPSPILOT_ALERTS_ENABLED", cfg.Alerts.Enabled)
	cfg.Alerts.IntervalSeconds = getEnvInt("OPSPILOT_ALERTS_INTERVAL_SECONDS", cfg.Alerts.IntervalSeconds)
	cfg.Alerts.AgentOffline.Enabled = getEnvBool("OPSPILOT_ALERTS_AGENT_OFFLINE_ENABLED", cfg.Alerts.AgentOffline.Enabled)
	cfg.Alerts.AgentOffline.Severity = getEnv("OPSPILOT_ALERTS_AGENT_OFFLINE_SEVERITY", cfg.Alerts.AgentOffline.Severity)
	cfg.Alerts.AgentOffline.MaxOfflineSeconds = getEnvInt("OPSPILOT_ALERTS_AGENT_OFFLINE_MAX_OFFLINE_SECONDS", cfg.Alerts.AgentOffline.MaxOfflineSeconds)
	cfg.Alerts.DiskUsage.Enabled = getEnvBool("OPSPILOT_ALERTS_DISK_USAGE_ENABLED", cfg.Alerts.DiskUsage.Enabled)
	cfg.Alerts.DiskUsage.Severity = getEnv("OPSPILOT_ALERTS_DISK_USAGE_SEVERITY", cfg.Alerts.DiskUsage.Severity)
	cfg.Alerts.DiskUsage.ThresholdPercent = getEnvFloat("OPSPILOT_ALERTS_DISK_USAGE_THRESHOLD_PERCENT", cfg.Alerts.DiskUsage.ThresholdPercent)
	cfg.Alerts.HealthReportStale.Enabled = getEnvBool("OPSPILOT_ALERTS_HEALTH_REPORT_STALE_ENABLED", cfg.Alerts.HealthReportStale.Enabled)
	cfg.Alerts.HealthReportStale.Severity = getEnv("OPSPILOT_ALERTS_HEALTH_REPORT_STALE_SEVERITY", cfg.Alerts.HealthReportStale.Severity)
	cfg.Alerts.HealthReportStale.MaxReportAgeSeconds = getEnvInt("OPSPILOT_ALERTS_HEALTH_REPORT_STALE_MAX_REPORT_AGE_SECONDS", cfg.Alerts.HealthReportStale.MaxReportAgeSeconds)
	cfg.Alerts.ProjectUnhealthy.Enabled = getEnvBool("OPSPILOT_ALERTS_PROJECT_UNHEALTHY_ENABLED", cfg.Alerts.ProjectUnhealthy.Enabled)
	cfg.Alerts.ProjectUnhealthy.Severity = getEnv("OPSPILOT_ALERTS_PROJECT_UNHEALTHY_SEVERITY", cfg.Alerts.ProjectUnhealthy.Severity)

	cfg.Webhook.Enabled = getEnvBool("OPSPILOT_WEBHOOK_ENABLED", cfg.Webhook.Enabled)
	cfg.Webhook.URL = getEnv("OPSPILOT_WEBHOOK_URL", cfg.Webhook.URL)
	cfg.Webhook.Secret = getEnv("OPSPILOT_WEBHOOK_SECRET", cfg.Webhook.Secret)
	cfg.Webhook.TimeoutSeconds = getEnvInt("OPSPILOT_WEBHOOK_TIMEOUT_SECONDS", cfg.Webhook.TimeoutSeconds)
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
