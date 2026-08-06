package agent

import (
	"context"
	"errors"
	"fmt"
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
// Availability reports whether the tool can actually run on this host
// (available=true with reason="") and, when unavailable, why. Metadata
// returns the semantic catalog entry of the tool.
type Tool interface {
	Name() string
	Version() string
	Description() string
	ParameterSchema() string
	ConfirmationLevel() ConfirmationLevel
	Availability(ctx context.Context) (available bool, reason string)
	Execute(ctx context.Context, payload []byte) ([]byte, error)
	Metadata() ToolMetadata
}

// Registry is a generic, concurrency-safe collection of named tools. It is
// also the single source of truth for the Tool Catalog: ListMetadata returns
// the canonical semantic metadata of every registered tool.
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]Tool
	metadata map[string]ToolMetadata
}

func NewRegistry() *Registry {
	return &Registry{
		tools:    make(map[string]Tool),
		metadata: make(map[string]ToolMetadata),
	}
}

// Register validates the tool's catalog metadata and stores the tool. The
// registry canonicalizes Name, Description, RequiresConfirmation and
// SinceVersion from the tool's own methods so the catalog can never drift from
// the executing tool. Invalid metadata is rejected before it reaches the
// catalog.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return errors.New("register: nil tool")
	}
	md := tool.Metadata()
	md.Name = tool.Name()
	md.Description = tool.Description()
	md.RequiresConfirmation = tool.ConfirmationLevel() == ConfirmationRequired
	if md.SinceVersion == "" {
		md.SinceVersion = tool.Version()
	}
	if err := md.Validate(); err != nil {
		return fmt.Errorf("register %s: %w", md.Name, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[md.Name] = tool
	r.metadata[md.Name] = md
	return nil
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

// ListMetadata returns the semantic metadata of every registered tool, sorted
// by name. It is the canonical source for discovery, filtering and generated
// documentation. Execution and dispatch never read it.
func (r *Registry) ListMetadata() []ToolMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolMetadata, 0, len(r.metadata))
	for _, md := range r.metadata {
		out = append(out, md)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
