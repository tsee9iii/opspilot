package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

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
		// ReadOnly disables execution/dispatch MCP tools so the Hermes
		// integration cannot mutate servers or deploy. It is enabled by
		// default; execution requires an explicit opt-out.
		ReadOnly bool
	}
}

// Validate fails closed on unsafe production configuration. In development the
// built-in dev credentials are accepted so the stack boots with zero
// environment; in production every known-default or missing secret is rejected
// and the database must use TLS. Errors reference environment variable names
// only — secret values are never included.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil configuration")
	}
	if c.Env != "production" {
		return nil
	}

	var errs []error
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
		ExecutionTimeoutSeconds int   `yaml:"execution_timeout_seconds"`
		ReadOnly                *bool `yaml:"read_only"`
	} `yaml:"mcp"`
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
	cfg.MCP.ReadOnly = true

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
	if fc.MCP.ReadOnly != nil {
		cfg.MCP.ReadOnly = *fc.MCP.ReadOnly
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
	cfg.MCP.ReadOnly = getEnvBool("OPSPILOT_MCP_READ_ONLY", cfg.MCP.ReadOnly)
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

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
