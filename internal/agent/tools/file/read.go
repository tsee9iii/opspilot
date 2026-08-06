// Package file exposes read-only file system tools as registered agent tools.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
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
      "description": "Absolute path, or a path relative to the first configured project root"
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
// than 1 MB, and binary files.
type FileReadTool struct {
	loader *project.Loader
}

func NewFileReadTool(loader *project.Loader) *FileReadTool {
	return &FileReadTool{loader: loader}
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

func (t *FileReadTool) Availability(context.Context) (bool, string) {
	return true, ""
}

func (t *FileReadTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := parseReadRequest(payload)
	if err != nil {
		return nil, err
	}

	target, root, err := t.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, statError(target, err)
	}
	if root != "" && !withinRoot(root, resolved) {
		return nil, pathEscapeError(req.Path)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, statError(resolved, err)
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

// resolvePath maps the requested path to an absolute target. Absolute paths
// are used directly; relative paths resolve against the first configured
// project root, and any path escaping that root is rejected.
func (t *FileReadTool) resolvePath(p string) (target, root string, err error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), "", nil
	}
	root, err = t.canonicalRoot()
	if err != nil {
		return "", "", err
	}
	target = filepath.Clean(filepath.Join(root, p))
	if !withinRoot(root, target) {
		return "", "", pathEscapeError(p)
	}
	return target, root, nil
}

// canonicalRoot returns the first configured project root with its symlinks
// resolved, so containment checks compare against the same canonical base as
// the resolved read target.
func (t *FileReadTool) canonicalRoot() (string, error) {
	root := t.projectRoot()
	if root == "" {
		return "", &agent.ToolError{
			Code:       "invalid_path",
			Message:    "relative paths require a configured project root",
			Suggestion: "Add a project with an absolute path to the agent config, or pass an absolute path.",
		}
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", statError(root, err)
	}
	return resolved, nil
}

func (t *FileReadTool) projectRoot() string {
	if t.loader == nil {
		return ""
	}
	projects := t.loader.Projects()
	if len(projects) == 0 {
		return ""
	}
	return projects[0].Repository
}

// withinRoot reports whether target is base itself or a descendant of base.
func withinRoot(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func pathEscapeError(p string) error {
	return &agent.ToolError{
		Code:       "invalid_path",
		Message:    fmt.Sprintf("path escapes the project root: %s", p),
		Suggestion: "Provide a path inside the project root, or an absolute path.",
	}
}

// statError maps file system lookup failures to structured tool errors.
func statError(path string, err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return &agent.ToolError{
			Code:       "file_not_found",
			Message:    fmt.Sprintf("file does not exist: %s", path),
			Suggestion: "Check that the path exists and is spelled correctly.",
		}
	case errors.Is(err, os.ErrPermission):
		return &agent.ToolError{
			Code:       "permission_denied",
			Message:    fmt.Sprintf("permission denied accessing file: %s", path),
			Suggestion: "Grant the agent read access to the file, or use a path the agent can read.",
		}
	default:
		return fmt.Errorf("%s: stat %s: %w", ToolFileRead, path, err)
	}
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
