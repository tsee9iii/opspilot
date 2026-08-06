package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

const (
	fileReadName        = "file_read"
	fileReadDescription = "Read a configuration file on an agent and return its exact contents."
)

const fileReadInputSchema = `{
  "type": "object",
  "required": ["agent_id", "path"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to read the file"},
    "path": {"type": "string", "description": "Absolute path, or a path relative to the agent's first configured project root"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const fileReadOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "file": {
      "type": "object",
      "properties": {
        "path": {"type": "string"},
        "size_bytes": {"type": "integer"},
        "encoding": {"type": "string"},
        "content": {"type": "string"}
      }
    },
    "error": {"type": "string"}
  }
}`

// FileReadTool dispatches the agent's file.read tool through the existing
// command pipeline. The tool is strictly read-only (confirmation level none),
// so dispatched commands complete or fail without awaiting operator approval.
type FileReadTool struct {
	dispatch              *dispatch.DispatchUseCase
	defaultTimeoutSeconds int
}

func NewFileReadTool(dispatch *dispatch.DispatchUseCase) *FileReadTool {
	return &FileReadTool{dispatch: dispatch, defaultTimeoutSeconds: 300}
}

// SetDefaultTimeoutSeconds overrides the default timeout used when a call
// omits timeout_seconds.
func (t *FileReadTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeoutSeconds = seconds
	}
}

func (t *FileReadTool) Name() string        { return fileReadName }
func (t *FileReadTool) Description() string { return fileReadDescription }
func (t *FileReadTool) Category() string    { return CategoryInvestigation }
func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(fileReadInputSchema)
}
func (t *FileReadTool) OutputSchema() json.RawMessage {
	return json.RawMessage(fileReadOutputSchema)
}

func (t *FileReadTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	path, err := requireString(args, "path")
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := optionalTimeoutSeconds(args, "timeout_seconds", t.defaultTimeoutSeconds, 600)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]string{"path": path})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.FileReadTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("file_read: %w", mapDispatchError(err))
	}
	return buildFileReadResult(resp), nil
}

var _ mcp.Tool = (*FileReadTool)(nil)

// fileReadOutput is the stable output of the file_read tool.
type fileReadOutput struct {
	CommandID string          `json:"command_id"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	File      json.RawMessage `json:"file,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// buildFileReadResult shapes a dispatch outcome into the stable tool output.
func buildFileReadResult(resp dispatch.DispatchResponse) json.RawMessage {
	out := fileReadOutput{CommandID: resp.CommandID, Status: resp.Status}
	switch resp.Status {
	case "awaiting_approval":
		out.Message = "Awaiting operator approval before the agent reads the file."
	case "completed":
		out.File = resp.Result
	case "failed":
		out.Error = resp.Error
	}
	b, _ := json.Marshal(out)
	return b
}
