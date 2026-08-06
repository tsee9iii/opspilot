package config

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnv blanks every OPSPILOT_* variable so tests are isolated from the
// host environment. Empty values are treated as unset by the loader.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OPSPILOT_CONFIG",
		"OPSPILOT_ENV",
		"OPSPILOT_HTTP_HOST",
		"OPSPILOT_HTTP_PORT",
		"OPSPILOT_DB_HOST",
		"OPSPILOT_DB_PORT",
		"OPSPILOT_DB_USER",
		"OPSPILOT_DB_PASSWORD",
		"OPSPILOT_DB_NAME",
		"OPSPILOT_DB_SSLMODE",
		"OPSPILOT_LOG_LEVEL",
		"OPSPILOT_AUTH_SERVER_SECRET",
		"OPSPILOT_MCP_EXECUTION_TIMEOUT_SECONDS",
	} {
		t.Setenv(k, "")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "central.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadDefaultsOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPSPILOT_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Host != "0.0.0.0" || cfg.HTTP.Port != 8080 {
		t.Fatalf("unexpected HTTP: %+v", cfg.HTTP)
	}
	if cfg.Database.Host != "localhost" || cfg.Database.Port != 5432 ||
		cfg.Database.User != "opspilot" || cfg.Database.Password != "opspilot" ||
		cfg.Database.Name != "opspilot" || cfg.Database.SSLMode != "disable" {
		t.Fatalf("unexpected database: %+v", cfg.Database)
	}
	if cfg.Logger.Level != "info" {
		t.Fatalf("unexpected logger: %+v", cfg.Logger)
	}
	if cfg.Auth.ServerSecret != "dev-only-secret-change-me" {
		t.Fatalf("unexpected auth: %+v", cfg.Auth)
	}
	if cfg.MCP.ExecutionTimeoutSeconds != 300 {
		t.Fatalf("unexpected mcp: %+v", cfg.MCP)
	}
	if cfg.Env != "development" {
		t.Fatalf("unexpected env: %q", cfg.Env)
	}
}

