// Package filesystem exposes read-only filesystem inspection tools as
// registered agent tools.
package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
	"github.com/tsee9iii/opspilot/internal/agent/tools/fsutil"
)

const (
	ToolFilesystemList      = "filesystem.list"
	toolListVersion         = "1.0.0"
	toolListDescription     = "Inspect the filesystem structure"
	toolListMaxDepth        = 5
	toolListMaxDirEntries   = 1000
	toolListMaxTotalEntries = 5000
)

const toolListParameterSchema = `{
  "type": "object",
  "required": ["path"],
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute path, or a path relative to the first configured project root"
    },
    "recursive": {
      "type": "boolean",
      "description": "Recurse into subdirectories (default false)"
    },
    "max_depth": {
      "type": "integer",
      "description": "Maximum recursion depth (default 1, max 5)"
    }
  },
  "additionalProperties": false
}`

type listRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	MaxDepth  int    `json:"max_depth"`
}

// listEntry is a single entry in a directory listing. size_bytes and
// modified_at are present only for regular files.
type listEntry struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	SizeBytes  *int64  `json:"size_bytes,omitempty"`
	ModifiedAt *string `json:"modified_at,omitempty"`
}

type listResult struct {
	Path    string      `json:"path"`
	Entries []listEntry `json:"entries"`
}

// FilesystemListTool lists the contents of a directory without ever reading
// file contents. Relative paths resolve against the first configured project
// root; paths that escape it (via ".." or a symlink) are rejected, as are
// special files, directories with more than 1000 entries, and listings larger
// than 5000 entries. Symlinks are reported but never followed.
type FilesystemListTool struct {
	resolver *fsutil.Resolver
}

func NewFilesystemListTool(loader *project.Loader) *FilesystemListTool {
	return &FilesystemListTool{resolver: fsutil.NewResolver(loader)}
}

func (t *FilesystemListTool) Name() string {
	return ToolFilesystemList
}

func (t *FilesystemListTool) Version() string {
	return toolListVersion
}

func (t *FilesystemListTool) Description() string {
	return toolListDescription
}

func (t *FilesystemListTool) ParameterSchema() string {
	return toolListParameterSchema
}

func (t *FilesystemListTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *FilesystemListTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategoryFilesystem,
		Domain:               "configuration",
		Tags:                 []string{"filesystem", "directory", "configuration"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationMedium,
		SinceVersion:         toolListVersion,
	}
}

func (t *FilesystemListTool) Availability(context.Context) (bool, string) {
	return true, ""
}

func (t *FilesystemListTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := parseListRequest(payload)
	if err != nil {
		return nil, err
	}

	target, root, err := t.resolver.Resolve(req.Path)
	if err != nil {
		return nil, err
	}

	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, fsutil.StatError(target, err, "directory_not_found", "directory")
	}
	if root != "" && !fsutil.WithinRoot(root, resolved) {
		return nil, fsutil.EscapeError(req.Path)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fsutil.StatError(resolved, err, "directory_not_found", "directory")
	}
	if !info.IsDir() {
		return nil, &agent.ToolError{
			Code:       "not_a_directory",
			Message:    fmt.Sprintf("path is not a directory: %s", resolved),
			Suggestion: "Point path at a directory to list its contents.",
		}
	}

	maxDepth := req.MaxDepth
	if maxDepth < 1 {
		maxDepth = 1
	}
	if maxDepth > toolListMaxDepth {
		maxDepth = toolListMaxDepth
	}

	entries := make([]listEntry, 0, 32)
	if err := t.walk(ctx, resolved, 1, maxDepth, req.Recursive, &entries); err != nil {
		return nil, err
	}
	sortEntries(entries)

	return json.Marshal(listResult{
		Path:    resolved,
		Entries: entries,
	})
}

