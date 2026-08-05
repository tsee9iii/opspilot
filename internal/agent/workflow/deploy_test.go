package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent/project"
)

func strPtr(s string) *string {
	return &s
}

func TestBuildDeployWorkflowWithHealthCheck(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, err := BuildDeployWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "deploy" {
		t.Fatalf("unexpected workflow name: %s", wf.Name)
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(wf.Steps))
	}

	step1 := wf.Steps[0]
	if step1.Name != "git.pull" || step1.Tool.Tool != "git.pull" {
		t.Fatalf("unexpected first step: %+v", step1)
	}
	var repoParams map[string]string
	if err := json.Unmarshal(step1.Tool.Parameters, &repoParams); err != nil {
		t.Fatalf("decode git.pull params: %v", err)
	}
	if repoParams["repository"] != "/srv/backend" {
		t.Fatalf("unexpected repository param: %v", repoParams)
	}

	step2 := wf.Steps[1]
	if step2.Name != "restart" || step2.Tool.Tool != "docker.restart" {
		t.Fatalf("unexpected restart step: %+v", step2)
	}
	var restartParams map[string]any
	if err := json.Unmarshal(step2.Tool.Parameters, &restartParams); err != nil {
		t.Fatalf("decode restart params: %v", err)
	}
	if restartParams["container"] != "backend" {
		t.Fatalf("restart tool parameters modified: %v", restartParams)
	}

	step3 := wf.Steps[2]
	if step3.Name != "health" || step3.Tool.Tool != "http.check" {
		t.Fatalf("unexpected health step: %+v", step3)
	}
	var urlParams map[string]string
	if err := json.Unmarshal(step3.Tool.Parameters, &urlParams); err != nil {
		t.Fatalf("decode http.check params: %v", err)
	}
	if urlParams["url"] != "http://localhost:3000/health" {
		t.Fatalf("unexpected url param: %v", urlParams)
	}
}

func TestBuildDeployWorkflowWithoutHealthCheck(t *testing.T) {
	p := loadTestProject(t, nil)
	wf, err := BuildDeployWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(wf.Steps))
	}
	for _, step := range wf.Steps {
		if step.Tool.Tool == "http.check" {
			t.Fatalf("http.check step must be omitted when no health URL: %+v", step)
		}
	}
}

func TestBuildDeployWorkflowMissingRestart(t *testing.T) {
	p := project.Project{
		Name:       "x",
		Repository: "/srv/x",
		Tools:      map[string]project.ToolReference{},
	}
	_, err := BuildDeployWorkflow(p)
	if err == nil {
		t.Fatal("expected error for missing restart tool")
	}
	if !strings.Contains(err.Error(), "no restart tool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeploySuccessWithHealthCheck(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, err := BuildDeployWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fe := newFakeExecutor()
	fe.results["git.pull"] = func([]byte) ([]byte, error) { return []byte(`{"updated":true}`), nil }
	fe.results["docker.restart"] = func([]byte) ([]byte, error) { return []byte(`{"status":"restarted"}`), nil }
	fe.results["http.check"] = func([]byte) ([]byte, error) { return []byte(`{"healthy":true}`), nil }

	res := run(t, fe, p, wf)

	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(res.Steps))
	}
	wantTools := []string{"git.pull", "docker.restart", "http.check"}
	if len(fe.tools) != len(wantTools) {
		t.Fatalf("expected %d executions, got %d: %v", len(wantTools), len(fe.tools), fe.tools)
	}
	for i, tool := range fe.tools {
		if tool != wantTools[i] {
			t.Fatalf("unexpected execution order: %v", fe.tools)
		}
		if string(fe.params[i]) != string(wf.Steps[i].Tool.Parameters) {
			t.Fatalf("step %d params mismatch: %s", i, fe.params[i])
		}
	}
	for i, sr := range res.Steps {
		if sr.Status != StepCompleted {
			t.Fatalf("step %d not completed: %+v", i, sr)
		}
	}
	if string(res.Steps[0].Result) != `{"updated":true}` {
		t.Fatalf("unexpected git.pull result: %s", res.Steps[0].Result)
	}
}

