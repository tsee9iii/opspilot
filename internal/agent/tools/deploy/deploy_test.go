package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	agentdeploy "github.com/tsee9iii/opspilot/internal/agent/deploy"
	"github.com/tsee9iii/opspilot/internal/agent/project"
	"github.com/tsee9iii/opspilot/internal/agent/workflow"
)

func strPtr(s string) *string {
	return &s
}

func testLoader(t *testing.T) *project.Loader {
	t.Helper()
	l, err := project.New([]project.Config{
		{
			Name:   "central",
			Path:   "/opt/opspilot",
			Health: &project.HealthConfig{URL: strPtr("http://127.0.0.1:9090/healthz")},
			Deploy: &project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "docker-compose.yml"},
		},
		{
			Name:   "merchant-api",
			Path:   "/srv/merchant-api",
			Health: &project.HealthConfig{URL: strPtr("http://127.0.0.1:3000/health")},
			Deploy: &project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"},
		},
		{
			Name:   "billing",
			Path:   "/srv/billing",
			Health: &project.HealthConfig{URL: strPtr("http://127.0.0.1:8080/health")},
			Deploy: &project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"},
		},
		{
			Name:   "edge",
			Path:   "/srv/edge",
			Deploy: &project.DeployConfig{Type: "kustomize"},
		},
		{
			Name:       "backend",
			Repository: "/srv/backend",
			Tools: map[string]project.ToolConfig{
				"restart": {Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
				"logs":    {Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("load projects: %v", err)
	}
	return l
}

type fakeExecutor struct {
	results map[string]func([]byte) ([]byte, error)
	tools   []string
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{results: map[string]func([]byte) ([]byte, error){}}
}

func (f *fakeExecutor) Execute(_ context.Context, tool string, payload []byte) ([]byte, error) {
	f.tools = append(f.tools, tool)
	if fn, ok := f.results[tool]; ok {
		return fn(payload)
	}
	return []byte(`{}`), nil
}

func (f *fakeExecutor) ok(tool string) {
	f.results[tool] = func([]byte) ([]byte, error) { return []byte(`{}`), nil }
}

func (f *fakeExecutor) fail(tool, message string) {
	f.results[tool] = func([]byte) ([]byte, error) { return nil, errors.New(message) }
}

// commandRunner records strategy command invocations and returns canned
// CommandResult-shaped JSON.
type commandRunner struct {
	calls   [][]string
	results []agent.CommandResult
	idx     int
}

func newCommandRunner(results ...agent.CommandResult) *commandRunner {
	return &commandRunner{results: results}
}

func (r *commandRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	res := agent.CommandResult{ExitCode: 0}
	if r.idx < len(r.results) {
		res = r.results[r.idx]
		r.idx++
	}
	b, _ := json.Marshal(res)
	return b, nil
}

func strategyRegistry(t *testing.T) (*agentdeploy.Registry, *commandRunner) {
	t.Helper()
	runner := newCommandRunner(
		agent.CommandResult{ExitCode: 0},
		agent.CommandResult{ExitCode: 0},
	)
	reg := agentdeploy.NewRegistry()
	reg.Register(agentdeploy.NewDockerComposeStrategyWithRun(runner.run))
	reg.Register(agentdeploy.NewPM2StrategyWithRun(runner.run))
	reg.Register(agentdeploy.NewScriptStrategyWithRun(runner.run))
	return reg, runner
}

// deployDirs carries the temp project directories backing strategyLoader.
type deployDirs struct {
	central string
	billing string
}

// strategyLoader returns a loader whose strategy-configured projects live in
// temp directories, so the strategies' Validate filesystem checks succeed.
func strategyLoader(t *testing.T) (*project.Loader, deployDirs) {
	t.Helper()
	centralDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(centralDir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	billingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(billingDir, "deploy.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	l, err := project.New([]project.Config{
		{
			Name:   "central",
			Path:   centralDir,
			Health: &project.HealthConfig{URL: strPtr("http://127.0.0.1:9090/healthz")},
			Deploy: &project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "docker-compose.yml"},
		},
		{
			Name:   "merchant-api",
			Path:   t.TempDir(),
			Health: &project.HealthConfig{URL: strPtr("http://127.0.0.1:3000/health")},
			Deploy: &project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"},
		},
		{
			Name:   "billing",
			Path:   billingDir,
			Health: &project.HealthConfig{URL: strPtr("http://127.0.0.1:8080/health")},
			Deploy: &project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"},
		},
	})
	if err != nil {
		t.Fatalf("load projects: %v", err)
	}
	return l, deployDirs{central: centralDir, billing: billingDir}
}

func TestDeployToolMetadata(t *testing.T) {
	tool := NewDeployTool(newFakeExecutor(), testLoader(t), "test-1.2.3")
	if tool.Name() != ToolDeploy {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatalf("expected version and description")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationRequired {
		t.Fatalf("deploying must require confirmation: %+v", tool.ConfirmationLevel())
	}
	avail, reason := tool.Availability(context.Background())
	if !avail || reason != "" {
		t.Fatalf("deploy tool must always be available: %v %q", avail, reason)
	}
	var schema json.RawMessage
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
}

func TestDeployProjectToolMetadata(t *testing.T) {
	reg, _ := strategyRegistry(t)
	tool := NewDeployProjectTool(testLoader(t), reg)
	if tool.Name() != ToolDeployProject {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatalf("expected version and description")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationRequired {
		t.Fatalf("deploying must require confirmation: %+v", tool.ConfirmationLevel())
	}
	if !reflect.DeepEqual(tool.ParameterSchema(), toolSchema) && !reflect.DeepEqual(tool.ParameterSchema(), toolProjectSchema) {
		t.Fatalf("unexpected schema: %s", tool.ParameterSchema())
	}
}

func TestDeployToolInvalidPayload(t *testing.T) {
	tool := NewDeployTool(newFakeExecutor(), testLoader(t), "test-1.2.3")
	_, err := tool.Execute(context.Background(), []byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
	_, err = tool.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}

func TestDeployToolUnknownProject(t *testing.T) {
	tool := NewDeployTool(newFakeExecutor(), testLoader(t), "test-1.2.3")
	_, err := tool.Execute(context.Background(), []byte(`{"project":"missing"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected a structured ToolError, got: %v", err)
	}
	if te.Code != "project_not_found" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
}

func TestDeployToolSuccess(t *testing.T) {
	loader := testLoader(t)
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["deploy.project"] = func([]byte) ([]byte, error) { return []byte(`{"status":"deployed"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":true}`), nil }

	tool := NewDeployTool(fe, loader, "test-1.2.3")
	out, err := tool.Execute(context.Background(), []byte(`{"project":"central"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Workflow != "deploy" || report.Status != "completed" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Version != workflow.DeployWorkflowVersion || report.AgentVersion != "test-1.2.3" {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if len(report.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(report.Steps))
	}
	for i, step := range report.Steps {
		if step.Status != "completed" {
			t.Fatalf("step %d not completed: %+v", i, step)
		}
	}
	wantTools := []string{"git.status", "git.pull", "deploy.project", "http.check"}
	if !reflect.DeepEqual(fe.tools, wantTools) {
		t.Fatalf("unexpected execution order: %v", fe.tools)
	}
}

func TestDeployToolDirtyRepository(t *testing.T) {
	loader := testLoader(t)
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":true}`), nil }

	tool := NewDeployTool(fe, loader, "test-1.2.3")
	out, err := tool.Execute(context.Background(), []byte(`{"project":"central"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != "failed" {
		t.Fatalf("expected failed status: %+v", report)
	}
	if len(fe.tools) != 1 {
		t.Fatalf("expected deployment to stop after git.status: %v", fe.tools)
	}
	if report.Steps[0].Status != "completed" {
		t.Fatalf("git.status must stay completed: %+v", report.Steps[0])
	}
	for _, step := range report.Steps[1:] {
		if step.Status != "skipped" {
			t.Fatalf("expected remaining steps skipped: %+v", report.Steps)
		}
	}
}

func TestDeployToolGitPullFailure(t *testing.T) {
	loader := testLoader(t)
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.fail("git.pull", "pull failed")

	tool := NewDeployTool(fe, loader, "test-1.2.3")
	out, err := tool.Execute(context.Background(), []byte(`{"project":"central"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != "failed" {
		t.Fatalf("expected failed status: %+v", report)
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected execution to stop after git.pull: %v", fe.tools)
	}
	if report.Steps[1].Status != "failed" || report.Steps[2].Status != "skipped" {
		t.Fatalf("unexpected step statuses: %+v", report.Steps)
	}
}

func TestDeployToolHealthFailure(t *testing.T) {
	loader := testLoader(t)
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["deploy.project"] = func([]byte) ([]byte, error) { return []byte(`{"status":"deployed"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":false}`), nil }

	tool := NewDeployTool(fe, loader, "test-1.2.3")
	out, err := tool.Execute(context.Background(), []byte(`{"project":"central"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != "failed" {
		t.Fatalf("expected failed status: %+v", report)
	}
	if report.Steps[3].Status != "failed" {
		t.Fatalf("expected health step failed: %+v", report.Steps[3])
	}
}

func TestDeployProjectToolUnknownProject(t *testing.T) {
	reg, _ := strategyRegistry(t)
	tool := NewDeployProjectTool(testLoader(t), reg)
	_, err := tool.Execute(context.Background(), []byte(`{"project":"missing"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) || te.Code != "project_not_found" {
		t.Fatalf("expected project_not_found, got: %v", err)
	}
}

func TestDeployProjectToolNoStrategy(t *testing.T) {
	reg, _ := strategyRegistry(t)
	tool := NewDeployProjectTool(testLoader(t), reg)
	_, err := tool.Execute(context.Background(), []byte(`{"project":"backend"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) || te.Code != "no_deploy_strategy" {
		t.Fatalf("expected no_deploy_strategy, got: %v", err)
	}
}

func TestDeployProjectToolUnsupportedStrategy(t *testing.T) {
	reg, _ := strategyRegistry(t)
	tool := NewDeployProjectTool(testLoader(t), reg)
	_, err := tool.Execute(context.Background(), []byte(`{"project":"edge"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) || te.Code != "unsupported_deploy_strategy" {
		t.Fatalf("expected unsupported_deploy_strategy, got: %v", err)
	}
}

func TestDeployProjectToolSuccess(t *testing.T) {
	reg, runner := strategyRegistry(t)
	loader, dirs := strategyLoader(t)
	tool := NewDeployProjectTool(loader, reg)

	out, err := tool.Execute(context.Background(), []byte(`{"project":"billing"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["project"] != "billing" || result["strategy"] != "script" || result["status"] != "deployed" {
		t.Fatalf("unexpected result: %v", result)
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != filepath.Join(dirs.billing, "deploy.sh") {
		t.Fatalf("expected the script strategy to run: %v", runner.calls)
	}
}

func TestDeployProjectToolValidateBeforeDeploy(t *testing.T) {
	reg, runner := strategyRegistry(t)

	dir := t.TempDir()
	loader, err := project.New([]project.Config{{
		Name:   "missing-compose",
		Path:   dir,
		Deploy: &project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "nope.yml"},
	}})
	if err != nil {
		t.Fatalf("load projects: %v", err)
	}

	tool := NewDeployProjectTool(loader, reg)
	_, err = tool.Execute(context.Background(), []byte(`{"project":"missing-compose"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) || te.Code != "compose_file_not_found" {
		t.Fatalf("expected compose_file_not_found before any deploy, got: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no strategy commands before validation passes: %v", runner.calls)
	}
}

const stubGitSchema = `{"type":"object","required":["repository"],"properties":{"repository":{"type":"string"}}}`

const stubHTTPSchema = `{"type":"object","required":["url"],"properties":{"url":{"type":"string"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":60}}}`

// stubTool is a minimal agent.Tool returning a canned result, used to stand in
// for git.status, git.pull and http.check when exercising the full registry
// executor path.
type stubTool struct {
	name   string
	schema string
	result []byte
}

func (s stubTool) Name() string                                    { return s.name }
func (s stubTool) Version() string                                 { return "1.0.0" }
func (s stubTool) Description() string                             { return "stub" }
func (s stubTool) ParameterSchema() string                         { return s.schema }
func (s stubTool) ConfirmationLevel() agent.ConfirmationLevel      { return agent.ConfirmationNone }
func (s stubTool) Availability(context.Context) (bool, string)     { return true, "" }
func (s stubTool) Execute(context.Context, []byte) ([]byte, error) { return s.result, nil }

func (s stubTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:              s.Name(),
		Description:       s.Description(),
		Category:          agent.CategorySystem,
		Tags:              []string{"test"},
		Risk:              agent.RiskReadOnly,
		EstimatedDuration: agent.DurationShort,
	}
}

// TestDeployToolThroughRegistryExecutor exercises the full production path for
// each strategy: workflow.deploy is dispatched like any other command through
// the RegistryExecutor, which applies the registry lookup, policy gate and
// JSON Schema payload validation before the workflow runs. The deploy.project
// step resolves the project's strategy through the registry and executes it.
func TestDeployToolThroughRegistryExecutor(t *testing.T) {
	loader, dirs := strategyLoader(t)
	cases := []struct {
		name      string
		project   string
		wantCalls [][]string
	}{
		{
			name:    "docker-compose",
			project: "central",
			wantCalls: [][]string{
				{"docker", "compose", "-f", filepath.Join(dirs.central, "docker-compose.yml"), "pull"},
				{"docker", "compose", "-f", filepath.Join(dirs.central, "docker-compose.yml"), "up", "-d"},
			},
		},
		{
			name:      "pm2",
			project:   "merchant-api",
			wantCalls: [][]string{{"pm2", "reload", "merchant-api"}},
		},
		{
			name:      "script",
			project:   "billing",
			wantCalls: [][]string{{filepath.Join(dirs.billing, "deploy.sh")}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := agent.NewRegistry()
			for _, stub := range []stubTool{
				{name: "git.status", schema: stubGitSchema, result: []byte(`{"dirty":false}`)},
				{name: "git.pull", schema: stubGitSchema, result: []byte(`{"updated":true}`)},
				{name: "http.check", schema: stubHTTPSchema, result: []byte(`{"healthy":true}`)},
			} {
				if err := registry.Register(stub); err != nil {
					t.Fatalf("register stub: %v", err)
				}
			}

			strategies, runner := strategyRegistry(t)
			if err := registry.Register(NewDeployProjectTool(loader, strategies)); err != nil {
				t.Fatalf("register deploy.project: %v", err)
			}
			exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})
			if err := registry.Register(NewDeployTool(exec, loader, "test-1.2.3")); err != nil {
				t.Fatalf("register workflow.deploy: %v", err)
			}

			out, err := exec.Execute(context.Background(), ToolDeploy, []byte(`{"project":"`+tc.project+`"}`))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var report workflow.Report
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatalf("decode report: %v", err)
			}
			if report.Status != "completed" {
				t.Fatalf("expected completed status: %+v", report)
			}
			if report.Version != workflow.DeployWorkflowVersion || report.AgentVersion != "test-1.2.3" {
				t.Fatalf("unexpected report metadata: %+v", report)
			}
			if len(report.Steps) != 4 {
				t.Fatalf("expected 4 steps, got %d", len(report.Steps))
			}
			for i, step := range report.Steps {
				if step.Status != "completed" {
					t.Fatalf("step %d not completed: %+v", i, step)
				}
			}
			if !reflect.DeepEqual(runner.calls, tc.wantCalls) {
				t.Fatalf("unexpected strategy calls: %v", runner.calls)
			}
		})
	}
}

func TestDeployToolSchemaRejectedByRegistry(t *testing.T) {
	loader := testLoader(t)
	registry := agent.NewRegistry()
	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})
	if err := registry.Register(NewDeployTool(exec, loader, "test-1.2.3")); err != nil {
		t.Fatalf("register workflow.deploy: %v", err)
	}

	_, err := exec.Execute(context.Background(), ToolDeploy, []byte(`{"project":123}`))
	if err == nil {
		t.Fatal("expected schema validation to reject a non-string project")
	}
}

func TestDeployToolUnregisteredDenied(t *testing.T) {
	exec := agent.NewRegistryExecutor(agent.NewRegistry(), agent.ExecutionPolicy{Enabled: true})
	_, err := exec.Execute(context.Background(), ToolDeploy, nil)
	if !errors.Is(err, agent.ErrToolNotImplemented) {
		t.Fatalf("expected ErrToolNotImplemented, got: %v", err)
	}
}
