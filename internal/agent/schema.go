package agent

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// validatePayload enforces a tool's JSON Schema against a command payload.
// An empty payload is treated as an empty object so that parameterless tools
// and tests that omit a payload validate cleanly.
func validatePayload(schema, payload []byte) error {
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schema),
		gojsonschema.NewBytesLoader(payload),
	)
	if err != nil {
		return fmt.Errorf("validate payload: %w", err)
	}
	if result.Valid() {
		return nil
	}

	msgs := make([]string, 0, len(result.Errors()))
	for _, e := range result.Errors() {
		msgs = append(msgs, schemaValidationMessage(e))
	}
	sort.Strings(msgs)
	return errors.New(strings.Join(msgs, "; "))
}

// schemaValidationMessage renders a single schema violation as a stable,
// human-readable message.
func schemaValidationMessage(e gojsonschema.ResultError) string {
	field := e.Field()
	if field == "(root)" {
		field = "payload"
	}

	switch e.Type() {
	case "required":
		return fmt.Sprintf("required property %q missing", e.Details()["property"])
	case "invalid_type":
		return fmt.Sprintf("property %q must be %v", field, e.Details()["expected"])
	case "enum":
		return fmt.Sprintf("property %q must be one of: %v", field, e.Details()["allowed"])
	case "number_gte":
		return fmt.Sprintf("property %q must be >= %v", field, e.Details()["min"])
	case "number_lte":
		return fmt.Sprintf("property %q must be <= %v", field, e.Details()["max"])
	case "additional_property_not_allowed":
		return fmt.Sprintf("property %q is not allowed", e.Details()["property"])
	default:
		return fmt.Sprintf("property %q: %s", field, e.Description())
	}
}
