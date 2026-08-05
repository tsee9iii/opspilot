package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/opspilot/opspilot/internal/agent/project"
)

func runDiagnose(t *testing.T, fe *fakeExecutor, p project.Project, wf Workflow) Result {
	t.Helper()
	return NewExecutor(fe).StopOnFailure(false).Execute(context.Background(), p, wf)
}

// loadDiagnoseProject builds a project via the Project Loader with the given
// restart/log tools and optional health URL.
func loadDiagnoseProject(t *testing.T, restart project.ToolConfig, logs project.ToolConfig, healthURL *string) project.Project {
	t.Helper()
	cfg := project.Config{
		Name:       "backend",
		Repository: "/srv/backend",
		Tools: map[string]project.ToolConfig{
			"restart": restart,
			"logs":    logs,
		},
	}
	if healthURL != nil {
		cfg.HealthURL = healthURL
	}
	l, err := project.New([]project.Config{cfg})
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	p, ok := l.FindProject("backend")
	if !ok {
		t.Fatal("project backend not found")
	}
	return p
}

func ref(tool string, params string) project.ToolReference {
	return project.ToolReference{Tool: tool, Parameters: json.RawMessage(params)}
}

func TestBuildDiagnoseWorkflowDockerProject(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
		project.ToolConfig{Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		strPtr("http://localhost:3000/health"),
	)
	wf, err := BuildDiagnoseWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "diagnose" {
		t.Fatalf("unexpected workflow name: %s", wf.Name)
	}

	wantNames := []string{"system.cpu", "system.memory", "system.disk", "system.processes", "docker.ps", "docker.logs", "http.check"}
	assertWorkflowSteps(t, wf, wantNames)

	for _, step := range wf.Steps[:4] {
		if string(step.Tool.Parameters) != "{}" {
			t.Fatalf("system step %s params should be {}: %s", step.Name, step.Tool.Parameters)
		}
	}
	if string(wf.Steps[4].Tool.Parameters) != "{}" {
		t.Fatalf("docker.ps params should be {}: %s", wf.Steps[4].Tool.Parameters)
	}
	if wf.Steps[5].Tool.Tool != "docker.logs" || string(wf.Steps[5].Tool.Parameters) != `{"container":"backend"}` {
		t.Fatalf("logs step not reused exactly: %+v", wf.Steps[5].Tool)
	}
	if wf.Steps[6].Tool.Tool != "http.check" || string(wf.Steps[6].Tool.Parameters) != `{"url":"http://localhost:3000/health"}` {
		t.Fatalf("health step unexpected: %+v", wf.Steps[6].Tool)
	}
}

func TestBuildDiagnoseWorkflowPM2Project(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "pm2.restart", Params: map[string]any{"process": "frontend"}},
		project.ToolConfig{Tool: "pm2.logs", Params: map[string]any{"process": "frontend"}},
		nil,
	)
	wf, err := BuildDiagnoseWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantNames := []string{"system.cpu", "system.memory", "system.disk", "system.processes", "pm2.list", "pm2.logs"}
	assertWorkflowSteps(t, wf, wantNames)
	if wf.Steps[4].Tool.Tool != "pm2.list" || string(wf.Steps[4].Tool.Parameters) != "{}" {
		t.Fatalf("pm2.list step unexpected: %+v", wf.Steps[4].Tool)
	}
	if wf.Steps[5].Tool.Tool != "pm2.logs" || string(wf.Steps[5].Tool.Parameters) != `{"process":"frontend"}` {
		t.Fatalf("logs step not reused exactly: %+v", wf.Steps[5].Tool)
	}
}

