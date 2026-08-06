// Package diagnose exposes the host diagnostic workflow as a registered tool,
// so central can dispatch it through the normal command execution path like
// any other command.
package diagnose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/workflow"
)

const (
	ToolDiagnose    = "workflow.diagnose"
	toolVersion     = "1.0.0"
	toolDescription = "Run a host-level diagnostic workflow and return a JSON report"
)

const toolParameterSchema = `{
  "type": "object",
  "properties": {
    "service": {
      "type": "string",
      "description": "Optional systemd service to include a systemctl.status step"
    }
  }
}`

type diagnoseRequest struct {
	Service string `json:"service"`
}

// DiagnoseTool runs the host diagnostic workflow through the injected
// executor — the same RegistryExecutor the agent uses for every other command.
// agentVersion is reported as report metadata so the source build is
// identifiable.
type DiagnoseTool struct {
	executor     agent.Executor
	agentVersion string
}

func NewDiagnoseTool(executor agent.Executor, agentVersion string) *DiagnoseTool {
	return &DiagnoseTool{executor: executor, agentVersion: agentVersion}
}

func (t *DiagnoseTool) Name() string {
	return ToolDiagnose
}

func (t *DiagnoseTool) Version() string {
	return toolVersion
}

func (t *DiagnoseTool) Description() string {
	return toolDescription
}

func (t *DiagnoseTool) ParameterSchema() string {
	return toolParameterSchema
}

func (t *DiagnoseTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *DiagnoseTool) Availability(context.Context) (bool, string) {
	return true, ""
}

// Execute builds and runs the host diagnostic workflow. Individual step
// failures are recorded in the report, not returned as errors: an error is
// only returned for an internal failure that prevented the report from being
// produced.
func (t *DiagnoseTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	var req diagnoseRequest
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("%s: invalid payload: %w", ToolDiagnose, err)
		}
	}

	report := workflow.RunHostDiagnose(ctx, t.executor, req.Service, t.agentVersion)
	out, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal report: %w", ToolDiagnose, err)
	}
	return out, nil
}
