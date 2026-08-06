package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// fakeExecutor stands in for the agent's RegistryExecutor. It records every
// invocation (tool name + payload) and returns per-tool canned results or
// errors, so workflow orchestration is tested without environment-dependent
// tools.
type fakeExecutor struct {
	results map[string]func(payload []byte) ([]byte, error)
	tools   []string
	params  []json.RawMessage
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{results: map[string]func([]byte) ([]byte, error){}}
}

func (f *fakeExecutor) Execute(_ context.Context, tool string, payload []byte) ([]byte, error) {
	f.tools = append(f.tools, tool)
	f.params = append(f.params, append(json.RawMessage(nil), payload...))
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

// loadTestProject builds a project through the existing Project Loader, so the
// executor is exercised with loader-produced profiles rather than raw YAML.
func loadTestProject(t *testing.T, healthURL *string) project.Project {
	t.Helper()
	l, err := project.New([]project.Config{
		{
			Name:       "backend",
			Repository: "/srv/backend",
			HealthURL:  healthURL,
			Tools: map[string]project.ToolConfig{
				"restart": {Tool: "docker.restart", Params: map[string]any{"container": "backend"}},
				"logs":    {Tool: "docker.logs", Params: map[string]any{"container": "backend"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	p, ok := l.FindProject("backend")
	if !ok {
		t.Fatal("project backend not found")
	}
	return p
}

func run(t *testing.T, fe *fakeExecutor, p project.Project, wf Workflow) Result {
	t.Helper()
	return NewExecutor(fe).Execute(context.Background(), p, wf)
}

func TestStepStatusValues(t *testing.T) {
	want := map[StepStatus]string{
		StepPending:   "pending",
		StepRunning:   "running",
		StepCompleted: "completed",
		StepFailed:    "failed",
		StepSkipped:   "skipped",
	}
	for status, value := range want {
		if string(status) != value {
			t.Fatalf("status %q != %q", status, value)
		}
	}
}

func TestExecuteEmptyWorkflow(t *testing.T) {
	p := loadTestProject(t, nil)
	res := run(t, newFakeExecutor(), p, NewWorkflow("deploy"))

	if !res.Success {
		t.Fatal("expected success")
	}
	if res.Workflow != "deploy" || res.Project != "backend" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.Steps) != 0 {
		t.Fatalf("expected no steps, got %d", len(res.Steps))
	}
	if res.StartedAt.IsZero() || res.FinishedAt.IsZero() {
		t.Fatal("expected workflow timestamps to be set")
	}
	if res.StartedAt.After(res.FinishedAt) {
		t.Fatal("workflow finished before it started")
	}
}

func TestExecuteMultipleStepsInOrder(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.ok("docker.restart")
	wf := NewWorkflow("deploy",
		Step{Name: "git.pull", Tool: project.ToolReference{Tool: "git.pull"}},
		Step{Name: "restart", Tool: p.Tools["restart"]},
	)
	res := run(t, fe, p, wf)

	if !res.Success {
		t.Fatal("expected success")
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(fe.tools))
	}
	if fe.tools[0] != "git.pull" || fe.tools[1] != "docker.restart" {
		t.Fatalf("unexpected execution order: %v", fe.tools)
	}
	if len(res.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(res.Steps))
	}
	for i, sr := range res.Steps {
		if sr.Status != StepCompleted {
			t.Fatalf("step %d not completed: %+v", i, sr)
		}
	}
}

func TestExecuteStepToolError(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.results["docker.restart"] = func([]byte) ([]byte, error) {
		return nil, &agent.ToolError{
			Code:       "docker_permission_denied",
			Message:    "The opspilot user is not a member of the docker group.",
			Suggestion: "Run: sudo usermod -aG docker opspilot && restart the agent.",
		}
	}
	wf := NewWorkflow("deploy", Step{Name: "restart", Tool: p.Tools["restart"]})
	res := run(t, fe, p, wf)

	if len(res.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(res.Steps))
	}
	sr := res.Steps[0]
	if sr.Status != StepFailed {
		t.Fatalf("expected step failed: %+v", sr)
	}
	if sr.ErrorCode != "docker_permission_denied" {
		t.Fatalf("unexpected error code: %s", sr.ErrorCode)
	}
	if sr.Message != "The opspilot user is not a member of the docker group." {
		t.Fatalf("unexpected message: %s", sr.Message)
	}
	if sr.Suggestion != "Run: sudo usermod -aG docker opspilot && restart the agent." {
		t.Fatalf("unexpected suggestion: %s", sr.Suggestion)
	}
	if sr.Error == "" {
		t.Fatal("expected error text to remain set")
	}
}

func TestExecuteStopsOnFailure(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.fail("docker.restart", "restart failed")
	fe.ok("http.check")
	wf := NewWorkflow("deploy",
		Step{Name: "pull", Tool: project.ToolReference{Tool: "git.pull"}},
		Step{Name: "restart", Tool: project.ToolReference{Tool: "docker.restart"}},
		Step{Name: "health", Tool: project.ToolReference{Tool: "http.check"}},
	)
	res := run(t, fe, p, wf)

	if res.Success {
		t.Fatal("expected failure")
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected execution to stop after 2 tools, got %d: %v", len(fe.tools), fe.tools)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(res.Steps))
	}
	if res.Steps[0].Status != StepCompleted {
		t.Fatalf("expected first step completed, got %+v", res.Steps[0])
	}
	if res.Steps[1].Status != StepFailed || res.Steps[1].Error != "restart failed" {
		t.Fatalf("unexpected failed step: %+v", res.Steps[1])
	}
	if res.Steps[2].Status != StepSkipped {
		t.Fatalf("expected remaining step skipped, got %+v", res.Steps[2])
	}
	if res.FinishedAt.Before(res.StartedAt) {
		t.Fatal("unexpected timestamps")
	}
}

func TestExecuteStepResults(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.results["docker.restart"] = func(payload []byte) ([]byte, error) {
		return []byte(`{"status":"restarted"}`), nil
	}
	wf := NewWorkflow("deploy", Step{Name: "restart", Tool: p.Tools["restart"]})
	res := run(t, fe, p, wf)

	if len(res.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(res.Steps))
	}
	sr := res.Steps[0]
	if sr.Name != "restart" || sr.Tool != "docker.restart" {
		t.Fatalf("unexpected step identity: %+v", sr)
	}
	if sr.Status != StepCompleted {
		t.Fatalf("unexpected status: %s", sr.Status)
	}
	if sr.StartedAt.IsZero() || sr.FinishedAt.IsZero() {
		t.Fatal("expected step timestamps to be set")
	}
	if sr.StartedAt.After(sr.FinishedAt) {
		t.Fatal("step finished before it started")
	}
	if string(sr.Result) != `{"status":"restarted"}` {
		t.Fatalf("unexpected tool result: %s", sr.Result)
	}
	if sr.Error != "" {
		t.Fatalf("unexpected error: %s", sr.Error)
	}
	if string(sr.Parameters) != `{"container":"backend"}` {
		t.Fatalf("unexpected parameters: %s", sr.Parameters)
	}
}

func TestExecuteStepTransitions(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("docker.restart")
	fe.fail("git.pull", "pull failed")
	wf := NewWorkflow("deploy",
		Step{Name: "pull", Tool: project.ToolReference{Tool: "git.pull"}},
		Step{Name: "restart", Tool: project.ToolReference{Tool: "docker.restart"}},
	)

	var transitions []struct {
		step   string
		status StepStatus
	}
	ex := NewExecutor(fe)
	ex.OnStep(func(step string, status StepStatus) {
		transitions = append(transitions, struct {
			step   string
			status StepStatus
		}{step, status})
	})
	ex.Execute(context.Background(), p, wf)

	want := []struct {
		step   string
		status StepStatus
	}{
		{"pull", StepPending},
		{"pull", StepRunning},
		{"pull", StepFailed},
		{"restart", StepSkipped},
	}
	if len(transitions) != len(want) {
		t.Fatalf("expected %d transitions, got %d: %+v", len(want), len(transitions), transitions)
	}
	for i, tr := range transitions {
		if tr.step != want[i].step || tr.status != want[i].status {
			t.Fatalf("transition %d: got %s/%s, want %s/%s",
				i, tr.step, tr.status, want[i].step, want[i].status)
		}
	}
}

func TestResultJSONShape(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("docker.restart")
	wf := NewWorkflow("deploy", Step{Name: "restart", Tool: p.Tools["restart"]})
	res := run(t, fe, p, wf)

	out, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if decoded["workflow"] != "deploy" || decoded["project"] != "backend" || decoded["success"] != true {
		t.Fatalf("unexpected result JSON: %s", out)
	}
}

func threeStepWorkflow() Workflow {
	return NewWorkflow("deploy",
		Step{Name: "a", Tool: project.ToolReference{Tool: "a"}},
		Step{Name: "b", Tool: project.ToolReference{Tool: "b"}},
		Step{Name: "c", Tool: project.ToolReference{Tool: "c"}},
	)
}

func TestExecuteAbortWhen(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("a")
	fe.ok("b")
	fe.ok("c")
	ex := NewExecutor(fe).AbortWhen(func(sr StepResult) bool {
		return sr.Name == "a"
	})
	res := ex.Execute(context.Background(), p, threeStepWorkflow())

	if res.Success {
		t.Fatal("expected abort to fail the workflow")
	}
	if len(fe.tools) != 1 || fe.tools[0] != "a" {
		t.Fatalf("expected only step a to run: %v", fe.tools)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(res.Steps))
	}
	if res.Steps[0].Status != StepCompleted {
		t.Fatalf("aborted step should stay completed: %+v", res.Steps[0])
	}
	if res.Steps[1].Status != StepSkipped || res.Steps[2].Status != StepSkipped {
		t.Fatalf("expected remaining steps skipped: %+v", res.Steps)
	}
}

func TestExecuteAbortWhenNoMatch(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("a")
	fe.ok("b")
	fe.ok("c")
	ex := NewExecutor(fe).AbortWhen(func(sr StepResult) bool {
		return sr.Name == "does-not-exist"
	})
	res := ex.Execute(context.Background(), p, threeStepWorkflow())

	if !res.Success {
		t.Fatal("expected success when the predicate never matches")
	}
	if len(fe.tools) != 3 {
		t.Fatalf("expected all steps to run: %v", fe.tools)
	}
}

func TestExecuteFailWhen(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("a")
	fe.ok("b")
	fe.ok("c")
	ex := NewExecutor(fe).FailWhen(func(sr StepResult) (bool, string) {
		if sr.Name == "b" {
			return true, "boom"
		}
		return false, ""
	})
	res := ex.Execute(context.Background(), p, threeStepWorkflow())

	if res.Success {
		t.Fatal("expected failure")
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected steps a and b to run: %v", fe.tools)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(res.Steps))
	}
	if res.Steps[0].Status != StepCompleted {
		t.Fatalf("expected step a completed: %+v", res.Steps[0])
	}
	if res.Steps[1].Status != StepFailed || res.Steps[1].Error != "boom" {
		t.Fatalf("expected step b failed with reason: %+v", res.Steps[1])
	}
	if res.Steps[2].Status != StepSkipped {
		t.Fatalf("expected step c skipped: %+v", res.Steps[2])
	}
}

func TestExecuteFailWhenNoMatch(t *testing.T) {
	p := loadTestProject(t, nil)
	fe := newFakeExecutor()
	fe.ok("a")
	fe.ok("b")
	fe.ok("c")
	ex := NewExecutor(fe).FailWhen(func(sr StepResult) (bool, string) {
		return false, ""
	})
	res := ex.Execute(context.Background(), p, threeStepWorkflow())

	if !res.Success {
		t.Fatal("expected success when the predicate never matches")
	}
	if len(fe.tools) != 3 {
		t.Fatalf("expected all steps to run: %v", fe.tools)
	}
	for _, sr := range res.Steps {
		if sr.Status != StepCompleted {
			t.Fatalf("expected all steps completed: %+v", res.Steps)
		}
	}
}
