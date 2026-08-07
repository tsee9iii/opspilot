package tools

import (
	"encoding/json"
	"fmt"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

// maxLogLines bounds the optional lines argument of the log investigation
// tools. It mirrors the agent-side schema bounds (default 100, maximum 1000)
// so the MCP layer never forwards an unbounded request.
const (
	defaultLogLines = 100
	maxLogLines     = 1000
)

// optionalLines returns a validated log-line count for key, defaulting to
// defaultLogLines and rejecting values outside 1..maxLogLines. It keeps the
// optional lines argument bounded before it ever reaches the agent tool.
func optionalLines(args map[string]any, key string) (int, error) {
	n, err := optionalInt(args, key, defaultLogLines)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > maxLogLines {
		return 0, &mcp.ToolError{
			Code:       "invalid_args",
			Message:    fmt.Sprintf("%s must be between 1 and %d", key, maxLogLines),
			Suggestion: fmt.Sprintf("Provide a %s value between 1 and %d", key, maxLogLines),
		}
	}
	return n, nil
}

// investigationOutput is the stable output envelope of every read-only
// investigation tool. Result carries the agent tool's structured JSON output
// verbatim; it is never flattened or stringified.
type investigationOutput struct {
	CommandID string          `json:"command_id"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// buildInvestigationResult shapes a dispatch outcome into the stable output
// envelope. approvalMsg describes the pending approval state for the specific
// tool.
func buildInvestigationResult(resp dispatch.DispatchResponse, approvalMsg string) json.RawMessage {
	out := investigationOutput{CommandID: resp.CommandID, Status: resp.Status}
	switch resp.Status {
	case "awaiting_approval":
		out.Message = approvalMsg
	case "completed":
		out.Result = resp.Result
	case "failed":
		out.Error = resp.Error
	}
	b, _ := json.Marshal(out)
	return b
}

// investigationTool carries the shared dispatch plumbing of every read-only
// investigation tool: the dispatch use case and the configurable execution
// timeout.
type investigationTool struct {
	dispatch              *dispatch.DispatchUseCase
	defaultTimeoutSeconds int
}

// SetDefaultTimeoutSeconds overrides the default timeout used when a call
// omits timeout_seconds.
func (t *investigationTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeoutSeconds = seconds
	}
}

// timeout validates the optional timeout_seconds argument against the shared
// execution timeout bound.
func (t *investigationTool) timeout(args map[string]any) (int, error) {
	return optionalTimeoutSeconds(args, "timeout_seconds", t.defaultTimeoutSeconds, 600)
}
