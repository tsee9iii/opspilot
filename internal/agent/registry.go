package agent

import (
	"context"
	"sort"
	"sync"
)

// Tool executes a named operation against a payload and returns a result.
// ParameterSchema returns the tool's accepted payload as a JSON Schema
// document.
type Tool interface {
	Name() string
	Version() string
	Description() string
	ParameterSchema() string
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
