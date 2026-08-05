package agent

import (
	"encoding/json"
	"testing"
)

func TestToolMetadata(t *testing.T) {
	tools := []Tool{NewUptimeTool(), NewMemoryTool(), NewCPUTool(), NewDiskTool(), NewProcessesTool()}
	for _, tool := range tools {
		if tool.Name() == "" || tool.Version() == "" || tool.Description() == "" {
			t.Fatalf("tool %s missing metadata", tool.Name())
		}
		var schema json.RawMessage
		if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
			t.Fatalf("tool %s has invalid parameter schema: %v", tool.Name(), err)
		}
	}
}