func TestBuildDiagnoseWorkflowSystemctlProject(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "systemctl.restart", Params: map[string]any{"service": "nginx"}},
		project.ToolConfig{Tool: "journal.logs", Params: map[string]any{"service": "nginx", "lines": 100}},
		nil,
	)
	wf, err := BuildDiagnoseWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantNames := []string{"system.cpu", "system.memory", "system.disk", "system.processes", "systemctl.status", "journal.logs"}
	assertWorkflowSteps(t, wf, wantNames)
	if wf.Steps[4].Tool.Tool != "systemctl.status" || string(wf.Steps[4].Tool.Parameters) != `{"service":"nginx"}` {
		t.Fatalf("systemctl.status step must reuse restart parameters exactly: %+v", wf.Steps[4].Tool)
	}
	if wf.Steps[5].Tool.Tool != "journal.logs" || string(wf.Steps[5].Tool.Parameters) != `{"lines":100,"service":"nginx"}` {
		t.Fatalf("logs step not reused exactly: %+v", wf.Steps[5].Tool)
	}
}

func TestBuildDiagnoseWorkflowNoHealthCheck(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "pm2.restart", Params: map[string]any{"process": "frontend"}},
		project.ToolConfig{Tool: "pm2.logs", Params: map[string]any{"process": "frontend"}},
		nil,
	)
	wf, err := BuildDiagnoseWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, step := range wf.Steps {
		if step.Tool.Tool == "http.check" {
			t.Fatalf("http.check step must be omitted without health URL: %+v", step)
		}
	}
}

func TestBuildDiagnoseWorkflowOptionalLogs(t *testing.T) {
	p := project.Project{
		Name:       "backend",
		Repository: "/srv/backend",
		Tools: map[string]project.ToolReference{
			"restart": ref("pm2.restart", `{"process":"frontend"}`),
		},
	}
	wf, err := BuildDiagnoseWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNames := []string{"system.cpu", "system.memory", "system.disk", "system.processes", "pm2.list"}
	assertWorkflowSteps(t, wf, wantNames)
}

func TestBuildDiagnoseWorkflowNoPlatformStep(t *testing.T) {
	p := project.Project{
		Name:       "backend",
		Repository: "/srv/backend",
		Tools:      map[string]project.ToolReference{},
	}
	wf, err := BuildDiagnoseWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNames := []string{"system.cpu", "system.memory", "system.disk", "system.processes"}
	assertWorkflowSteps(t, wf, wantNames)
}

func TestDiagnoseExecutionOrder(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
		project.ToolConfig{Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		strPtr("http://localhost:3000/health"),
	)
	wf, _ := BuildDiagnoseWorkflow(p)
	fe := newFakeExecutor()
	runDiagnose(t, fe, p, wf)

	wantTools := []string{"system.cpu", "system.memory", "system.disk", "system.processes", "docker.ps", "docker.logs", "http.check"}
	if len(fe.tools) != len(wantTools) {
		t.Fatalf("expected %d executions, got %d: %v", len(wantTools), len(fe.tools), fe.tools)
	}
	for i, tool := range fe.tools {
		if tool != wantTools[i] {
			t.Fatalf("unexpected execution order: %v", fe.tools)
		}
	}
}

func TestDiagnoseExactParameters(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "systemctl.restart", Params: map[string]any{"service": "nginx"}},
		project.ToolConfig{Tool: "journal.logs", Params: map[string]any{"service": "nginx", "lines": 100}},
		strPtr("http://localhost:3000/health"),
	)
	wf, _ := BuildDiagnoseWorkflow(p)
	fe := newFakeExecutor()
	runDiagnose(t, fe, p, wf)

	wantParams := []string{
		`{}`,
		`{}`,
		`{}`,
		`{}`,
		`{"service":"nginx"}`,
		`{"lines":100,"service":"nginx"}`,
		`{"url":"http://localhost:3000/health"}`,
	}
	if len(fe.params) != len(wantParams) {
		t.Fatalf("expected %d payloads, got %d: %s", len(wantParams), len(fe.params), fe.params)
	}
	for i, want := range wantParams {
		if string(fe.params[i]) != want {
			t.Fatalf("payload %d: got %s, want %s", i, fe.params[i], want)
		}
	}
}

