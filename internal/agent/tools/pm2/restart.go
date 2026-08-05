package pm2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolPM2Restart            = "pm2.restart"
	toolPM2RestartVersion     = "1.0.0"
	toolPM2RestartDescription = "Restart a PM2 process"
)

const toolPM2RestartParameterSchema = `{"type":"object","required":["process"],"properties":{"process":{"type":"string"}}}`

type pm2RestartResult struct {
	Process string `json:"process"`
	Status  string `json:"status"`
}

// PM2RestartTool restarts a PM2 process via the `pm2 restart` CLI.
type PM2RestartTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewPM2RestartTool() *PM2RestartTool {
	return &PM2RestartTool{run: agent.RunCommand}
}

func (t *PM2RestartTool) Name() string {
	return ToolPM2Restart
}

func (t *PM2RestartTool) Version() string {
	return toolPM2RestartVersion
}

func (t *PM2RestartTool) Description() string {
	return toolPM2RestartDescription
}

func (t *PM2RestartTool) ParameterSchema() string {
	return toolPM2RestartParameterSchema
}

func (t *PM2RestartTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationRequired
}

func (t *PM2RestartTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	process, err := parsePM2RestartRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := t.ensureProcess(ctx, process); err != nil {
		return nil, err
	}
	if err := t.restart(ctx, process); err != nil {
		return nil, err
	}

	return json.Marshal(pm2RestartResult{Process: process, Status: "restarted"})
}

// parsePM2RestartRequest validates the payload against the tool's parameter
// schema: process is required.
func parsePM2RestartRequest(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("pm2.restart: payload is required")
	}
	var req struct {
		Process string `json:"process"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("pm2.restart: invalid payload: %w", err)
	}
	if req.Process == "" {
		return "", errors.New("pm2.restart: process is required")
	}
	return req.Process, nil
}

// ensureProcess verifies the requested process is managed by pm2.
func (t *PM2RestartTool) ensureProcess(ctx context.Context, name string) error {
	out, err := t.run(ctx, "pm2", "jlist")
	if err != nil {
		return wrapPM2RestartError("pm2 jlist", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("pm2.restart: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("pm2.restart: pm2 jlist failed: %s", res.Stderr)
	}

	procs, err := parsePM2List([]byte(res.Stdout))
	if err != nil {
		return fmt.Errorf("pm2.restart: %w", err)
	}
	for _, p := range procs {
		if p.Name == name {
			return nil
		}
	}
	return fmt.Errorf("pm2.restart: process not found: %s", name)
}

// restart runs `pm2 restart <process>` and surfaces non-zero exits.
func (t *PM2RestartTool) restart(ctx context.Context, process string) error {
	out, err := t.run(ctx, "pm2", "restart", process)
	if err != nil {
		return wrapPM2RestartError("pm2 restart", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("pm2.restart: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("pm2.restart: restart failed: %s", res.Stderr)
	}
	return nil
}

func wrapPM2RestartError(action string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("pm2.restart: pm2 is not installed: %w", err)
	}
	return fmt.Errorf("pm2.restart: %s: %w", action, err)
}
