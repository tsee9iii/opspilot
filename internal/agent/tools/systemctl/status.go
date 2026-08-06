package systemctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolSystemCtlStatus            = "systemctl.status"
	toolSystemCtlStatusVersion     = "1.0.0"
	toolSystemCtlStatusDescription = "Report the status of a systemd service"
)

const toolSystemCtlStatusParameterSchema = `{
  "type": "object",
  "required": ["service"],
  "properties": {
    "service": {
      "type": "string",
      "description": "Systemd service name (example: nginx.service)"
    }
  }
}`

type systemctlStatusResult struct {
	Service       string `json:"service"`
	Description   string `json:"description"`
	LoadState     string `json:"load_state"`
	ActiveState   string `json:"active_state"`
	SubState      string `json:"sub_state"`
	UnitFileState string `json:"unit_file_state"`
	MainPID       int    `json:"main_pid"`
	ExitStatus    int    `json:"exit_status"`
}

type systemctlStatusRequest struct {
	Service string `json:"service"`
}

// SystemCtlStatusTool reports the status of a systemd service via the
// key=value output of `systemctl show`.
type SystemCtlStatusTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewSystemCtlStatusTool() *SystemCtlStatusTool {
	return &SystemCtlStatusTool{run: agent.RunCommand}
}

func (t *SystemCtlStatusTool) Name() string {
	return ToolSystemCtlStatus
}

func (t *SystemCtlStatusTool) Version() string {
	return toolSystemCtlStatusVersion
}

func (t *SystemCtlStatusTool) Description() string {
	return toolSystemCtlStatusDescription
}

func (t *SystemCtlStatusTool) ParameterSchema() string {
	return toolSystemCtlStatusParameterSchema
}

func (t *SystemCtlStatusTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *SystemCtlStatusTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategorySystemd,
		Domain:               "linux",
		Tags:                 []string{"systemd", "service", "status", "health"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationShort,
		SinceVersion:         toolSystemCtlStatusVersion,
	}
}

func (t *SystemCtlStatusTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "systemctl")
}

func (t *SystemCtlStatusTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	service, err := parseSystemCtlStatusRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := ensureSystemCtl(ctx, t.run, "systemctl.status"); err != nil {
		return nil, err
	}

	result, err := systemCtlShow(ctx, t.run, "systemctl.status", service)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// parseSystemCtlStatusRequest validates the payload against the tool's
// parameter schema: service is required.
func parseSystemCtlStatusRequest(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("systemctl.status: payload is required")
	}
	var req systemctlStatusRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("systemctl.status: invalid payload: %w", err)
	}
	if req.Service == "" {
		return "", errors.New("systemctl.status: service is required")
	}
	return req.Service, nil
}

// ensureSystemCtl verifies the systemctl CLI is available on PATH.
func ensureSystemCtl(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool string) error {
	out, err := run(ctx, "systemctl", "--version")
	if err != nil {
		return wrapSystemCtlError(tool, err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("%s: decode command result: %w", tool, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: systemctl --version failed: %s", tool, res.Stderr)
	}
	return nil
}

// systemCtlShow runs `systemctl show <service>` with the status tool's
// properties and parses the key=value output, reusing parseSystemCtlStatus.
func systemCtlShow(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool, service string) (systemctlStatusResult, error) {
	out, err := run(ctx, "systemctl", "show", service,
		"--property=Id", "--property=Description", "--property=LoadState",
		"--property=ActiveState", "--property=SubState", "--property=UnitFileState",
		"--property=MainPID", "--property=ExecMainStatus", "--no-pager")
	if err != nil {
		return systemctlStatusResult{}, fmt.Errorf("%s: %w", tool, err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return systemctlStatusResult{}, fmt.Errorf("%s: decode command result: %w", tool, err)
	}
	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "not found") || strings.Contains(res.Stderr, "could not be found") {
			return systemctlStatusResult{}, fmt.Errorf("%s: service not found: %s", tool, service)
		}
		return systemctlStatusResult{}, fmt.Errorf("%s: systemctl show failed: %s", tool, res.Stderr)
	}
	result, err := parseSystemCtlStatus(res.Stdout)
	if err != nil {
		return systemctlStatusResult{}, fmt.Errorf("%s: %w", tool, err)
	}
	return result, nil
}

// parseSystemCtlStatus parses the key=value lines emitted by
// `systemctl show`.
func parseSystemCtlStatus(stdout string) (systemctlStatusResult, error) {
	result := systemctlStatusResult{}
	seenID := false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return result, fmt.Errorf("invalid property line: %q", line)
		}
		switch key {
		case "Id":
			result.Service = value
			seenID = true
		case "Description":
			result.Description = value
		case "LoadState":
			result.LoadState = value
		case "ActiveState":
			result.ActiveState = value
		case "SubState":
			result.SubState = value
		case "UnitFileState":
			result.UnitFileState = value
		case "MainPID":
			pid, err := strconv.Atoi(value)
			if err != nil {
				return result, fmt.Errorf("invalid main_pid: %q", value)
			}
			result.MainPID = pid
		case "ExecMainStatus":
			status, err := strconv.Atoi(value)
			if err != nil {
				return result, fmt.Errorf("invalid exit_status: %q", value)
			}
			result.ExitStatus = status
		}
	}
	if !seenID {
		return result, errors.New("output missing Id property")
	}
	return result, nil
}

func wrapSystemCtlError(tool string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s: systemctl is not installed: %w", tool, err)
	}
	return fmt.Errorf("%s: %w", tool, err)
}