func TestDiagnoseFailureDoesNotStopWorkflow(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
		project.ToolConfig{Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		strPtr("http://localhost:3000/health"),
	)
	wf, _ := BuildDiagnoseWorkflow(p)
	fe := newFakeExecutor()
	fe.fail("system.memory", "cannot read /proc/meminfo")

	res := runDiagnose(t, fe, p, wf)

	wantTools := []string{"system.cpu", "system.memory", "system.disk", "system.processes", "docker.ps", "docker.logs", "http.check"}
	if len(fe.tools) != len(wantTools) {
		t.Fatalf("expected all %d steps to execute, got %d: %v", len(wantTools), len(fe.tools), fe.tools)
	}
	if len(res.Steps) != len(wantTools) {
		t.Fatalf("expected %d step results, got %d", len(wantTools), len(res.Steps))
	}
	if res.Steps[1].Status != StepFailed || res.Steps[1].Error != "cannot read /proc/meminfo" {
		t.Fatalf("unexpected failed step: %+v", res.Steps[1])
	}
	if res.Steps[2].Status != StepCompleted {
		t.Fatalf("workflow must continue after failure: %+v", res.Steps[2])
	}
	for _, sr := range res.Steps {
		if sr.Status == StepSkipped {
			t.Fatalf("diagnose must not mark steps skipped: %+v", sr)
		}
	}
}

func TestDiagnoseSuccessWithPartialFailures(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
		project.ToolConfig{Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		strPtr("http://localhost:3000/health"),
	)
	wf, _ := BuildDiagnoseWorkflow(p)
	fe := newFakeExecutor()
	fe.fail("system.disk", "statfs failed")
	fe.fail("http.check", "request timed out")

	res := runDiagnose(t, fe, p, wf)

	if !res.Success {
		t.Fatalf("expected success with partial failures: %+v", res)
	}
	failed := 0
	completed := 0
	for _, sr := range res.Steps {
		switch sr.Status {
		case StepFailed:
			failed++
		case StepCompleted:
			completed++
		}
	}
	if failed != 2 || completed != 5 {
		t.Fatalf("unexpected step outcomes: %d failed, %d completed", failed, completed)
	}
}

func TestDiagnoseAllStepsFail(t *testing.T) {
	p := loadDiagnoseProject(t,
		project.ToolConfig{Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
		project.ToolConfig{Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
		strPtr("http://localhost:3000/health"),
	)
	wf, _ := BuildDiagnoseWorkflow(p)
	fe := newFakeExecutor()
	for _, tool := range []string{"system.cpu", "system.memory", "system.disk", "system.processes", "docker.ps", "docker.logs", "http.check"} {
		fe.fail(tool, "boom")
	}

	res := runDiagnose(t, fe, p, wf)

	if res.Success {
		t.Fatal("expected failure when every step fails")
	}
	if len(res.Steps) != 7 {
		t.Fatalf("expected all 7 steps to run, got %d", len(res.Steps))
	}
	for _, sr := range res.Steps {
		if sr.Status != StepFailed {
			t.Fatalf("expected all steps failed: %+v", sr)
		}
	}
}

func assertWorkflowSteps(t *testing.T, wf Workflow, wantNames []string) {
	t.Helper()
	if len(wf.Steps) != len(wantNames) {
		t.Fatalf("expected %d steps, got %d: %v", len(wantNames), len(wf.Steps), stepNames(wf))
	}
	for i, want := range wantNames {
		if wf.Steps[i].Name != want {
			t.Fatalf("step %d: got %s, want %s (all: %v)", i, wf.Steps[i].Name, want, stepNames(wf))
		}
	}
}

func stepNames(wf Workflow) []string {
	names := make([]string, len(wf.Steps))
	for i, step := range wf.Steps {
		names[i] = step.Name
	}
	return names
}
