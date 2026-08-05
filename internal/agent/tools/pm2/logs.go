package pm2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolPM2Logs            = "pm2.logs"
	toolPM2LogsVersion     = "1.0.0"
	toolPM2LogsDescription = "Retrieve logs for a PM2 process"

	defaultPM2LogLines = 100
	maxPM2LogLines     = 1000
)

const toolPM2LogsParameterSchema = `{
  "type": "object",
  "required": ["process"],
  "properties": {
    "process": {
      "type": "string",
      "description": "PM2 process name"
    },
    "lines": {
      "type": "integer",
      "minimum": 1,
      "maximum": 1000,
      "default": 100
    }
  }
}`

type pm2LogsResult struct {
	Process string `json:"process"`
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
	Lines   int    `json:"lines"`
}

type pm2LogsRequest struct {
	Process string `json:"process"`
	Lines   *int   `json:"lines"`
}

// PM2LogsTool reports the recent logs of a PM2 process via `pm2 logs`.
type PM2LogsTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewPM2LogsTool() *PM2LogsTool {
	return &PM2LogsTool{run: agent.RunCommand}
}

func (t *PM2LogsTool) Name() string {
	return ToolPM2Logs
}

func (t *PM2LogsTool) Version() string {
	return toolPM2LogsVersion
}

func (t *PM2LogsTool) Description() string {
	return toolPM2LogsDescription
}

func (t *PM2LogsTool) ParameterSchema() string {
	return toolPM2LogsParameterSchema
}

func (t *PM2LogsTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *PM2LogsTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "pm2")
}

func (t *PM2LogsTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	process, lines, err := parsePM2LogsRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := t.ensureProcess(ctx, process); err != nil {
		return nil, err
	}

	stdout, err := t.pm2Logs(ctx, process, lines, "out")
	if err != nil {
		return nil, err
	}
	stderr, err := t.pm2Logs(ctx, process, lines, "err")
	if err != nil {
		return nil, err
	}

	return json.Marshal(pm2LogsResult{
		Process: process,
		Stdout:  stdout,
		Stderr:  stderr,
		Lines:   lines,
	})
}

// parsePM2LogsRequest validates the payload against the tool's parameter
// schema: process is required, lines defaults to 100 and must be 1..1000.
func parsePM2LogsRequest(payload []byte) (string, int, error) {
	if len(payload) == 0 {
		return "", 0, errors.New("pm2.logs: payload is required")
	}
	var req pm2LogsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", 0, fmt.Errorf("pm2.logs: invalid payload: %w", err)
	}
	if req.Process == "" {
		return "", 0, errors.New("pm2.logs: process is required")
	}
	lines := defaultPM2LogLines
	if req.Lines != nil {
		lines = *req.Lines
		if lines < 1 || lines > maxPM2LogLines {
			return "", 0, fmt.Errorf("pm2.logs: lines must be between 1 and %d", maxPM2LogLines)
		}
	}
	return req.Process, lines, nil
}

// ensureProcess verifies the requested process is managed by pm2.
func (t *PM2LogsTool) ensureProcess(ctx context.Context, name string) error {
	out, err := t.run(ctx, "pm2", "jlist")
	if err != nil {
		return wrapPM2Error("pm2 jlist", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("pm2.logs: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("pm2.logs: pm2 jlist failed: %s", res.Stderr)
	}

	procs, err := parsePM2List([]byte(res.Stdout))
	if err != nil {
		return fmt.Errorf("pm2.logs: %w", err)
	}
	for _, p := range procs {
		if p.Name == name {
			return nil
		}
	}
	return fmt.Errorf("pm2.logs: process not found: %s", name)
}

// pm2Logs fetches raw log lines for a process, for the stdout ("out") or
// stderr ("err") stream.
func (t *PM2LogsTool) pm2Logs(ctx context.Context, process string, lines int, stream string) (string, error) {
	args := []string{"logs", process, "--lines", strconv.Itoa(lines), "--nostream", "--raw", "--" + stream}
	out, err := t.run(ctx, "pm2", args...)
	if err != nil {
		return "", wrapPM2Error("pm2 logs", err)
	}

	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("pm2.logs: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("pm2.logs: pm2 logs failed: %s", res.Stderr)
	}
	return res.Stdout, nil
}

func wrapPM2Error(action string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("pm2.logs: pm2 is not installed: %w", err)
	}
	return fmt.Errorf("pm2.logs: %s: %w", action, err)
}
