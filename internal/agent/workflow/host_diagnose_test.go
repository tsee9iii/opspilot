package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

func cmdOut(stdout, stderr string, exit int) []byte {
	b, _ := json.Marshal(agent.CommandResult{Stdout: stdout, Stderr: stderr, ExitCode: exit})
	return b
}

func newHostExecutor() *fakeExecutor {
	return newFakeExecutor()
}

func TestBuildHostDiagnoseWorkflowNoService(t *testing.T) {
	wf := BuildHostDiagnoseWorkflow("")

	wantNames := []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps"}
	assertWorkflowSteps(t, wf, wantNames)
	for _, step := range wf.Steps {
		if string(step.Tool.Parameters) != "{}" {
			t.Fatalf("step %s params should be {}: %s", step.Name, step.Tool.Parameters)
		}
	}
}

func TestBuildHostDiagnoseWorkflowWithService(t *testing.T) {
	wf := BuildHostDiagnoseWorkflow("nginx")

	wantNames := []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps", "systemctl.status"}
	assertWorkflowSteps(t, wf, wantNames)
	if wf.Steps[5].Tool.Tool != "systemctl.status" || string(wf.Steps[5].Tool.Parameters) != `{"service":"nginx"}` {
		t.Fatalf("systemctl.status step unexpected: %+v", wf.Steps[5].Tool)
	}
}

func TestHostDiagnoseReportShape(t *testing.T) {
	fe := newHostExecutor()
	fe.results["system.uptime"] = func([]byte) ([]byte, error) { return cmdOut("up 5 days", "", 0), nil }

	res := runDiagnose(t, fe, project.Project{}, BuildHostDiagnoseWorkflow("nginx"))
	out, err := json.Marshal(reportFromResult(res))
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	for _, key := range []string{"workflow", "status", "started_at", "completed_at", "steps"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("report missing key %s: %s", key, out)
		}
	}
	if decoded["workflow"] != "diagnose" || decoded["status"] != "completed" {
		t.Fatalf("unexpected report: %s", out)
	}
	steps, ok := decoded["steps"].([]any)
	if !ok || len(steps) != 6 {
		t.Fatalf("expected 6 steps: %s", out)
	}
	first, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("step not an object: %s", out)
	}
	for _, key := range []string{"name", "status", "duration_ms", "stdout", "stderr"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("step missing key %s: %s", key, out)
		}
	}
}

func TestHostDiagnoseCommandResultStdoutStderr(t *testing.T) {
	fe := newHostExecutor()
	fe.results["system.uptime"] = func([]byte) ([]byte, error) { return cmdOut(" 10:00 up 5 days", "", 0), nil }
	fe.results["system.disk"] = func([]byte) ([]byte, error) { return cmdOut("", "statfs failed", 1), nil }

	report := RunHostDiagnose(context.Background(), fe, "")

	if report.Steps[0].Stdout != " 10:00 up 5 days" || report.Steps[0].Stderr != "" {
		t.Fatalf("unexpected uptime output: %+v", report.Steps[0])
	}
	if report.Steps[3].Stderr != "statfs failed" {
		t.Fatalf("unexpected disk stderr: %+v", report.Steps[3])
	}
}

func TestHostDiagnoseStructuredOutputPreserved(t *testing.T) {
	fe := newHostExecutor()
	fe.results["systemctl.status"] = func([]byte) ([]byte, error) {
		return []byte(`{"service":"nginx","active_state":"inactive"}`), nil
	}

	report := RunHostDiagnose(context.Background(), fe, "nginx")

	step := report.Steps[5]
	if step.Stdout != `{"service":"nginx","active_state":"inactive"}` {
		t.Fatalf("structured output not preserved: %+v", step)
	}
}

func TestHostDiagnoseDockerUnavailable(t *testing.T) {
	fe := newHostExecutor()
	for _, tool := range []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "systemctl.status"} {
		fe.ok(tool)
	}
	fe.fail("docker.ps", "docker is not installed")

	report := RunHostDiagnose(context.Background(), fe, "nginx")

	if report.Status != "completed" {
		t.Fatalf("workflow must succeed despite docker being unavailable: %+v", report)
	}
	if len(report.Steps) != 6 {
		t.Fatalf("expected all 6 steps recorded, got %d", len(report.Steps))
	}
	if report.Steps[4].Status != "failed" || report.Steps[4].Stderr != "docker is not installed" {
		t.Fatalf("docker.ps step unexpected: %+v", report.Steps[4])
	}
	if report.Steps[5].Status != "completed" {
		t.Fatalf("workflow must continue after docker failure: %+v", report.Steps[5])
	}
}

