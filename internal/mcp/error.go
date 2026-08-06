// Package mcp implements a Model Context Protocol server that adapts the
// platform's application use cases into stable, machine-reasoning-friendly
// tools. It contains no business logic: every tool is a thin adapter from an
// MCP call to an application use case.
package mcp

import (
	"encoding/json"
	"errors"
)

// ToolError is a machine-readable error returned by a tool. Code is stable and
// resolvable by the client; Message explains the failure; Suggestion points at
// a deterministic remediation when one exists.
type ToolError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

func (e *ToolError) Error() string { return e.Message }

// AsToolError converts any error into a *ToolError. Structured errors are
// preserved; anything else becomes an internal_error so callers always receive
// machine-readable output.
func AsToolError(err error) *ToolError {
	if err == nil {
		return nil
	}
	var te *ToolError
	if errors.As(err, &te) {
		return te
	}
	return &ToolError{
		Code:    "internal_error",
		Message: err.Error(),
	}
}

// marshalToolError serializes a tool error for embedding in tool output.
func marshalToolError(err error) json.RawMessage {
	b, _ := json.Marshal(AsToolError(err))
	return b
}
