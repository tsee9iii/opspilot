package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
	"github.com/opspilot/opspilot/internal/agent/tools/pm2"
	"github.com/opspilot/opspilot/internal/agent/tools/system"
)

func TestToolMetadata(t *testing.T) {
	tools := []agent.Tool{
		system.NewUptimeTool(),
		system.NewMemoryTool(),
		system.NewCPUTool(),
		system.NewDiskTool(),
		system.NewProcessesTool(),
		pm2.NewPM2ListTool(),
		pm2.NewPM2LogsTool(),
		pm2.NewPM2RestartTool(),
	}
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