// walk appends the entries of dir, recursing into subdirectories while depth
// is below maxDepth. depth is 1 for the requested directory. It enforces the
// per-directory and total entry limits at every level and never follows
// symlinks.
func (t *FilesystemListTool) walk(ctx context.Context, dir string, depth, maxDepth int, recursive bool, entries *[]listEntry) error {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return permissionDeniedError(dir)
		}
		return fmt.Errorf("%s: read directory %s: %w", ToolFilesystemList, dir, err)
	}
	if len(dirEntries) > toolListMaxDirEntries {
		return dirTooLargeError(dir, len(dirEntries))
	}

	for _, de := range dirEntries {
		if len(*entries) >= toolListMaxTotalEntries {
			return totalTooLargeError()
		}
		entry, err := classifyEntry(de)
		if err != nil {
			return err
		}
		*entries = append(*entries, entry)

		if recursive && depth < maxDepth && de.IsDir() {
			if err := t.walk(ctx, filepath.Join(dir, de.Name()), depth+1, maxDepth, recursive, entries); err != nil {
				return err
			}
		}
	}
	return nil
}

// classifyEntry maps a directory entry to its listing shape, rejecting special
// files (sockets, pipes, block and character devices).
func classifyEntry(de os.DirEntry) (listEntry, error) {
	switch {
	case de.Type().IsRegular():
		info, err := de.Info()
		if err != nil {
			return listEntry{}, fmt.Errorf("%s: stat %s: %w", ToolFilesystemList, de.Name(), err)
		}
		size := info.Size()
		mod := info.ModTime().Format(time.RFC3339)
		return listEntry{Name: de.Name(), Type: "file", SizeBytes: &size, ModifiedAt: &mod}, nil
	case de.IsDir():
		return listEntry{Name: de.Name(), Type: "directory"}, nil
	case de.Type()&os.ModeSymlink != 0:
		return listEntry{Name: de.Name(), Type: "symlink"}, nil
	default:
		return listEntry{}, &agent.ToolError{
			Code:       "invalid_path",
			Message:    fmt.Sprintf("entry has an unsupported file type: %s", de.Name()),
			Suggestion: "Sockets, pipes and device files cannot be listed.",
		}
	}
}

// sortEntries orders entries directories first, then files, then symlinks,
// alphabetically within each group.
func sortEntries(entries []listEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		ri, rj := typeRank(entries[i].Type), typeRank(entries[j].Type)
		if ri != rj {
			return ri < rj
		}
		return entries[i].Name < entries[j].Name
	})
}

func typeRank(typ string) int {
	switch typ {
	case "directory":
		return 0
	case "file":
		return 1
	default:
		return 2 // symlink
	}
}

func permissionDeniedError(path string) error {
	return &agent.ToolError{
		Code:       "permission_denied",
		Message:    fmt.Sprintf("permission denied reading directory: %s", path),
		Suggestion: "Grant the agent read access to the directory.",
	}
}

func dirTooLargeError(path string, count int) error {
	return &agent.ToolError{
		Code:       "directory_too_large",
		Message:    fmt.Sprintf("directory has %d entries (limit %d): %s", count, toolListMaxDirEntries, path),
		Suggestion: "List a subdirectory or a non-recursive listing instead.",
	}
}

func totalTooLargeError() error {
	return &agent.ToolError{
		Code:       "directory_too_large",
		Message:    fmt.Sprintf("listing exceeds the %d entry limit", toolListMaxTotalEntries),
		Suggestion: "Reduce max_depth or target a subdirectory.",
	}
}

func parseListRequest(payload []byte) (listRequest, error) {
	if len(payload) == 0 {
		return listRequest{}, errors.New("filesystem.list: payload is required")
	}
	var req listRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, fmt.Errorf("filesystem.list: invalid payload: %w", err)
	}
	if req.Path == "" {
		return req, &agent.ToolError{
			Code:       "invalid_path",
			Message:    "path is required",
			Suggestion: "Provide a directory path to list.",
		}
	}
	return req, nil
}