func TestHostDiagnoseServiceInactive(t *testing.T) {
	fe := newHostExecutor()
	for _, tool := range []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps"} {
		fe.ok(tool)
	}
	fe.results["systemctl.status"] = func([]byte) ([]byte, error) {
		return []byte(`{"service":"nginx","active_state":"inactive"}`), nil
	}

	report := RunHostDiagnose(context.Background(), fe, "nginx")

	if report.Status != "completed" {
		t.Fatalf("expected completed status: %+v", report)
	}
	step := report.Steps[5]
	if step.Status != "completed" {
		t.Fatalf("inactive service is still a completed step: %+v", step)
	}
}

func TestHostDiagnoseOneCapabilityFails(t *testing.T) {
	fe := newHostExecutor()
	for _, tool := range []string{"system.uptime", "system.memory", "system.disk", "docker.ps"} {
		fe.ok(tool)
	}
	fe.fail("system.cpu", "cannot read /proc/stat")

	report := RunHostDiagnose(context.Background(), fe, "")

	if report.Status != "completed" {
		t.Fatalf("workflow must succeed when a single check fails: %+v", report)
	}
	if report.Steps[1].Status != "failed" {
		t.Fatalf("expected system.cpu failed: %+v", report.Steps[1])
	}
}

func TestHostDiagnoseAllCapabilitiesSucceed(t *testing.T) {
	fe := newHostExecutor()
	for _, tool := range []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps", "systemctl.status"} {
		fe.ok(tool)
	}

	report := RunHostDiagnose(context.Background(), fe, "nginx")

	if report.Status != "completed" {
		t.Fatalf("expected completed status: %+v", report)
	}
	for _, step := range report.Steps {
		if step.Status != "completed" {
			t.Fatalf("expected all steps completed: %+v", step)
		}
	}
}

func TestHostDiagnoseAllStepsFail(t *testing.T) {
	fe := newHostExecutor()
	for _, tool := range []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps"} {
		fe.fail(tool, "boom")
	}

	report := RunHostDiagnose(context.Background(), fe, "")

	if report.Status != "failed" {
		t.Fatalf("expected failed status when no step completes: %+v", report)
	}
	if len(report.Steps) != 5 {
		t.Fatalf("expected all 5 steps recorded, got %d", len(report.Steps))
	}
}

func TestHostDiagnoseStepDuration(t *testing.T) {
	fe := newHostExecutor()
	fe.ok("system.uptime")
	fe.ok("system.cpu")
	fe.ok("system.memory")
	fe.ok("system.disk")
	fe.ok("docker.ps")

	report := RunHostDiagnose(context.Background(), fe, "")

	for i, step := range report.Steps {
		if step.DurationMS < 0 {
			t.Fatalf("step %d has negative duration: %+v", i, step)
		}
	}
	if report.StartedAt.IsZero() || report.CompletedAt.IsZero() {
		t.Fatal("expected workflow timestamps to be set")
	}
	if report.StartedAt.After(report.CompletedAt) {
		t.Fatal("workflow completed before it started")
	}
}

func TestHostDiagnoseExactToolOrder(t *testing.T) {
	fe := newHostExecutor()

	runDiagnose(t, fe, project.Project{}, BuildHostDiagnoseWorkflow("nginx"))

	wantTools := []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps", "systemctl.status"}
	if len(fe.tools) != len(wantTools) {
		t.Fatalf("expected %d executions, got %d: %v", len(wantTools), len(fe.tools), fe.tools)
	}
	for i, tool := range fe.tools {
		if tool != wantTools[i] {
			t.Fatalf("unexpected execution order: %v", fe.tools)
		}
	}
	if string(fe.params[5]) != `{"service":"nginx"}` {
		t.Fatalf("unexpected systemctl.status payload: %s", fe.params[5])
	}
}
