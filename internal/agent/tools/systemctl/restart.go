package systemctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolSystemCtlRestart            = "systemctl.restart"
	toolSystemCtlRestartVersion     = "1.0.0"
	toolSystemCtlRestartDescription = "Restart a systemd service"
)

const toolSystemCtlRestartParameterSchema = `{
  "type": "object",
  "required": ["service"],
  "properties": {
    "service": {
      "type": "string",
      "description": "Systemd service name"
    }
  }
}`

type systemctlRestartResult struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type systemctlRestartRequest struct {
	Service string `json:"service"`
}

// SystemCtlRestartTool restarts a systemd service via `systemctl restart`.
type SystemCtlRestartTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewSystemCtlRestartTool() *SystemCtlRestartTool {
	return &SystemCtlRestartTool{run: agent.RunCommand}
}

func (t *SystemCtlRestartTool) Name() string {
	return ToolSystemCtlRestart
}

func (t *SystemCtlRestartTool) Version() string {
	return toolSystemCtlRestartVersion
}

func (t *SystemCtlRestartTool) Description() string {
	return toolSystemCtlRestartDescription
}

func (t *SystemCtlRestartTool) ParameterSchema() string {
	return toolSystemCtlRestartParameterSchema
}

func (t *SystemCtlRestartTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationRequired
}

func (t *SystemCtlRestartTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "systemctl")
}

func (t *SystemCtlRestartTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	service, err := parseSystemCtlRestartRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := ensureSystemCtl(ctx, t.run, "systemctl.restart"); err != nil {
		return nil, err
	}
	if _, err := systemCtlShow(ctx, t.run, "systemctl.restart", service); err != nil {
		return nil, err
	}

	out, err := t.run(ctx, "systemctl", "restart", service)
	if err != nil {
		return nil, fmt.Errorf("systemctl.restart: %w", err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("systemctl.restart: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("systemctl.restart: systemctl restart failed: %s", res.Stderr)
	}

	return json.Marshal(systemctlRestartResult{
		Service: service,
		Status:  "restarted",
	})
}

// parseSystemCtlRestartRequest validates the payload against the tool's
// parameter schema: service is required.
func parseSystemCtlRestartRequest(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("systemctl.restart: payload is required")
	}
	var req systemctlRestartRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("systemctl.restart: invalid payload: %w", err)
	}
	if req.Service == "" {
		return "", errors.New("systemctl.restart: service is required")
	}
	return req.Service, nil
}