func TestDeploySuccessWithoutHealthCheck(t *testing.T) {
	p := loadTestProject(t, nil)
	wf, err := BuildDeployWorkflow(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.ok("docker.restart")

	res := run(t, fe, p, wf)

	if !res.Success {
		t.Fatalf("expected success: %+v", res)
	}
	if len(res.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(res.Steps))
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected 2 executions, got %d: %v", len(fe.tools), fe.tools)
	}
	if fe.tools[0] != "git.pull" || fe.tools[1] != "docker.restart" {
		t.Fatalf("unexpected execution order: %v", fe.tools)
	}
}

func TestDeployGitPullFailure(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, _ := BuildDeployWorkflow(p)
	fe := newFakeExecutor()
	fe.fail("git.pull", "pull failed")

	res := run(t, fe, p, wf)

	if res.Success {
		t.Fatal("expected failure")
	}
	if len(fe.tools) != 1 {
		t.Fatalf("expected execution to stop after git.pull, got %d: %v", len(fe.tools), fe.tools)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(res.Steps))
	}
	if res.Steps[0].Status != StepFailed || res.Steps[0].Error != "pull failed" {
		t.Fatalf("unexpected failed step: %+v", res.Steps[0])
	}
	if res.Steps[1].Status != StepSkipped || res.Steps[2].Status != StepSkipped {
		t.Fatalf("expected remaining steps skipped: %+v", res.Steps)
	}
}

func TestDeployRestartFailure(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, _ := BuildDeployWorkflow(p)
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.fail("docker.restart", "restart failed")

	res := run(t, fe, p, wf)

	if res.Success {
		t.Fatal("expected failure")
	}
	if len(fe.tools) != 2 {
		t.Fatalf("expected execution to stop after 2 tools, got %d: %v", len(fe.tools), fe.tools)
	}
	if res.Steps[0].Status != StepCompleted {
		t.Fatalf("expected git.pull completed: %+v", res.Steps[0])
	}
	if res.Steps[1].Status != StepFailed || res.Steps[1].Error != "restart failed" {
		t.Fatalf("unexpected failed step: %+v", res.Steps[1])
	}
	if res.Steps[2].Status != StepSkipped {
		t.Fatalf("expected health step skipped: %+v", res.Steps[2])
	}
}

func TestDeployHTTPCheckFailure(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, _ := BuildDeployWorkflow(p)
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.ok("docker.restart")
	fe.fail("http.check", "request timed out")

	res := run(t, fe, p, wf)

	if res.Success {
		t.Fatal("expected failure")
	}
	if len(fe.tools) != 3 {
		t.Fatalf("expected 3 executions, got %d: %v", len(fe.tools), fe.tools)
	}
	if res.Steps[0].Status != StepCompleted || res.Steps[1].Status != StepCompleted {
		t.Fatalf("expected first two steps completed: %+v", res.Steps)
	}
	if res.Steps[2].Status != StepFailed || res.Steps[2].Error != "request timed out" {
		t.Fatalf("unexpected failed step: %+v", res.Steps[2])
	}
}

func TestDeploySkippedSteps(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, _ := BuildDeployWorkflow(p)
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.fail("docker.restart", "restart failed")

	res := run(t, fe, p, wf)

	skipped := 0
	for _, sr := range res.Steps {
		if sr.Status == StepSkipped {
			skipped++
			if sr.StartedAt.IsZero() != true || sr.FinishedAt.IsZero() != true {
				t.Fatalf("skipped step should have no timestamps: %+v", sr)
			}
		}
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skipped step, got %d", skipped)
	}
}

func TestDeployParamsPassedToExecutor(t *testing.T) {
	p := loadTestProject(t, strPtr("http://localhost:3000/health"))
	wf, _ := BuildDeployWorkflow(p)
	fe := newFakeExecutor()
	fe.ok("git.pull")
	fe.ok("docker.restart")
	fe.ok("http.check")

	run(t, fe, p, wf)

	wantParams := []string{
		`{"repository":"/srv/backend"}`,
		`{"container":"backend"}`,
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
