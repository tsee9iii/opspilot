package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env    string
	HTTP   struct {
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
}

func Load() (*Config, error) {
	cfg := &Config{}
	cfg.Env = getEnv("OPSPILOT_ENV", "development")

	cfg.HTTP.Host = getEnv("OPSPILOT_HTTP_HOST", "0.0.0.0")
	cfg.HTTP.Port = getEnvInt("OPSPILOT_HTTP_PORT", 8080)

	cfg.Database.Host = getEnv("OPSPILOT_DB_HOST", "localhost")
	cfg.Database.Port = getEnvInt("OPSPILOT_DB_PORT", 5432)
	cfg.Database.User = getEnv("OPSPILOT_DB_USER", "opspilot")
	cfg.Database.Password = getEnv("OPSPILOT_DB_PASSWORD", "opspilot")
	cfg.Database.Name = getEnv("OPSPILOT_DB_NAME", "opspilot")
	cfg.Database.SSLMode = getEnv("OPSPILOT_DB_SSLMODE", "disable")

	cfg.Logger.Level = getEnv("OPSPILOT_LOG_LEVEL", "info")

	return cfg, nil
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
