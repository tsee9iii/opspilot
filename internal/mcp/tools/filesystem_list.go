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
	filesystemListName        = "filesystem_list"
	filesystemListDescription = "Inspect the filesystem structure on an agent."
)

const filesystemListInputSchema = `{
  "type": "object",
  "required": ["agent_id", "path"],
  "properties": {
    "agent_id": {"type": "string", "description": "Agent UUID on which to list the directory"},
    "path": {"type": "string", "description": "Absolute path, or a path relative to the agent's first configured project root"},
    "recursive": {"type": "boolean", "description": "Recurse into subdirectories (default false)"},
    "max_depth": {"type": "integer", "description": "Maximum recursion depth (default 1, max 5)"},
    "timeout_seconds": {"type": "integer", "description": "Maximum seconds to wait for completion (default 300, max 600)"}
  }
}`

const filesystemListOutputSchema = `{
  "type": "object",
  "required": ["command_id", "status"],
  "properties": {
    "command_id": {"type": "string"},
    "status": {"type": "string", "enum": ["awaiting_approval", "completed", "failed"]},
    "message": {"type": "string"},
    "listing": {
      "type": "object",
      "properties": {
        "path": {"type": "string"},
        "entries": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "name": {"type": "string"},
              "type": {"type": "string", "enum": ["file", "directory", "symlink"]},
              "size_bytes": {"type": "integer"},
              "modified_at": {"type": "string"}
            }
          }
        }
      }
    },
    "error": {"type": "string"}
  }
}`

// FilesystemListTool dispatches the agent's filesystem.list tool through the
// existing command pipeline. The tool is strictly read-only (confirmation
// level none), so dispatched commands complete or fail without awaiting
// operator approval.
type FilesystemListTool struct {
	dispatch              *dispatch.DispatchUseCase
	defaultTimeoutSeconds int
}

func NewFilesystemListTool(dispatch *dispatch.DispatchUseCase) *FilesystemListTool {
	return &FilesystemListTool{dispatch: dispatch, defaultTimeoutSeconds: 300}
}

// SetDefaultTimeoutSeconds overrides the default timeout used when a call
// omits timeout_seconds.
func (t *FilesystemListTool) SetDefaultTimeoutSeconds(seconds int) {
	if seconds > 0 {
		t.defaultTimeoutSeconds = seconds
	}
}

func (t *FilesystemListTool) Name() string        { return filesystemListName }
func (t *FilesystemListTool) Description() string { return filesystemListDescription }
func (t *FilesystemListTool) Category() string    { return CategoryInvestigation }
func (t *FilesystemListTool) InputSchema() json.RawMessage {
	return json.RawMessage(filesystemListInputSchema)
}
func (t *FilesystemListTool) OutputSchema() json.RawMessage {
	return json.RawMessage(filesystemListOutputSchema)
}

func (t *FilesystemListTool) Call(ctx context.Context, args map[string]any) (json.RawMessage, error) {
	agentID, err := requireString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	path, err := requireString(args, "path")
	if err != nil {
		return nil, err
	}
	recursive, err := optionalBool(args, "recursive")
	if err != nil {
		return nil, err
	}
	maxDepth, err := optionalInt(args, "max_depth", 0)
	if err != nil {
		return nil, err
	}
	timeoutSeconds, err := optionalTimeoutSeconds(args, "timeout_seconds", t.defaultTimeoutSeconds, 600)
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]any{
		"path":      path,
		"recursive": recursive,
		"max_depth": maxDepth,
	})
	resp, err := t.dispatch.Dispatch(ctx, dispatch.DispatchRequest{
		AgentID: agentID,
		Tool:    dispatch.FilesystemListTool,
		Payload: payload,
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("filesystem_list: %w", mapDispatchError(err))
	}
	return buildFilesystemListResult(resp), nil
}

var _ mcp.Tool = (*FilesystemListTool)(nil)

// filesystemListOutput is the stable output of the filesystem_list tool.
type filesystemListOutput struct {
	CommandID string          `json:"command_id"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	Listing   json.RawMessage `json:"listing,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// buildFilesystemListResult shapes a dispatch outcome into the stable tool
// output.
func buildFilesystemListResult(resp dispatch.DispatchResponse) json.RawMessage {
	out := filesystemListOutput{CommandID: resp.CommandID, Status: resp.Status}
	switch resp.Status {
	case "awaiting_approval":
		out.Message = "Awaiting operator approval before the agent lists the directory."
	case "completed":
		out.Listing = resp.Result
	case "failed":
		out.Error = resp.Error
	}
	b, _ := json.Marshal(out)
	return b
}
