package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// healthReport is the full health report the agent submits to central. It
// carries normalized typed fields (used for alerting) plus the complete raw
// snapshot of collected metrics and project probes.
type healthReport struct {
	AgentID           string              `json:"agent_id"`
	ReportedAt        time.Time           `json:"reported_at"`
	AgentVersion      string              `json:"agent_version"`
	Hostname          string              `json:"hostname"`
	Environment       string              `json:"environment"`
	Status            string              `json:"status"`
	CPUUserPercent    float64             `json:"cpu_user_percent"`
	CPUSystemPercent  float64             `json:"cpu_system_percent"`
	CPUIdlePercent    float64             `json:"cpu_idle_percent"`
	MemoryUsedPercent float64             `json:"memory_used_percent"`
	DiskUsedPercent   float64             `json:"disk_used_percent"`
	ProjectHealth     *projectHealthProbe `json:"project_health,omitempty"`
}

// projectHealthProbe is the outcome of a project's health check. When several
// projects are configured, the probe reports the first unhealthy project (the
// most actionable signal); the full per-project results stay in the raw
// snapshot.
type projectHealthProbe struct {
	Project    string `json:"project"`
	Healthy    bool   `json:"healthy"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// HealthCollector gathers the agent's runtime metrics and project health into a
// health report. It reuses the registered system.* and http.check tools, so the
// report is always consistent with what the tools actually measure.
type HealthCollector struct {
	cfg      *Config
	registry *Registry
	log      *zap.Logger
}

func NewHealthCollector(cfg *Config, registry *Registry, log *zap.Logger) *HealthCollector {
	return &HealthCollector{cfg: cfg, registry: registry, log: log}
}

// Collect assembles a health report at the current time. A failed or
// unavailable metric is recorded as degraded (status "degraded") rather than
// failing the report: a single broken metric should not hide the rest.
func (c *HealthCollector) Collect(ctx context.Context) (*healthReport, error) {
	report := &healthReport{
		AgentID:      c.cfg.AgentID,
		ReportedAt:   time.Now().UTC(),
		AgentVersion: c.cfg.Version,
		Hostname:     c.cfg.Server.Hostname,
		Environment:  c.cfg.Server.Environment,
		Status:       "ok",
	}

	if err := c.collectMetrics(ctx, report); err != nil {
		return nil, err
	}
	c.collectProjectHealth(ctx, report)

	return report, nil
}

func (c *HealthCollector) collectMetrics(ctx context.Context, report *healthReport) error {
	var degraded []string

	cpu, err := c.toolJSON(ctx, "system.cpu")
	if err == nil {
		var r struct {
			UserPercent   float64 `json:"user_percent"`
			SystemPercent float64 `json:"system_percent"`
			IdlePercent   float64 `json:"idle_percent"`
		}
		if json.Unmarshal(cpu, &r) == nil {
			report.CPUUserPercent = r.UserPercent
			report.CPUSystemPercent = r.SystemPercent
			report.CPUIdlePercent = r.IdlePercent
		} else {
			degraded = append(degraded, "cpu")
		}
	} else {
		degraded = append(degraded, "cpu")
	}

	mem, err := c.toolJSON(ctx, "system.memory")
	if err == nil {
		var r struct {
			UsedPercent float64 `json:"used_percent"`
		}
		if json.Unmarshal(mem, &r) == nil {
			report.MemoryUsedPercent = r.UsedPercent
		} else {
			degraded = append(degraded, "memory")
		}
	} else {
		degraded = append(degraded, "memory")
	}

	disk, err := c.toolJSON(ctx, "system.disk")
	if err == nil {
		var r struct {
			UsedPercent float64 `json:"used_percent"`
		}
		if json.Unmarshal(disk, &r) == nil {
			report.DiskUsedPercent = r.UsedPercent
		} else {
			degraded = append(degraded, "disk")
		}
	} else {
		degraded = append(degraded, "disk")
	}

	if len(degraded) > 0 {
		report.Status = "degraded"
	}
	return nil
}

func (c *HealthCollector) collectProjectHealth(ctx context.Context, report *healthReport) {
	projects := c.cfg.Projects()
	for _, p := range projects {
		if p.HealthURL == nil {
			continue
		}
		probe, err := c.checkProject(ctx, p.Name, *p.HealthURL)
		if err != nil {
			c.log.Warn("project health check failed",
				zap.String("project", p.Name),
				zap.String("url", *p.HealthURL),
				zap.Error(err),
			)
			continue
		}
		if report.ProjectHealth == nil {
			report.ProjectHealth = probe
		}
		if !probe.Healthy {
			// The first unhealthy probe is the actionable signal; keep it.
			report.ProjectHealth = probe
			return
		}
	}
}

// checkProject probes a project health endpoint through the registered
// http.check tool, so the SSRF hardening and policy apply.
func (c *HealthCollector) checkProject(ctx context.Context, name, url string) (*projectHealthProbe, error) {
	tool, ok := c.registry.Find("http.check")
	if !ok {
		return &projectHealthProbe{Project: name, URL: url, Healthy: false, Error: "http.check tool unavailable"}, nil
	}
	payload, err := json.Marshal(map[string]any{
		"url":             url,
		"expected_status": 200,
		"timeout_seconds": 10,
	})
	if err != nil {
		return nil, err
	}
	out, err := tool.Execute(ctx, payload)
	if err != nil {
		return &projectHealthProbe{Project: name, URL: url, Healthy: false, Error: err.Error()}, nil
	}
	var result struct {
		Healthy    bool `json:"healthy"`
		StatusCode int  `json:"status_code"`
		Reachable  bool `json:"reachable"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("health: decode http.check result: %w", err)
	}
	probe := &projectHealthProbe{
		Project:    name,
		URL:        url,
		Healthy:    result.Healthy,
		StatusCode: result.StatusCode,
	}
	if !result.Healthy && !result.Reachable {
		probe.Error = "endpoint unreachable"
	}
	return probe, nil
}

// toolJSON executes a registered tool with an empty payload and returns its
// raw JSON result.
func (c *HealthCollector) toolJSON(ctx context.Context, name string) ([]byte, error) {
	tool, ok := c.registry.Find(name)
	if !ok {
		return nil, fmt.Errorf("health: tool %s not registered", name)
	}
	if available, reason := tool.Availability(ctx); !available {
		return nil, fmt.Errorf("health: tool %s unavailable: %s", name, reason)
	}
	return tool.Execute(ctx, nil)
}