func TestLoadYAMLOnly(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
server:
  host: 10.0.0.5
  port: 9090
database:
  host: db.internal
  port: 6432
  database: prod
  username: alice
  password: s3cret
  sslmode: require
logger:
  level: debug
auth:
  server_secret: yaml-secret
`)
	t.Setenv("OPSPILOT_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Host != "10.0.0.5" || cfg.HTTP.Port != 9090 {
		t.Fatalf("unexpected HTTP: %+v", cfg.HTTP)
	}
	if cfg.Database.Host != "db.internal" || cfg.Database.Port != 6432 ||
		cfg.Database.Name != "prod" || cfg.Database.User != "alice" ||
		cfg.Database.Password != "s3cret" || cfg.Database.SSLMode != "require" {
		t.Fatalf("unexpected database: %+v", cfg.Database)
	}
	if cfg.Logger.Level != "debug" {
		t.Fatalf("unexpected logger: %+v", cfg.Logger)
	}
	if cfg.Auth.ServerSecret != "yaml-secret" {
		t.Fatalf("unexpected auth: %+v", cfg.Auth)
	}
}

func TestLoadEnvironmentOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPSPILOT_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv("OPSPILOT_HTTP_HOST", "1.2.3.4")
	t.Setenv("OPSPILOT_HTTP_PORT", "9999")
	t.Setenv("OPSPILOT_DB_HOST", "env.db")
	t.Setenv("OPSPILOT_DB_PORT", "7000")
	t.Setenv("OPSPILOT_DB_USER", "bob")
	t.Setenv("OPSPILOT_DB_PASSWORD", "env-pass")
	t.Setenv("OPSPILOT_DB_NAME", "envdb")
	t.Setenv("OPSPILOT_DB_SSLMODE", "verify-full")
	t.Setenv("OPSPILOT_LOG_LEVEL", "warn")
	t.Setenv("OPSPILOT_AUTH_SERVER_SECRET", "env-secret")
	t.Setenv("OPSPILOT_ENV", "production")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Host != "1.2.3.4" || cfg.HTTP.Port != 9999 {
		t.Fatalf("unexpected HTTP: %+v", cfg.HTTP)
	}
	if cfg.Database.Host != "env.db" || cfg.Database.Port != 7000 ||
		cfg.Database.User != "bob" || cfg.Database.Password != "env-pass" ||
		cfg.Database.Name != "envdb" || cfg.Database.SSLMode != "verify-full" {
		t.Fatalf("unexpected database: %+v", cfg.Database)
	}
	if cfg.Logger.Level != "warn" {
		t.Fatalf("unexpected logger: %+v", cfg.Logger)
	}
	if cfg.Auth.ServerSecret != "env-secret" {
		t.Fatalf("unexpected auth: %+v", cfg.Auth)
	}
	if cfg.Env != "production" {
		t.Fatalf("unexpected env: %q", cfg.Env)
	}
}

func TestLoadYAMLWithEnvironmentOverride(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
server:
  host: 10.0.0.5
  port: 9090
database:
  host: db.internal
  port: 6432
  database: prod
  username: alice
  password: s3cret
  sslmode: require
logger:
  level: debug
auth:
  server_secret: yaml-secret
`)
	t.Setenv("OPSPILOT_CONFIG", path)
	t.Setenv("OPSPILOT_HTTP_HOST", "env-host")
	t.Setenv("OPSPILOT_DB_PORT", "7000")
	t.Setenv("OPSPILOT_DB_PASSWORD", "env-pass")
	t.Setenv("OPSPILOT_AUTH_SERVER_SECRET", "env-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Overridden by env.
	if cfg.HTTP.Host != "env-host" {
		t.Fatalf("env should override host, got %q", cfg.HTTP.Host)
	}
	if cfg.Database.Port != 7000 {
		t.Fatalf("env should override db port, got %d", cfg.Database.Port)
	}
	if cfg.Database.Password != "env-pass" {
		t.Fatalf("env should override password, got %q", cfg.Database.Password)
	}
	if cfg.Auth.ServerSecret != "env-secret" {
		t.Fatalf("env should override server_secret, got %q", cfg.Auth.ServerSecret)
	}
	// Left to YAML.
	if cfg.HTTP.Port != 9090 {
		t.Fatalf("yaml should keep port, got %d", cfg.HTTP.Port)
	}
	if cfg.Database.Host != "db.internal" || cfg.Database.User != "alice" ||
		cfg.Database.Name != "prod" || cfg.Database.SSLMode != "require" {
		t.Fatalf("yaml should keep other db values: %+v", cfg.Database)
	}
	if cfg.Logger.Level != "debug" {
		t.Fatalf("yaml should keep log level, got %q", cfg.Logger.Level)
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPSPILOT_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("missing config must not be an error: %v", err)
	}
	if cfg.HTTP.Port != 8080 {
		t.Fatalf("expected defaults, got %+v", cfg.HTTP)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, "server: [unclosed\n  bad: :::")
	t.Setenv("OPSPILOT_CONFIG", path)

	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadPartialYAML(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
server:
  host: 10.0.0.5
database:
  host: db.internal
`)
	t.Setenv("OPSPILOT_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// Provided values are honored, the rest stay default.
	if cfg.HTTP.Host != "10.0.0.5" || cfg.HTTP.Port != 8080 {
		t.Fatalf("unexpected HTTP: %+v", cfg.HTTP)
	}
	if cfg.Database.Host != "db.internal" || cfg.Database.Port != 5432 {
		t.Fatalf("unexpected database: %+v", cfg.Database)
	}
}

func TestLoadInstallerGeneratedYAML(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
server:
  host: 0.0.0.0
  port: 8080

database:
  host:
  port:
  database:
  username:
  password:
`)
	t.Setenv("OPSPILOT_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("installer-generated YAML must load: %v", err)
	}
	if cfg.HTTP.Host != "0.0.0.0" || cfg.HTTP.Port != 8080 {
		t.Fatalf("unexpected HTTP: %+v", cfg.HTTP)
	}
	// Empty database block must fall back to defaults.
	if cfg.Database.Host != "localhost" || cfg.Database.Port != 5432 ||
		cfg.Database.User != "opspilot" || cfg.Database.Password != "opspilot" ||
		cfg.Database.Name != "opspilot" {
		t.Fatalf("unexpected database: %+v", cfg.Database)
	}
}

func TestLoadOPSPILOTConfigOverride(t *testing.T) {
	clearEnv(t)
	// A YAML file with non-default values proves the override path is used.
	path := writeConfig(t, `
server:
  host: override-host
  port: 1234
`)
	t.Setenv("OPSPILOT_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HTTP.Host != "override-host" || cfg.HTTP.Port != 1234 {
		t.Fatalf("OPSPILOT_CONFIG not honored: %+v", cfg.HTTP)
	}
}

func TestConfigPathDefault(t *testing.T) {
	if got := configPath(); got != "/etc/opspilot/central.yaml" {
		t.Fatalf("expected default config path, got %q", got)
	}
}
