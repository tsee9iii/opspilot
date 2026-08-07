// Package fsutil provides the project-aware filesystem path resolution and
// error mapping shared by the read-only filesystem agent tools (file.read,
// filesystem.list).
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// Resolver maps tool input paths to absolute targets. By default absolute
// paths are rejected (fail closed): tool callers are restricted to paths under
// the configured project roots, resolved against the first project root, whose
// symlinks are canonicalized so containment is compared resolved to resolved.
// Escaping the root (via ".." or a symlink) is rejected. Absolute paths are
// only honoured when AllowAbsolutePaths was explicitly enabled by the agent
// operator.
type Resolver struct {
	loader             *project.Loader
	allowAbsolutePaths bool
}

// NewResolver builds a Resolver that rejects absolute paths.
func NewResolver(loader *project.Loader) *Resolver {
	return &Resolver{loader: loader}
}

// NewResolverWithPolicy builds a Resolver with an explicit absolute-path
// policy. allowAbsolutePaths defaults to deny; enable only when the agent
// operator trusts every tool caller (e.g. the Hermes integration is not
// reachable by untrusted parties).
func NewResolverWithPolicy(loader *project.Loader, allowAbsolutePaths bool) *Resolver {
	return &Resolver{loader: loader, allowAbsolutePaths: allowAbsolutePaths}
}

// Resolve returns the absolute target for p and the canonical project root
// (empty when p is absolute and absolute paths are enabled). It returns a
// structured ToolError when p is absolute (and not enabled), when a relative
// path has no project root to resolve against, or when it escapes the root.
func (r *Resolver) Resolve(p string) (target, root string, err error) {
	if filepath.IsAbs(p) {
		if !r.allowAbsolutePaths {
			return "", "", AbsolutePathDeniedError(p)
		}
		return filepath.Clean(p), "", nil
	}
	root, err = r.canonicalRoot()
	if err != nil {
		return "", "", err
	}
	target = filepath.Clean(filepath.Join(root, p))
	if !WithinRoot(root, target) {
		return "", "", EscapeError(p)
	}
	return target, root, nil
}

// ProjectRoot returns the first configured project root, or "" when the agent
// has no configured projects.
func (r *Resolver) ProjectRoot() string {
	if r.loader == nil {
		return ""
	}
	projects := r.loader.Projects()
	if len(projects) == 0 {
		return ""
	}
	return projects[0].Repository
}

// canonicalRoot returns the first configured project root with its symlinks
// resolved, so containment checks compare against the same canonical base as
// the resolved read target.
func (r *Resolver) canonicalRoot() (string, error) {
	root := r.ProjectRoot()
	if root == "" {
		return "", NoProjectRootError()
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", StatError(root, err, "", "")
	}
	return resolved, nil
}

// WithinRoot reports whether target is base itself or a descendant of base.
func WithinRoot(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// NoProjectRootError is returned when a relative path is used but the agent
// has no configured project root to resolve it against.
func NoProjectRootError() error {
	return &agent.ToolError{
		Code:       "invalid_path",
		Message:    "relative paths require a configured project root",
		Suggestion: "Add a project with an absolute path to the agent config.",
	}
}

// AbsolutePathDeniedError is returned when a caller passes an absolute path
// but absolute paths are not enabled in the agent configuration. This is the
// default and intended fail-closed behaviour for Hermes-facing file tools.
func AbsolutePathDeniedError(p string) error {
	return &agent.ToolError{
		Code:       "invalid_path",
		Message:    fmt.Sprintf("absolute path denied: %s", p),
		Suggestion: "Use a path relative to a configured project root, or enable allow_absolute_paths only if you trust every tool caller.",
	}
}

// EscapeError is returned when a path resolves outside the project root.
func EscapeError(p string) error {
	return &agent.ToolError{
		Code:       "invalid_path",
		Message:    fmt.Sprintf("path escapes the project root: %s", p),
		Suggestion: "Provide a path inside the project root, or an absolute path.",
	}
}

// StatError maps a file system lookup failure to a structured ToolError.
// notFoundCode and notFoundNoun tailor the missing-path error ("file" for
// file.read, "directory" for filesystem.list); an empty notFoundCode falls
// back to a generic missing-path message. Permission failures always map to
// "permission_denied".
func StatError(path string, err error, notFoundCode, notFoundNoun string) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		code := notFoundCode
		if code == "" {
			code = "invalid_path"
		}
		noun := notFoundNoun
		if noun == "" {
			noun = "path"
		}
		return &agent.ToolError{
			Code:       code,
			Message:    fmt.Sprintf("%s does not exist: %s", noun, path),
			Suggestion: "Check that the path exists and is spelled correctly.",
		}
	case errors.Is(err, os.ErrPermission):
		return &agent.ToolError{
			Code:       "permission_denied",
			Message:    fmt.Sprintf("permission denied accessing path: %s", path),
			Suggestion: "Grant the agent read access to the path.",
		}
	default:
		return fmt.Errorf("stat %s: %w", path, err)
	}
}
