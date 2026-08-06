// Package tools adapts the platform's application use cases into stable MCP
// tools. Tools only validate arguments, call a use case, and shape the result;
// they contain no business logic.
package tools

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/tsee9iii/opspilot/internal/mcp"
)

// requireString returns the non-empty string argument for key.
func requireString(args map[string]any, key string) (string, error) {
	s, err := optionalString(args, key)
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", &mcp.ToolError{
			Code:       "invalid_args",
			Message:    fmt.Sprintf("%s is required", key),
			Suggestion: "Provide a value for " + key,
		}
	}
	return s, nil
}

// optionalString returns the string argument for key, if present.
func optionalString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", &mcp.ToolError{
			Code:       "invalid_args",
			Message:    fmt.Sprintf("%s must be a string", key),
			Suggestion: "Provide a string value for " + key,
		}
	}
	return s, nil
}

// optionalInt returns the integer argument for key, or def when absent.
// MCP arguments are decoded from JSON, so numbers arrive as float64; plain Go
// integers are accepted too. Fractional values are rejected.
func optionalInt(args map[string]any, key string, def int) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return def, nil
	}
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			break
		}
		return int(n), nil
	case int:
		return n, nil
	case int64:
		if n > int64(maxInt) {
			break
		}
		return int(n), nil
	}
	return 0, &mcp.ToolError{
		Code:       "invalid_args",
		Message:    fmt.Sprintf("%s must be an integer", key),
		Suggestion: "Provide an integer value for " + key,
	}
}

const maxInt = int(^uint(0) >> 1)

// requireUUID returns the UUID argument for key.
func requireUUID(args map[string]any, key string) (uuid.UUID, error) {
	s, err := requireString(args, key)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, &mcp.ToolError{
			Code:       "invalid_args",
			Message:    fmt.Sprintf("%s must be a valid UUID", key),
			Suggestion: "Provide a UUID value for " + key,
		}
	}
	return id, nil
}

// optionalUUID returns the UUID argument for key, or nil when absent.
func optionalUUID(args map[string]any, key string) (*uuid.UUID, error) {
	s, err := optionalString(args, key)
	if err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, &mcp.ToolError{
			Code:       "invalid_args",
			Message:    fmt.Sprintf("%s must be a valid UUID", key),
			Suggestion: "Provide a UUID value for " + key,
		}
	}
	return &id, nil
}

// optionalTimeoutSeconds returns a validated timeout in seconds for key,
// bounded to the given maximum, or def when absent.
func optionalTimeoutSeconds(args map[string]any, key string, def, max int) (int, error) {
	n, err := optionalInt(args, key, def)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > max {
		return 0, &mcp.ToolError{
			Code:       "invalid_args",
			Message:    fmt.Sprintf("%s must be between 1 and %d seconds", key, max),
			Suggestion: fmt.Sprintf("Provide a %s value between 1 and %d", key, max),
		}
	}
	return n, nil
}
