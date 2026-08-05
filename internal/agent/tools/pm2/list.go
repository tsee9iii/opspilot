package pm2

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolPM2List            = "pm2.list"
	toolPM2ListVersion     = "1.0.0"
	toolPM2ListDescription = "List running PM2 processes"
)

// pm2ListResult is one running PM2 process. uptime is in seconds.
type pm2ListResult struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	PID         int     `json:"pid"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes int64   `json:"memory_bytes"`
	Uptime      int64   `json:"uptime"`
}

// pm2RawProcess is a single element of the `pm2 jlist` output.
type pm2RawProcess struct {
	Name  string `json:"name"`
	Pid   int    `json:"pid"`
	Monit struct {
		CPU    float64 `json:"cpu"`
		Memory int64   `json:"memory"`
	} `json:"monit"`
	PM2Env struct {
		Status   string `json:"status"`
		PMUptime int64  `json:"pm_uptime"`
	} `json:"pm2_env"`
}

// PM2ListTool reports running PM2 processes via the `pm2 jlist` CLI.
type PM2ListTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
	now func() time.Time
}

func NewPM2ListTool() *PM2ListTool {
	return &PM2ListTool{
		run: agent.RunCommand,
		now: time.Now,
	}
}

func (t *PM2ListTool) Name() string {
	return ToolPM2List
}

func (t *PM2ListTool) Version() string {
	return toolPM2ListVersion
}

func (t *PM2ListTool) Description() string {
	return toolPM2ListDescription
}

func (t *PM2ListTool) ParameterSchema() string {
	return agent.EmptyParameterSchema
}

func (t *PM2ListTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *PM2ListTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	out, err := t.run(ctx, "pm2", "jlist")
	if err != nil {
		return nil, err
	}

	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("pm2.list: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("pm2.list: pm2 jlist failed: %s", res.Stderr)
	}

	raw, err := parsePM2List([]byte(res.Stdout))
	if err != nil {
		return nil, err
	}
	return json.Marshal(mapPM2Processes(raw, t.now()))
}

func parsePM2List(data []byte) ([]pm2RawProcess, error) {
	var procs []pm2RawProcess
	if err := json.Unmarshal(data, &procs); err != nil {
		return nil, fmt.Errorf("pm2.list: parse pm2 jlist: %w", err)
	}
	return procs, nil
}

// mapPM2Processes converts raw pm2 processes into the reported result.
// uptime is derived from pm_uptime (start epoch, ms) against now.
func mapPM2Processes(raw []pm2RawProcess, now time.Time) []pm2ListResult {
	results := make([]pm2ListResult, 0, len(raw))
	for _, p := range raw {
		uptime := int64(0)
		if p.PM2Env.PMUptime > 0 {
			uptime = now.UnixMilli() - p.PM2Env.PMUptime
			if uptime < 0 {
				uptime = 0
			}
			uptime /= 1000
		}
		results = append(results, pm2ListResult{
			Name:        p.Name,
			Status:      p.PM2Env.Status,
			PID:         p.Pid,
			CPUPercent:  p.Monit.CPU,
			MemoryBytes: p.Monit.Memory,
			Uptime:      uptime,
		})
	}
	return results
}
