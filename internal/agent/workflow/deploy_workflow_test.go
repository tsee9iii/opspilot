package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent/project"
)

func deployStrategyProject(healthURL *string) project.Project {
	return project.Project{
		Name:       "merchant-api",
		Repository: "/srv/merchant-api",
		HealthURL:  healthURL,
		Deploy:     &project.DeployConfig{Type: project.StrategyPM2, Process: "merchant-api"},
	}
}

func TestBuildStrategyDeployWorkflow(t *testing.T) {
	wf, err := BuildStrategyDeployWorkflow(deployStrategyProject(strPtr("http://127.0.0.1:3000/health")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "deploy" {
		t.Fatalf("unexpected workflow name: %s", wf.Name)
	}
	if len(wf.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(wf.Steps))
	}

	want := []struct {
		name  string
		tool  string
		param string
	}{
		{"git.status", "git.status", `{"repository":"/srv/merchant-api"}`},
		{"git.pull", "git.pull", `{"repository":"/srv/merchant-api"}`},
		{"deploy.project", "deploy.project", `{"project":"merchant-api"}`},
		{"health", "http.check", ""},
	}
	for i, w := range want {
		step := wf.Steps[i]
		if step.Name != w.name || step.Tool.Tool != w.tool {
			t.Fatalf("step %d: got %s/%s, want %s/%s", i, step.Name, step.Tool.Tool, w.name, w.tool)
		}
		if w.param != "" && string(step.Tool.Parameters) != w.param {
			t.Fatalf("step %d params: got %s, want %s", i, step.Tool.Parameters, w.param)
		}
	}

	var healthParams map[string]any
	if err := json.Unmarshal(wf.Steps[3].Tool.Parameters, &healthParams); err != nil {
		t.Fatalf("decode http.check params: %v", err)
	}
	if healthParams["url"] != "http://127.0.0.1:3000/health" {
		t.Fatalf("unexpected health url: %v", healthParams)
	}
	if healthParams["timeout_seconds"] != float64(30) {
		t.Fatalf("unexpected health timeout: %v", healthParams)
	}
}

func TestBuildStrategyDeployWorkflowWithoutHealth(t *testing.T) {
	wf, err := BuildStrategyDeployWorkflow(deployStrategyProject(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(wf.Steps))
	}
	for _, step := range wf.Steps {
		if step.Tool.Tool == "http.check" {
			t.Fatalf("health step must be omitted without a health URL: %+v", step)
		}
	}
}

func TestRunStrategyDeploySuccess(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["deploy.project"] = func([]byte) ([]byte, error) { return []byte(`{"status":"deployed"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":true}`), nil }

	report := RunStrategyDeploy(context.Background(), fe, deployStrategyProject(strPtr("http://127.0.0.1:3000/health")), "test-1.2.3")

	if report.Status != "completed" {
		t.Fatalf("expected completed status: %+v", report)
	}
	if report.Workflow != "deploy" || report.Version != DeployWorkflowVersion {
		t.Fatalf("unexpected report identity: %+v", report)
	}
	if report.AgentVersion != "test-1.2.3" || report.Hostname == "" {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if report.Strategy != project.StrategyPM2 {
		t.Fatalf("expected report strategy metadata: %+v", report)
	}
	if len(report.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(report.Steps))
	}
	for i, step := range report.Steps {
		if step.Status != "completed" {
			t.Fatalf("step %d not completed: %+v", i, step)
		}
	}
}

func TestRunStrategyDeployDirtyRepository(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":true}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["deploy.project"] = func([]byte) ([]byte, error) { return []byte(`{"status":"deployed"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":true}`), nil }

	report := RunStrategyDeploy(context.Background(), fe, deployStrategyProject(strPtr("http://127.0.0.1:3000/health")), "test-1.2.3")

	if report.Status != "failed" {
		t.Fatalf("expected failed status for a dirty repository: %+v", report)
	}
	if len(fe.tools) != 1 || fe.tools[0] != "git.status" {
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

func TestRunStrategyDeployGitPullFailure(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.fail("git.pull", "pull failed")

	report := RunStrategyDeploy(context.Background(), fe, deployStrategyProject(strPtr("http://127.0.0.1:3000/health")), "test-1.2.3")

	if report.Status != "failed" {
		t.Fatalf("expected failed status: %+v", report)
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected execution to stop after git.pull: %v", fe.tools)
	}
	if report.Steps[0].Status != "completed" || report.Steps[1].Status != "failed" {
		t.Fatalf("unexpected step statuses: %+v", report.Steps)
	}
	if report.Steps[1].Stderr != "pull failed" {
		t.Fatalf("unexpected error: %+v", report.Steps[1])
	}
	if report.Steps[2].Status != "skipped" || report.Steps[3].Status != "skipped" {
		t.Fatalf("expected remaining steps skipped: %+v", report.Steps)
	}
}

func TestRunStrategyDeployDeployFailure(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.fail("deploy.project", "docker compose up failed")

	report := RunStrategyDeploy(context.Background(), fe, deployStrategyProject(strPtr("http://127.0.0.1:3000/health")), "test-1.2.3")

	if report.Status != "failed" {
		t.Fatalf("expected failed status: %+v", report)
	}
	if report.Steps[2].Status != "failed" || report.Steps[2].Stderr != "docker compose up failed" {
		t.Fatalf("unexpected deploy step: %+v", report.Steps[2])
	}
	if report.Steps[3].Status != "skipped" {
		t.Fatalf("expected health step skipped: %+v", report.Steps[3])
	}
}

func TestRunStrategyDeployHealthUnhealthy(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["deploy.project"] = func([]byte) ([]byte, error) { return []byte(`{"status":"deployed"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":false}`), nil }

	report := RunStrategyDeploy(context.Background(), fe, deployStrategyProject(strPtr("http://127.0.0.1:3000/health")), "test-1.2.3")

	if report.Status != "failed" {
		t.Fatalf("expected failed status when health check is unhealthy: %+v", report)
	}
	if report.Steps[0].Status != "completed" || report.Steps[1].Status != "completed" || report.Steps[2].Status != "completed" {
		t.Fatalf("expected first three steps completed: %+v", report.Steps)
	}
	if report.Steps[3].Status != "failed" {
		t.Fatalf("expected health step failed: %+v", report.Steps[3])
	}
	if report.Steps[3].Stderr == "" {
		t.Fatal("expected health failure reason to be recorded")
	}
}

func TestRunStrategyDeployReportShape(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["git.status"] = func([]byte) ([]byte, error) { return []byte(`{"dirty":false}`), nil }
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["deploy.project"] = func([]byte) ([]byte, error) { return []byte(`{"status":"deployed"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":true}`), nil }

	report := RunStrategyDeploy(context.Background(), fe, deployStrategyProject(strPtr("http://127.0.0.1:3000/health")), "test-1.2.3")
	out, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	for _, key := range []string{"workflow", "version", "agent_version", "hostname", "strategy", "status", "started_at", "completed_at", "duration_ms", "steps"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("report missing key %s: %s", key, out)
		}
	}
	if decoded["workflow"] != "deploy" || decoded["version"] != DeployWorkflowVersion {
		t.Fatalf("unexpected report identity: %s", out)
	}
}
