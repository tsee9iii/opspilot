package project

import (
	"encoding/json"
	"strings"
	"testing"
)

func backendConfig() Config {
	return Config{
		Name:       "backend",
		Repository: "/srv/backend",
		HealthURL:  strPtr("http://localhost:3000/health"),
		Tools: map[string]ToolConfig{
			"restart": {Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
			"logs":    {Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		},
	}
}

func strPtr(s string) *string {
	return &s
}

func TestNewValidConfiguration(t *testing.T) {
	l, err := New([]Config{backendConfig()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	projects := l.Projects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	p := projects[0]
	if p.Name != "backend" {
		t.Fatalf("unexpected name: %s", p.Name)
	}
	if p.Repository != "/srv/backend" {
		t.Fatalf("unexpected repository: %s", p.Repository)
	}
	if p.HealthURL == nil || *p.HealthURL != "http://localhost:3000/health" {
		t.Fatalf("unexpected health URL: %v", p.HealthURL)
	}
	restart, ok := p.Tools["restart"]
	if !ok {
		t.Fatal("missing restart tool")
	}
	if restart.Tool != "docker.restart" {
		t.Fatalf("unexpected restart tool: %s", restart.Tool)
	}
	assertParams(t, restart.Parameters, `{"container":"backend"}`)
}

func TestNewParametersAreArbitraryJSON(t *testing.T) {
	cfg := Config{
		Name:       "backend",
		Repository: "/srv/backend",
		Tools: map[string]ToolConfig{
			"restart": {
				Tool: "docker.restart",
				Params: map[string]any{
					"container": "backend",
					"limits":    map[string]any{"memory": 512},
					"enabled":   true,
				},
			},
			"logs": {Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		},
	}
	l, err := New([]Config{cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	params := l.projects[0].Tools["restart"].Parameters
	var decoded map[string]any
	if err := json.Unmarshal(params, &decoded); err != nil {
		t.Fatalf("parameters are not valid JSON: %v", err)
	}
	if decoded["container"] != "backend" {
		t.Fatalf("unexpected container param: %v", decoded["container"])
	}
	if _, ok := decoded["limits"].(map[string]any); !ok {
		t.Fatalf("nested parameters not preserved: %v", decoded["limits"])
	}
	if decoded["enabled"] != true {
		t.Fatalf("boolean parameter not preserved: %v", decoded["enabled"])
	}
}

func TestNewEmptyParametersDefaultToObject(t *testing.T) {
	cfg := Config{
		Name:       "backend",
		Repository: "/srv/backend",
		Tools: map[string]ToolConfig{
			"restart": {Tool: "docker.restart"},
			"logs":    {Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		},
	}
	l, err := New([]Config{cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertParams(t, l.projects[0].Tools["restart"].Parameters, `{}`)
}

func TestNewDuplicateProjectNames(t *testing.T) {
	_, err := New([]Config{backendConfig(), backendConfig()})
	if err == nil {
		t.Fatal("expected error for duplicate project names")
	}
	if !strings.Contains(err.Error(), "duplicate project name: backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRelativeRepositoryPath(t *testing.T) {
	cfg := backendConfig()
	cfg.Repository = "srv/backend"
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for relative repository path")
	}
	if !strings.Contains(err.Error(), "repository must be an absolute path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewInvalidHealthURL(t *testing.T) {
	cfg := backendConfig()
	cfg.HealthURL = strPtr("not-a-url")
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for invalid health URL")
	}
	if !strings.Contains(err.Error(), "invalid health URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewHealthURLOptional(t *testing.T) {
	cfg := backendConfig()
	cfg.HealthURL = nil
	l, err := New([]Config{cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.projects[0].HealthURL != nil {
		t.Fatalf("expected nil health URL, got %v", l.projects[0].HealthURL)
	}
}

func TestNewMissingRestartTool(t *testing.T) {
	cfg := backendConfig()
	delete(cfg.Tools, "restart")
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for missing restart tool")
	}
	if !strings.Contains(err.Error(), "missing restart tool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewMissingLogsTool(t *testing.T) {
	cfg := backendConfig()
	delete(cfg.Tools, "logs")
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for missing logs tool")
	}
	if !strings.Contains(err.Error(), "missing logs tool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewMissingProjectName(t *testing.T) {
	cfg := backendConfig()
	cfg.Name = ""
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for missing project name")
	}
	if !strings.Contains(err.Error(), "project name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewToolWithoutName(t *testing.T) {
	cfg := backendConfig()
	cfg.Tools["restart"] = ToolConfig{}
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for tool without a name")
	}
	if !strings.Contains(err.Error(), "has no tool name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewNoProjects(t *testing.T) {
	l, err := New(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.Projects()) != 0 {
		t.Fatalf("expected no projects, got %d", len(l.Projects()))
	}
	if _, ok := l.FindProject("anything"); ok {
		t.Fatal("expected not found")
	}
}

func TestFindProject(t *testing.T) {
	l, err := New([]Config{
		backendConfig(),
		{
			Name:       "frontend",
			Repository: "/srv/frontend",
			Tools: map[string]ToolConfig{
				"restart": {Tool: "pm2.restart", Params: map[string]any{"process": "frontend"}},
				"logs":    {Tool: "pm2.logs", Params: map[string]any{"process": "frontend"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, ok := l.FindProject("backend")
	if !ok {
		t.Fatal("expected to find backend")
	}
	if p.Name != "backend" {
		t.Fatalf("unexpected project: %s", p.Name)
	}

	if _, ok := l.FindProject("missing"); ok {
		t.Fatal("expected missing project to not be found")
	}
}

func TestProjects(t *testing.T) {
	cfg := backendConfig()
	l, err := New([]Config{cfg, {
		Name:       "frontend",
		Repository: "/srv/frontend",
		Tools: map[string]ToolConfig{
			"restart": {Tool: "pm2.restart", Params: map[string]any{"process": "frontend"}},
			"logs":    {Tool: "pm2.logs", Params: map[string]any{"process": "frontend"}},
		},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	projects := l.Projects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "backend" || projects[1].Name != "frontend" {
		t.Fatalf("unexpected order: %s, %s", projects[0].Name, projects[1].Name)
	}
}

func assertParams(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	if string(got) != want {
		t.Fatalf("unexpected parameters: %s, want %s", got, want)
	}
}

func TestNewDeployStrategyConfig(t *testing.T) {
	cfg := Config{
		Name:   "central",
		Path:   "/opt/opspilot",
		Health: &HealthConfig{URL: strPtr("http://127.0.0.1:9090/healthz")},
		Deploy: &DeployConfig{Type: "docker-compose", ComposeFile: "docker-compose.yml"},
	}
	l, err := New([]Config{cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := l.projects[0]
	if p.Repository != "/opt/opspilot" {
		t.Fatalf("unexpected repository: %s", p.Repository)
	}
	if p.HealthURL == nil || *p.HealthURL != "http://127.0.0.1:9090/healthz" {
		t.Fatalf("unexpected health URL: %v", p.HealthURL)
	}
	if p.Deploy == nil || p.Deploy.Type != "docker-compose" || p.Deploy.ComposeFile != "docker-compose.yml" {
		t.Fatalf("unexpected deploy config: %+v", p.Deploy)
	}
}

func TestNewDeployStrategyAliasesLegacyFields(t *testing.T) {
	cfg := Config{
		Name:       "central",
		Repository: "/opt/opspilot",
		HealthURL:  strPtr("http://127.0.0.1:9090/healthz"),
		Deploy:     &DeployConfig{Type: "docker-compose", ComposeFile: "docker-compose.yml"},
	}
	l, err := New([]Config{cfg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := l.projects[0]
	if p.Repository != "/opt/opspilot" {
		t.Fatalf("unexpected repository: %s", p.Repository)
	}
	if p.HealthURL == nil || *p.HealthURL != "http://127.0.0.1:9090/healthz" {
		t.Fatalf("unexpected health URL: %v", p.HealthURL)
	}
}

func TestNewDeployStrategyDoesNotRequireTools(t *testing.T) {
	cfg := Config{
		Name:   "billing",
		Path:   "/srv/billing",
		Deploy: &DeployConfig{Type: "script", Script: "./deploy.sh"},
	}
	if _, err := New([]Config{cfg}); err != nil {
		t.Fatalf("deploy projects must not require restart/logs tools: %v", err)
	}
}

func TestNewDeployWithToolsAllowed(t *testing.T) {
	cfg := Config{
		Name:   "central",
		Path:   "/opt/opspilot",
		Deploy: &DeployConfig{Type: "docker-compose", ComposeFile: "docker-compose.yml"},
		Tools: map[string]ToolConfig{
			"restart": {Tool: "docker.restart", Params: map[string]any{"container": "central"}},
			"logs":    {Tool: "docker.logs", Params: map[string]any{"container": "central"}},
		},
	}
	if _, err := New([]Config{cfg}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDeployTypeRequired(t *testing.T) {
	cfg := Config{Name: "x", Path: "/srv/x", Deploy: &DeployConfig{}}
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for missing deploy.type")
	}
	if !strings.Contains(err.Error(), "deploy.type is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDeployMissingStrategyParams(t *testing.T) {
	cases := []struct {
		cfg  Config
		want string
	}{
		{
			cfg:  Config{Name: "x", Path: "/srv/x", Deploy: &DeployConfig{Type: "docker-compose"}},
			want: "deploy.compose_file is required",
		},
		{
			cfg:  Config{Name: "x", Path: "/srv/x", Deploy: &DeployConfig{Type: "pm2"}},
			want: "deploy.process is required",
		},
		{
			cfg:  Config{Name: "x", Path: "/srv/x", Deploy: &DeployConfig{Type: "script"}},
			want: "deploy.script is required",
		},
	}
	for _, tc := range cases {
		_, err := New([]Config{tc.cfg})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("type %s: expected error containing %q, got %v", tc.cfg.Deploy.Type, tc.want, err)
		}
	}
}

func TestNewDeployUnknownTypeAllowed(t *testing.T) {
	cfg := Config{Name: "edge", Path: "/srv/edge", Deploy: &DeployConfig{Type: "kustomize"}}
	if _, err := New([]Config{cfg}); err != nil {
		t.Fatalf("unknown deploy types must pass loader validation: %v", err)
	}
}

func TestNewDeployRelativePath(t *testing.T) {
	cfg := Config{Name: "x", Path: "srv/x", Deploy: &DeployConfig{Type: "pm2", Process: "x"}}
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for relative path")
	}
	if !strings.Contains(err.Error(), "repository must be an absolute path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewDeployInvalidHealthURL(t *testing.T) {
	cfg := Config{
		Name:   "x",
		Path:   "/srv/x",
		Health: &HealthConfig{URL: strPtr("not-a-url")},
		Deploy: &DeployConfig{Type: "pm2", Process: "x"},
	}
	_, err := New([]Config{cfg})
	if err == nil {
		t.Fatal("expected error for invalid health URL")
	}
	if !strings.Contains(err.Error(), "invalid health URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}
