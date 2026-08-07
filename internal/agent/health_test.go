package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agent"
)

// metricTool is a registry stub whose Execute returns a canned JSON payload per
// registered name, letting the collector be exercised without real syscalls.
type metricTool struct {
	name string
	json string
}

func (t *metricTool) Name() string                               { return t.name }
func (t *metricTool) Version() string                            { return "0.0.1" }
func (t *metricTool) Description() string                        { return "metric stub" }
func (t *metricTool) ParameterSchema() string                    { return agent.EmptyParameterSchema }
func (t *metricTool) ConfirmationLevel() agent.ConfirmationLevel { return agent.ConfirmationNone }
func (t *metricTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:              t.Name(),
		Description:       t.Description(),
		Category:          agent.CategorySystem,
		Tags:              []string{"test"},
		Risk:              agent.RiskReadOnly,
		EstimatedDuration: agent.DurationShort,
	}
}
func (t *metricTool) Availability(context.Context) (bool, string) { return true, "" }
func (t *metricTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	return []byte(t.json), nil
}

func healthConfig() *agent.Config {
	return &agent.Config{
		Version:              "0.1.0",
		AgentID:              "agent-1",
		Server:               agent.ServerInfo{Hostname: "host-1", Environment: "prod"},
		HealthReportInterval: 60,
	}
}

func TestHealthCollectorGathersMetrics(t *testing.T) {
	r := agent.NewRegistry()
	for _, tool := range []agent.Tool{
		&metricTool{name: "system.cpu", json: `{"user_percent":10,"system_percent":5,"idle_percent":85}`},
		&metricTool{name: "system.memory", json: `{"used_percent":61.5}`},
		&metricTool{name: "system.disk", json: `{"used_percent":42.7}`},
	} {
		if err := r.Register(tool); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	c := agent.NewHealthCollector(healthConfig(), r, zap.NewNop())
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if report.Status != "ok" {
		t.Fatalf("expected ok status, got %q", report.Status)
	}
	if report.CPUUserPercent != 10 || report.CPUSystemPercent != 5 || report.CPUIdlePercent != 85 {
		t.Fatalf("unexpected cpu: %+v", report)
	}
	if report.MemoryUsedPercent != 61.5 || report.DiskUsedPercent != 42.7 {
		t.Fatalf("unexpected memory/disk: %+v", report)
	}
	if report.AgentVersion != "0.1.0" || report.Hostname != "host-1" || report.Environment != "prod" {
		t.Fatalf("unexpected identity: %+v", report)
	}
}

func TestHealthCollectorDegradesOnMissingMetric(t *testing.T) {
	r := agent.NewRegistry()
	if err := r.Register(&metricTool{name: "system.cpu", json: `{"user_percent":1,"system_percent":1,"idle_percent":98}`}); err != nil {
		t.Fatalf("register: %v", err)
	}

	c := agent.NewHealthCollector(healthConfig(), r, zap.NewNop())
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if report.Status != "degraded" {
		t.Fatalf("expected degraded status when a metric is unavailable, got %q", report.Status)
	}
}

func TestHealthCollectorChecksProjectHealth(t *testing.T) {
	r := agent.NewRegistry()
	for _, tool := range []agent.Tool{
		&metricTool{name: "system.cpu", json: `{"user_percent":1,"system_percent":1,"idle_percent":98}`},
		&metricTool{name: "system.memory", json: `{"used_percent":50}`},
		&metricTool{name: "system.disk", json: `{"used_percent":20}`},
		&metricTool{name: "http.check", json: `{"reachable":true,"status_code":500,"healthy":false}`},
	} {
		if err := r.Register(tool); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	c := agent.NewHealthCollector(healthConfig(), r, zap.NewNop())
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	// No projects configured, so no probe runs.
	if report.ProjectHealth != nil {
		t.Fatalf("expected no project probe, got %+v", report.ProjectHealth)
	}

	// Verify the report marshals as valid JSON (central consumes it verbatim).
	if _, err := json.Marshal(report); err != nil {
		t.Fatalf("marshal report: %v", err)
	}
}
