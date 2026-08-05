package agent

import (
	"context"
	"sort"
	"sync"
)

// ConfirmationLevel describes whether executing a tool requires explicit
// operator confirmation.
type ConfirmationLevel string

const (
	// ConfirmationNone marks read-only tools that require no confirmation.
	ConfirmationNone ConfirmationLevel = "none"
	// ConfirmationRequired marks write tools that must be confirmed before
	// they run.
	ConfirmationRequired ConfirmationLevel = "required"
)

// Tool executes a named operation against a payload and returns a result.
// ParameterSchema returns the tool's accepted payload as a JSON Schema
// document; ConfirmationLevel is the tool's confirmation metadata.
type Tool interface {
	Name() string
	Version() string
	Description() string
	ParameterSchema() string
	ConfirmationLevel() ConfirmationLevel
	Execute(ctx context.Context, payload []byte) ([]byte, error)
}

// Registry is a generic, concurrency-safe collection of named tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

func (r *Registry) Find(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
