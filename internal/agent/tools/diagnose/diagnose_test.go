package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/tools/docker"
	"github.com/tsee9iii/opspilot/internal/agent/tools/system"
	"github.com/tsee9iii/opspilot/internal/agent/tools/systemctl"
	"github.com/tsee9iii/opspilot/internal/agent/workflow"
)

// fakeExecutor records invocations and returns per-tool canned results or
// errors, so the tool layer is tested deterministically.
type fakeExecutor struct {
	results map[string]func([]byte) ([]byte, error)
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{results: map[string]func([]byte) ([]byte, error){}}
}

func (f *fakeExecutor) Execute(_ context.Context, tool string, payload []byte) ([]byte, error) {
	if fn, ok := f.results[tool]; ok {
		return fn(payload)
	}
	return []byte(`{}`), nil
}

func (f *fakeExecutor) fail(tool, message string) {
	f.results[tool] = func([]byte) ([]byte, error) { return nil, errors.New(message) }
}

func cmdOut(stdout, stderr string, exit int) []byte {
	b, _ := json.Marshal(agent.CommandResult{Stdout: stdout, Stderr: stderr, ExitCode: exit})
	return b
}

func TestDiagnoseToolMetadata(t *testing.T) {
	tool := NewDiagnoseTool(newFakeExecutor(), "test-1.2.3")
	if tool.Name() != ToolDiagnose {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatalf("expected version and description")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("expected no confirmation: %+v", tool.ConfirmationLevel())
	}
	avail, reason := tool.Availability(context.Background())
	if !avail || reason != "" {
		t.Fatalf("diagnose tool must always be available: %v %q", avail, reason)
	}
	if tool.ParameterSchema() == "" {
		t.Fatal("expected a parameter schema")
	}
}

