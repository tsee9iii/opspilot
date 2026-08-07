// Package file exposes read-only file system tools as registered agent tools.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
	"github.com/tsee9iii/opspilot/internal/agent/tools/fsutil"
)

const (
	ToolFileRead        = "file.read"
	toolReadVersion     = "1.0.0"
	toolReadDescription = "Read and understand configuration files"
	toolReadMaxSize     = 1 << 20 // 1 MB
	toolReadEncoding    = "utf-8"
)

const toolReadParameterSchema = `{
  "type": "object",
  "required": ["path"],
  "properties": {
    "path": {
      "type": "string",
      "description": "Path relative to the first configured project root (absolute paths are denied by default)"
    }
  },
  "additionalProperties": false
}`

type fileReadRequest struct {
	Path string `json:"path"`
}

type fileReadResult struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Encoding  string `json:"encoding"`
	Content   string `json:"content"`
}

// FileReadTool reads a single UTF-8 text file and returns its exact contents.
// It is strictly read-only and never executes anything. Relative paths resolve
// against the first configured project root; paths that escape it (via ".."
// or a symlink) are rejected, as are directories, special files, files larger
// than 1 MB, and binary files. Absolute paths are denied by default and only
// honoured when the agent operator enabled allow_absolute_paths.
type FileReadTool struct {
	resolver *fsutil.Resolver
}

// NewFileReadTool builds a tool that denies absolute paths.
func NewFileReadTool(loader *project.Loader) *FileReadTool {
	return &FileReadTool{resolver: fsutil.NewResolver(loader)}
}

// NewFileReadToolWithPolicy builds a file.read tool with an explicit
// absolute-path policy (default deny).
func NewFileReadToolWithPolicy(loader *project.Loader, allowAbsolutePaths bool) *FileReadTool {
	return &FileReadTool{resolver: fsutil.NewResolverWithPolicy(loader, allowAbsolutePaths)}
}

func (t *FileReadTool) Name() string {
	return ToolFileRead
}

func (t *FileReadTool) Version() string {
	return toolReadVersion
}

func (t *FileReadTool) Description() string {
	return toolReadDescription
}

func (t *FileReadTool) ParameterSchema() string {
	return toolReadParameterSchema
}

func (t *FileReadTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *FileReadTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategoryFilesystem,
		Domain:               "configuration",
		Tags:                 []string{"file", "configuration", "filesystem"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationShort,
		SinceVersion:         toolReadVersion,
	}
}

func (t *FileReadTool) Availability(context.Context) (bool, string) {
	return true, ""
}

func (t *FileReadTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := parseReadRequest(payload)
	if err != nil {
		return nil, err
	}

	target, root, err := t.resolver.Resolve(req.Path)
	if err != nil {
		return nil, err
	}

	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, fsutil.StatError(target, err, "file_not_found", "file")
	}
	if root != "" && !fsutil.WithinRoot(root, resolved) {
		return nil, fsutil.EscapeError(req.Path)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fsutil.StatError(resolved, err, "file_not_found", "file")
	}
	switch {
	case info.IsDir():
		return nil, &agent.ToolError{
			Code:       "directory_not_allowed",
			Message:    fmt.Sprintf("path is a directory: %s", resolved),
			Suggestion: "Point path at a regular file, not a directory.",
		}
	case !info.Mode().IsRegular():
		return nil, &agent.ToolError{
			Code:       "invalid_path",
			Message:    fmt.Sprintf("path is not a regular file: %s", resolved),
			Suggestion: "Only regular files can be read.",
		}
	case info.Size() > toolReadMaxSize:
		return nil, &agent.ToolError{
			Code:       "file_too_large",
			Message:    fmt.Sprintf("file exceeds the %d byte read limit: %s", toolReadMaxSize, resolved),
			Suggestion: "Use a targeted read on a smaller file, or inspect the file on the agent directly.",
		}
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil, &agent.ToolError{
				Code:       "permission_denied",
				Message:    fmt.Sprintf("permission denied reading file: %s", resolved),
				Suggestion: "Grant the agent read access to the file, or use a path the agent can read.",
			}
		}
		return nil, fmt.Errorf("%s: read %s: %w", ToolFileRead, resolved, err)
	}
	if !utf8.Valid(data) {
		return nil, &agent.ToolError{
			Code:       "binary_file",
			Message:    fmt.Sprintf("file is not valid UTF-8 text: %s", resolved),
			Suggestion: "file.read only supports UTF-8 text files.",
		}
	}

	return json.Marshal(fileReadResult{
		Path:      resolved,
		SizeBytes: info.Size(),
		Encoding:  toolReadEncoding,
		Content:   string(data),
	})
}

func parseReadRequest(payload []byte) (fileReadRequest, error) {
	if len(payload) == 0 {
		return fileReadRequest{}, errors.New("file.read: payload is required")
	}
	var req fileReadRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, fmt.Errorf("file.read: invalid payload: %w", err)
	}
	if req.Path == "" {
		return req, &agent.ToolError{
			Code:       "invalid_path",
			Message:    "path is required",
			Suggestion: "Provide a file path to read.",
		}
	}
	return req, nil
}
