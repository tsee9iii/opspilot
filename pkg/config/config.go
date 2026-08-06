package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
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
		ServerSecret string
	}
	MCP struct {
		// ExecutionTimeoutSeconds is the default timeout for dispatched
		// workflow commands that require no operator confirmation.
		ExecutionTimeoutSeconds int
	}
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
		ServerSecret string `yaml:"server_secret"`
	} `yaml:"auth"`
	MCP struct {
		ExecutionTimeoutSeconds int `yaml:"execution_timeout_seconds"`
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

	cfg.HTTP.Host = "0.0.0.0"
	cfg.HTTP.Port = 8080

	cfg.Database.Host = "localhost"
	cfg.Database.Port = 5432
	cfg.Database.User = "opspilot"
	cfg.Database.Password = "opspilot"
	cfg.Database.Name = "opspilot"
	cfg.Database.SSLMode = "disable"

	cfg.Logger.Level = "info"

	cfg.Auth.ServerSecret = "dev-only-secret-change-me"

	cfg.MCP.ExecutionTimeoutSeconds = 300

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

	if fc.MCP.ExecutionTimeoutSeconds != 0 {
		cfg.MCP.ExecutionTimeoutSeconds = fc.MCP.ExecutionTimeoutSeconds
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

	cfg.MCP.ExecutionTimeoutSeconds = getEnvInt("OPSPILOT_MCP_EXECUTION_TIMEOUT_SECONDS", cfg.MCP.ExecutionTimeoutSeconds)
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