func TestDiagnoseToolInvalidPayload(t *testing.T) {
	tool := NewDiagnoseTool(newFakeExecutor(), "test-1.2.3")
	_, err := tool.Execute(context.Background(), []byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestDiagnoseToolExecute(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["system.uptime"] = func([]byte) ([]byte, error) { return cmdOut("up 5 days", "", 0), nil }
	fe.fail("docker.ps", "docker is not installed")

	tool := NewDiagnoseTool(fe, "test-1.2.3")
	out, err := tool.Execute(context.Background(), []byte(`{"service":"nginx"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.DiagnoseReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Workflow != "diagnose" || report.Status != "completed" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Version != workflow.DiagnoseWorkflowVersion {
		t.Fatalf("unexpected workflow version: %s", report.Version)
	}
	if report.AgentVersion != "test-1.2.3" {
		t.Fatalf("unexpected agent version: %s", report.AgentVersion)
	}
	if report.Hostname == "" {
		t.Fatal("expected report hostname to be set")
	}
	if report.DurationMS < 0 {
		t.Fatalf("unexpected total duration: %d", report.DurationMS)
	}
	if report.CompletedAt.Before(report.StartedAt) {
		t.Fatal("report completed before it started")
	}
	if len(report.Steps) != 6 {
		t.Fatalf("expected 6 steps, got %d", len(report.Steps))
	}
	if report.Steps[0].Stdout != "up 5 days" {
		t.Fatalf("unexpected uptime stdout: %+v", report.Steps[0])
	}
	if report.Steps[4].Status != "failed" || report.Steps[4].Stderr != "docker is not installed" {
		t.Fatalf("docker.ps step unexpected: %+v", report.Steps[4])
	}
	if report.Steps[5].Name != "systemctl.status" {
		t.Fatalf("expected systemctl.status step: %+v", report.Steps[5])
	}
}

func TestDiagnoseToolExecuteEmptyPayload(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["system.uptime"] = func([]byte) ([]byte, error) { return cmdOut("up 5 days", "", 0), nil }

	tool := NewDiagnoseTool(fe, "test-1.2.3")
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.DiagnoseReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if len(report.Steps) != 5 {
		t.Fatalf("expected 5 steps without a service, got %d", len(report.Steps))
	}
}

func TestDiagnoseToolDockerPermissionDenied(t *testing.T) {
	fe := newFakeExecutor()
	fe.results["system.uptime"] = func([]byte) ([]byte, error) { return cmdOut("up 5 days", "", 0), nil }
	fe.results["docker.ps"] = func([]byte) ([]byte, error) {
		return nil, &agent.ToolError{
			Code:       "docker_permission_denied",
			Message:    "The opspilot user is not a member of the docker group.",
			Suggestion: "Run: sudo usermod -aG docker opspilot && restart the agent.",
		}
	}

	tool := NewDiagnoseTool(fe, "test-1.2.3")
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.DiagnoseReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	step := report.Steps[4]
	if step.Status != "failed" {
		t.Fatalf("expected docker.ps failed: %+v", step)
	}
	if step.ErrorCode != "docker_permission_denied" {
		t.Fatalf("unexpected error_code: %s", step.ErrorCode)
	}
	if step.Message != "The opspilot user is not a member of the docker group." {
		t.Fatalf("unexpected message: %s", step.Message)
	}
	if step.Suggestion != "Run: sudo usermod -aG docker opspilot && restart the agent." {
		t.Fatalf("unexpected suggestion: %s", step.Suggestion)
	}
}

// TestDiagnoseToolThroughRegistryExecutor exercises the full production path:
// the tool is dispatched exactly like any other command through the
// RegistryExecutor, which applies the registry lookup, policy gate and JSON
// Schema payload validation before the workflow runs.
func TestDiagnoseToolThroughRegistryExecutor(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(system.NewUptimeTool())
	registry.Register(system.NewCPUTool())
	registry.Register(system.NewMemoryTool())
	registry.Register(system.NewDiskTool())
	registry.Register(docker.NewDockerPsTool())
	registry.Register(systemctl.NewSystemCtlStatusTool())

	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})
	registry.Register(NewDiagnoseTool(exec, "test-1.2.3"))

	out, err := exec.Execute(context.Background(), ToolDiagnose, []byte(`{"service":"nginx"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report workflow.DiagnoseReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Workflow != "diagnose" {
		t.Fatalf("unexpected workflow name: %s", report.Workflow)
	}

	wantNames := []string{"system.uptime", "system.cpu", "system.memory", "system.disk", "docker.ps", "systemctl.status"}
	if len(report.Steps) != len(wantNames) {
		t.Fatalf("expected %d steps, got %d: %+v", len(wantNames), len(report.Steps), report.Steps)
	}
	for i, name := range wantNames {
		if report.Steps[i].Name != name {
			t.Fatalf("step %d: got %s, want %s", i, report.Steps[i].Name, name)
		}
		if report.Steps[i].DurationMS < 0 {
			t.Fatalf("step %d has negative duration: %+v", i, report.Steps[i])
		}
		if report.Steps[i].Status != "completed" && report.Steps[i].Status != "failed" {
			t.Fatalf("step %d has invalid status: %+v", i, report.Steps[i])
		}
	}

	// system.disk reads the root filesystem via statfs and always completes,
	// so the workflow must succeed even when the Linux-only or unavailable
	// capabilities (cpu/memory/docker/systemctl) fail on the test machine.
	if report.Status != "completed" {
		t.Fatalf("expected completed status: %+v", report)
	}
}

func TestDiagnoseToolSchemaRejectedByRegistry(t *testing.T) {
	registry := agent.NewRegistry()
	registry.Register(system.NewUptimeTool())
	registry.Register(system.NewCPUTool())
	registry.Register(system.NewMemoryTool())
	registry.Register(system.NewDiskTool())
	registry.Register(docker.NewDockerPsTool())
	registry.Register(systemctl.NewSystemCtlStatusTool())

	exec := agent.NewRegistryExecutor(registry, agent.ExecutionPolicy{Enabled: true})
	registry.Register(NewDiagnoseTool(exec, "test-1.2.3"))

	_, err := exec.Execute(context.Background(), ToolDiagnose, []byte(`{"service":123}`))
	if err == nil {
		t.Fatal("expected schema validation to reject a non-string service")
	}
}

func TestDiagnoseToolUnregisteredDenied(t *testing.T) {
	exec := agent.NewRegistryExecutor(agent.NewRegistry(), agent.ExecutionPolicy{Enabled: true})
	_, err := exec.Execute(context.Background(), "workflow.diagnose", nil)
	if !errors.Is(err, agent.ErrToolNotImplemented) {
		t.Fatalf("expected ErrToolNotImplemented, got: %v", err)
	}
}
