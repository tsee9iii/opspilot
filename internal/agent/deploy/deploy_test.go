package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// commandRunner records invocations and returns canned CommandResult-shaped
// JSON per invocation.
type commandRunner struct {
	calls   [][]string
	results []agent.CommandResult
	idx     int
}

func newCommandRunner(results ...agent.CommandResult) *commandRunner {
	return &commandRunner{results: results}
}

func (r *commandRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if r.idx < len(r.results) {
		res := r.results[r.idx]
		r.idx++
		b, _ := json.Marshal(res)
		return b, nil
	}
	b, _ := json.Marshal(agent.CommandResult{ExitCode: 0})
	return b, nil
}

func okResult() agent.CommandResult {
	return agent.CommandResult{Stdout: "ok", ExitCode: 0}
}

func failResult() agent.CommandResult {
	return agent.CommandResult{Stderr: "boom", ExitCode: 1}
}

// deployContext returns a DeployContext for the given deploy config, rooted at
// a fixed project path so tests can assert resolved command arguments.
func deployContext(cfg *project.DeployConfig) DeployContext {
	return DeployContext{
		Project: project.Project{
			Name:       "svc",
			Repository: "/srv/svc",
			Deploy:     cfg,
		},
		WorkingDir:   "/srv/svc",
		DeployConfig: *cfg,
	}
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	r.Register(NewDockerComposeStrategy())
	r.Register(NewPM2Strategy())
	r.Register(NewScriptStrategy())

	if _, ok := r.Get(project.StrategyDockerCompose); !ok {
		t.Fatal("expected docker-compose strategy")
	}
	if _, ok := r.Get(project.StrategyPM2); !ok {
		t.Fatal("expected pm2 strategy")
	}
	if _, ok := r.Get(project.StrategyScript); !ok {
		t.Fatal("expected script strategy")
	}
	if _, ok := r.Get("kubernetes"); ok {
		t.Fatal("expected no strategy for unregistered type")
	}
}

func TestRegistryRegisterNilOrEmpty(t *testing.T) {
	r := NewRegistry()
	r.Register(nil)
	r.Register(&emptyStrategy{})
	if len(r.strategies) != 0 {
		t.Fatalf("expected empty registry, got %d", len(r.strategies))
	}
}

type emptyStrategy struct{}

func (s *emptyStrategy) Type() string {
	return ""
}

func (s *emptyStrategy) Validate(DeployContext) error {
	return nil
}

func (s *emptyStrategy) Deploy(context.Context, DeployContext) error {
	return nil
}

func TestStrategyTypes(t *testing.T) {
	want := map[string]string{
		project.StrategyDockerCompose: NewDockerComposeStrategy().Type(),
		project.StrategyPM2:           NewPM2Strategy().Type(),
		project.StrategyScript:        NewScriptStrategy().Type(),
	}
	for typ, got := range want {
		if got != typ {
			t.Fatalf("strategy %s reported type %s", typ, got)
		}
	}
}

func TestDockerComposeStrategyDeploy(t *testing.T) {
	runner := newCommandRunner(okResult(), okResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "docker-compose.yml"})

	if err := NewDockerComposeStrategyWithRun(runner.run).Deploy(context.Background(), dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := [][]string{
		{"docker", "compose", "-f", "/srv/svc/docker-compose.yml", "pull"},
		{"docker", "compose", "-f", "/srv/svc/docker-compose.yml", "up", "-d"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("unexpected calls: %v", runner.calls)
	}
}

func TestDockerComposeStrategyAbsoluteComposeFile(t *testing.T) {
	runner := newCommandRunner(okResult(), okResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "/opt/stack/compose.yml"})

	if err := NewDockerComposeStrategyWithRun(runner.run).Deploy(context.Background(), dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.calls[0][3] != "/opt/stack/compose.yml" {
		t.Fatalf("expected absolute compose file: %v", runner.calls[0])
	}
}

func TestDockerComposeStrategyPullFailure(t *testing.T) {
	runner := newCommandRunner(failResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "docker-compose.yml"})

	err := NewDockerComposeStrategyWithRun(runner.run).Deploy(context.Background(), dc)
	if err == nil {
		t.Fatal("expected pull failure")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected pull only, got %d calls", len(runner.calls))
	}
}

func TestDockerComposeStrategyMissingComposeFile(t *testing.T) {
	runner := newCommandRunner()
	dc := deployContext(&project.DeployConfig{Type: project.StrategyDockerCompose})
	if err := NewDockerComposeStrategyWithRun(runner.run).Deploy(context.Background(), dc); err == nil {
		t.Fatal("expected error for missing compose_file")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no commands when compose_file is missing")
	}
}

