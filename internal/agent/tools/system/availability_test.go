package system

import (
	"context"
	"runtime"
	"testing"

	"github.com/opspilot/opspilot/internal/agent"
)

func TestSystemToolsAvailability(t *testing.T) {
	tools := []agent.Tool{
		NewUptimeTool(),
		NewMemoryTool(),
		NewCPUTool(),
		NewDiskTool(),
		NewProcessesTool(),
	}
	for _, tool := range tools {
		ok, reason := tool.Availability(context.Background())
		if runtime.GOOS == "linux" {
			if !ok || reason != "" {
				t.Fatalf("%s: expected available on linux, got ok=%v reason=%q", tool.Name(), ok, reason)
			}
		} else {
			if ok || reason != "unsupported platform" {
				t.Fatalf("%s: expected unsupported platform, got ok=%v reason=%q", tool.Name(), ok, reason)
			}
		}
	}
}
