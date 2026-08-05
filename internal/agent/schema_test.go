package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

const validationTestSchema = `{
	"type": "object",
	"properties": {
		"process": {"type": "string"},
		"lines": {"type": "integer", "minimum": 1, "maximum": 1000},
		"mode": {"type": "string", "enum": ["tail", "head"]}
	},
	"required": ["process"],
	"additionalProperties": false
}`

type validationTool struct {
	name     string
	schema   string
	executed bool
}

func (t *validationTool) Name() string { return t.name }

func (t *validationTool) Version() string { return "0.0.1" }

func (t *validationTool) Description() string { return "validation test tool" }

func (t *validationTool) ParameterSchema() string { return t.schema }

func (t *validationTool) ConfirmationLevel() agent.ConfirmationLevel { return agent.ConfirmationNone }

func (t *validationTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	t.executed = true
	return []byte(`{"ok":true}`), nil
}

func TestRegistryExecutorValidatesPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{"valid minimal", `{"process":"web"}`, ""},
		{"valid full", `{"process":"web","lines":50,"mode":"tail"}`, ""},
		{"missing required", `{"lines":5}`, `required property "process" missing`},
		{"wrong type", `{"process":"web","lines":"50"}`, `property "lines" must be integer`},
		{"enum mismatch", `{"process":"web","mode":"live"}`, `property "mode" must be one of`},
		{"below minimum", `{"process":"web","lines":0}`, `property "lines" must be >= 1`},
		{"above maximum", `{"process":"web","lines":1001}`, `property "lines" must be <= 1000`},
		{"additional property", `{"process":"web","extra":true}`, `property "extra" is not allowed`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &validationTool{name: "validate", schema: validationTestSchema}
			reg := agent.NewRegistry()
			reg.Register(tool)

			exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{Enabled: true})
			_, err := exec.Execute(context.Background(), "validate", []byte(tt.payload))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !tool.executed {
					t.Fatal("tool was not executed")
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if tool.executed {
				t.Fatal("tool executed despite invalid payload")
			}
		})
	}
}

func TestRegistryExecutorEmptyPayloadValidatesAsEmptyObject(t *testing.T) {
	tool := &validationTool{name: "empty", schema: agent.EmptyParameterSchema}
	reg := agent.NewRegistry()
	reg.Register(tool)

	exec := agent.NewRegistryExecutor(reg, agent.ExecutionPolicy{Enabled: true})
	if _, err := exec.Execute(context.Background(), "empty", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tool.executed {
		t.Fatal("tool was not executed")
	}
}
