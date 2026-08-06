package mcp

import (
	"context"
	"encoding/json"
)

// Tool is a stable capability exposed over MCP. Implementations are thin
// adapters from MCP arguments to an application use case; they never contain
// business logic.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	OutputSchema() json.RawMessage
	// Call returns the stable JSON result of the tool, or an error. Errors are
	// surfaced to the client in machine-readable form.
	Call(ctx context.Context, args map[string]any) (json.RawMessage, error)
}

// ToolSet is an ordered collection of registered tools.
type ToolSet struct {
	byName map[string]Tool
	order  []string
}

func NewToolSet(tools ...Tool) *ToolSet {
	ts := &ToolSet{byName: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		if t == nil || t.Name() == "" {
			continue
		}
		if _, exists := ts.byName[t.Name()]; !exists {
			ts.order = append(ts.order, t.Name())
		}
		ts.byName[t.Name()] = t
	}
	return ts
}

func (ts *ToolSet) Get(name string) (Tool, bool) {
	t, ok := ts.byName[name]
	return t, ok
}

func (ts *ToolSet) Definitions() []toolDefinition {
	defs := make([]toolDefinition, 0, len(ts.order))
	for _, name := range ts.order {
		t := ts.byName[name]
		defs = append(defs, toolDefinition{
			Name:         t.Name(),
			Description:  t.Description(),
			InputSchema:  t.InputSchema(),
			OutputSchema: t.OutputSchema(),
		})
	}
	return defs
}