func TestPM2StrategyReload(t *testing.T) {
	runner := newCommandRunner(okResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"})

	if err := NewPM2StrategyWithRun(runner.run).Deploy(context.Background(), dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected a single pm2 reload: %v", runner.calls)
	}
	if !reflect.DeepEqual(runner.calls[0], []string{"pm2", "reload", "merchant-api"}) {
		t.Fatalf("unexpected call: %v", runner.calls[0])
	}
}

func TestPM2StrategyReloadFallbackToRestart(t *testing.T) {
	runner := newCommandRunner(failResult(), okResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"})

	if err := NewPM2StrategyWithRun(runner.run).Deploy(context.Background(), dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [][]string{
		{"pm2", "reload", "merchant-api"},
		{"pm2", "restart", "merchant-api"},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("unexpected calls: %v", runner.calls)
	}
}

func TestPM2StrategyBothFail(t *testing.T) {
	runner := newCommandRunner(failResult(), failResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"})

	err := NewPM2StrategyWithRun(runner.run).Deploy(context.Background(), dc)
	if err == nil {
		t.Fatal("expected error when both reload and restart fail")
	}
}

func TestPM2StrategyMissingProcess(t *testing.T) {
	runner := newCommandRunner()
	dc := deployContext(&project.DeployConfig{Type: project.StrategyPM2})
	if err := NewPM2StrategyWithRun(runner.run).Deploy(context.Background(), dc); err == nil {
		t.Fatal("expected error for missing process")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no commands when process is missing")
	}
}

func TestScriptStrategyRelativePath(t *testing.T) {
	runner := newCommandRunner(okResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"})

	if err := NewScriptStrategyWithRun(runner.run).Deploy(context.Background(), dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected a single script invocation: %v", runner.calls)
	}
	if runner.calls[0][0] != "/srv/svc/deploy.sh" {
		t.Fatalf("expected script resolved against the working dir: %v", runner.calls[0])
	}
}

func TestScriptStrategyAbsolutePath(t *testing.T) {
	runner := newCommandRunner(okResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyScript, Script: "/opt/scripts/deploy-billing.sh"})

	if err := NewScriptStrategyWithRun(runner.run).Deploy(context.Background(), dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.calls[0][0] != "/opt/scripts/deploy-billing.sh" {
		t.Fatalf("expected absolute script used as-is: %v", runner.calls[0])
	}
}

func TestScriptStrategyFailure(t *testing.T) {
	runner := newCommandRunner(failResult())
	dc := deployContext(&project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"})

	err := NewScriptStrategyWithRun(runner.run).Deploy(context.Background(), dc)
	if err == nil {
		t.Fatal("expected script failure")
	}
}

func TestScriptStrategyMissingScript(t *testing.T) {
	runner := newCommandRunner()
	dc := deployContext(&project.DeployConfig{Type: project.StrategyScript})
	if err := NewScriptStrategyWithRun(runner.run).Deploy(context.Background(), dc); err == nil {
		t.Fatal("expected error for missing script")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no commands when script is missing")
	}
}

func TestDockerComposeStrategyValidateOK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("version: '3'"), 0o644); err != nil {
		t.Fatal(err)
	}
	dc := DeployContext{
		WorkingDir:   dir,
		DeployConfig: project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "compose.yml"},
	}
	if err := NewDockerComposeStrategy().Validate(dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerComposeStrategyValidateMissingFile(t *testing.T) {
	dc := DeployContext{
		WorkingDir:   t.TempDir(),
		DeployConfig: project.DeployConfig{Type: project.StrategyDockerCompose, ComposeFile: "nope.yml"},
	}
	err := NewDockerComposeStrategy().Validate(dc)
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected ToolError, got: %v", err)
	}
	if te.Code != "compose_file_not_found" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
}

func TestDockerComposeStrategyValidateMissingConfig(t *testing.T) {
	dc := DeployContext{
		WorkingDir:   t.TempDir(),
		DeployConfig: project.DeployConfig{Type: project.StrategyDockerCompose},
	}
	err := NewDockerComposeStrategy().Validate(dc)
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected ToolError, got: %v", err)
	}
	if te.Code != "compose_file_not_configured" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
}

func TestPM2StrategyValidateOK(t *testing.T) {
	dc := deployContext(&project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"})
	if err := NewPM2Strategy().Validate(dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPM2StrategyValidateMissingProcess(t *testing.T) {
	dc := deployContext(&project.DeployConfig{Type: project.StrategyPM2})
	err := NewPM2Strategy().Validate(dc)
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected ToolError, got: %v", err)
	}
	if te.Code != "pm2_process_not_configured" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
}

func TestScriptStrategyValidateOK(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "deploy.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dc := DeployContext{
		WorkingDir:   dir,
		DeployConfig: project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"},
	}
	if err := NewScriptStrategy().Validate(dc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScriptStrategyValidateNotExecutable(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "deploy.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dc := DeployContext{
		WorkingDir:   dir,
		DeployConfig: project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"},
	}
	err := NewScriptStrategy().Validate(dc)
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected ToolError, got: %v", err)
	}
	if te.Code != "deploy_script_not_executable" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
}

func TestScriptStrategyValidateMissingFile(t *testing.T) {
	dc := DeployContext{
		WorkingDir:   t.TempDir(),
		DeployConfig: project.DeployConfig{Type: project.StrategyScript, Script: "./deploy.sh"},
	}
	err := NewScriptStrategy().Validate(dc)
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected ToolError, got: %v", err)
	}
	if te.Code != "deploy_script_not_found" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	run := func(context.Context, string, ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}
	_, err := runCommand(context.Background(), run, "docker")
	if err == nil || !strings.Contains(err.Error(), "is not installed") {
		t.Fatalf("expected not installed error, got: %v", err)
	}
}
