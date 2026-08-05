package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	content := `central_url: http://localhost:8080
registration_token: tok
secret: secret
version: "0.1.0"
server:
  hostname: host1
  environment: prod
projects:
  - name: backend
    repository: /srv/backend
    health_url: http://localhost:3000/health
    tools:
      restart:
        tool: docker.restart
        container: backend
      logs:
        tool: docker.logs
        container: backend
  - name: frontend
    repository: /srv/frontend
    tools:
      restart:
        tool: pm2.restart
        process: frontend
      logs:
        tool: pm2.logs
        process: frontend
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projects := cfg.Projects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "backend" || projects[0].Repository != "/srv/backend" {
		t.Fatalf("unexpected project: %+v", projects[0])
	}
	if projects[0].HealthURL == nil || *projects[0].HealthURL != "http://localhost:3000/health" {
		t.Fatalf("unexpected health URL: %v", projects[0].HealthURL)
	}
	restart, ok := projects[0].Tools["restart"]
	if !ok || restart.Tool != "docker.restart" || string(restart.Parameters) != `{"container":"backend"}` {
		t.Fatalf("unexpected restart reference: %+v", restart)
	}
	if projects[1].HealthURL != nil {
		t.Fatalf("expected nil health URL for frontend, got %v", projects[1].HealthURL)
	}

	p, ok := cfg.FindProject("frontend")
	if !ok || p.Name != "frontend" {
		t.Fatalf("FindProject frontend: ok=%v p=%+v", ok, p)
	}
	if _, ok := cfg.FindProject("missing"); ok {
		t.Fatal("expected missing project to not be found")
	}
}

func TestLoadConfigInvalidProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	content := `central_url: http://localhost:8080
projects:
  - name: backend
    repository: relative/path
    tools:
      restart:
        tool: docker.restart
      logs:
        tool: docker.logs
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for relative repository path")
	}
	if !strings.Contains(err.Error(), "repository must be an absolute path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigSavePreservesProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	content := `central_url: http://localhost:8080
projects:
  - name: backend
    repository: /srv/backend
    tools:
      restart:
        tool: docker.restart
        container: backend
        restart_policy: always
      logs:
        tool: docker.logs
        container: backend
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg.AgentID = "new-id"
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(saved), "restart_policy: always") {
		t.Fatalf("tool parameters lost on save:\n%s", saved)
	}

	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AgentID != "new-id" {
		t.Fatalf("unexpected agent id: %s", reloaded.AgentID)
	}
	projects := reloaded.Projects()
	if len(projects) != 1 || projects[0].Name != "backend" {
		t.Fatalf("projects lost on save: %+v", projects)
	}
}

func TestConfigWithoutProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	content := `central_url: http://localhost:8080
registration_token: tok
secret: secret
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Projects() != nil && len(cfg.Projects()) != 0 {
		t.Fatalf("expected no projects, got %+v", cfg.Projects())
	}
	if _, ok := cfg.FindProject("anything"); ok {
		t.Fatal("expected not found")
	}
}
