package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolJournalLogs            = "journal.logs"
	toolJournalLogsVersion     = "1.0.0"
	toolJournalLogsDescription = "Retrieve logs for a systemd service from the journal"

	defaultJournalLogLines = 100
	maxJournalLogLines     = 1000
)

const toolJournalLogsParameterSchema = `{
  "type": "object",
  "required": ["service"],
  "properties": {
    "service": {
      "type": "string",
      "description": "Systemd service name"
    },
    "lines": {
      "type": "integer",
      "minimum": 1,
      "maximum": 1000,
      "default": 100
    }
  }
}`

type journalLogsResult struct {
	Service string `json:"service"`
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
	Lines   int    `json:"lines"`
}

type journalLogsRequest struct {
	Service string `json:"service"`
	Lines   *int   `json:"lines"`
}

// JournalLogsTool reports the recent journal entries of a systemd service via
// `journalctl`.
type JournalLogsTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewJournalLogsTool() *JournalLogsTool {
	return &JournalLogsTool{run: agent.RunCommand}
}

func (t *JournalLogsTool) Name() string {
	return ToolJournalLogs
}

func (t *JournalLogsTool) Version() string {
	return toolJournalLogsVersion
}

func (t *JournalLogsTool) Description() string {
	return toolJournalLogsDescription
}

func (t *JournalLogsTool) ParameterSchema() string {
	return toolJournalLogsParameterSchema
}

func (t *JournalLogsTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *JournalLogsTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	service, lines, err := parseJournalLogsRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := t.ensureJournalCtl(ctx); err != nil {
		return nil, err
	}

	out, err := t.run(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	if err != nil {
		return nil, fmt.Errorf("journal.logs: %w", err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("journal.logs: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("journal.logs: journalctl failed: %s", res.Stderr)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return nil, fmt.Errorf("journal.logs: service not found: %s", service)
	}

	return json.Marshal(journalLogsResult{
		Service: service,
		Stdout:  res.Stdout,
		Stderr:  res.Stderr,
		Lines:   lines,
	})
}

// parseJournalLogsRequest validates the payload against the tool's parameter
// schema: service is required, lines defaults to 100 and must be 1..1000.
func parseJournalLogsRequest(payload []byte) (string, int, error) {
	if len(payload) == 0 {
		return "", 0, errors.New("journal.logs: payload is required")
	}
	var req journalLogsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", 0, fmt.Errorf("journal.logs: invalid payload: %w", err)
	}
	if req.Service == "" {
		return "", 0, errors.New("journal.logs: service is required")
	}
	lines := defaultJournalLogLines
	if req.Lines != nil {
		lines = *req.Lines
		if lines < 1 || lines > maxJournalLogLines {
			return "", 0, fmt.Errorf("journal.logs: lines must be between 1 and %d", maxJournalLogLines)
		}
	}
	return req.Service, lines, nil
}

// ensureJournalCtl verifies the journalctl CLI is available on PATH.
func (t *JournalLogsTool) ensureJournalCtl(ctx context.Context) error {
	out, err := t.run(ctx, "journalctl", "--version")
	if err != nil {
		return wrapJournalError(err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("journal.logs: decode command result: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("journal.logs: journalctl --version failed: %s", res.Stderr)
	}
	return nil
}

func wrapJournalError(err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("journal.logs: journalctl is not installed: %w", err)
	}
	return fmt.Errorf("journal.logs: %w", err)
}
