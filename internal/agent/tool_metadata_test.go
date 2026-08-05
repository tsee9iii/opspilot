package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/tools/docker"
	"github.com/tsee9iii/opspilot/internal/agent/tools/git"
	httptool "github.com/tsee9iii/opspilot/internal/agent/tools/http"
	"github.com/tsee9iii/opspilot/internal/agent/tools/journal"
	"github.com/tsee9iii/opspilot/internal/agent/tools/pm2"
	"github.com/tsee9iii/opspilot/internal/agent/tools/system"
	"github.com/tsee9iii/opspilot/internal/agent/tools/systemctl"
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
		docker.NewDockerPsTool(),
		docker.NewDockerLogsTool(),
		docker.NewDockerRestartTool(),
		systemctl.NewSystemCtlStatusTool(),
		systemctl.NewSystemCtlRestartTool(),
		journal.NewJournalLogsTool(),
		git.NewGitStatusTool(),
		git.NewGitCurrentCommitTool(),
		git.NewGitBranchTool(),
		git.NewGitPullTool(),
		httptool.NewHTTPCheckTool(),
	}
	for _, tool := range tools {
		if tool.Name() == "" || tool.Version() == "" || tool.Description() == "" {
			t.Fatalf("tool %s missing metadata", tool.Name())
		}
		var schema json.RawMessage
		if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
			t.Fatalf("tool %s has invalid parameter schema: %v", tool.Name(), err)
		}
		if tool.ConfirmationLevel() != agent.ConfirmationNone && tool.ConfirmationLevel() != agent.ConfirmationRequired {
			t.Fatalf("tool %s has invalid confirmation level: %s", tool.Name(), tool.ConfirmationLevel())
		}
	}
}

func TestConfirmationLevels(t *testing.T) {
	readOnly := []agent.Tool{
		system.NewUptimeTool(),
		system.NewMemoryTool(),
		system.NewCPUTool(),
		system.NewDiskTool(),
		system.NewProcessesTool(),
		pm2.NewPM2ListTool(),
		pm2.NewPM2LogsTool(),
		docker.NewDockerPsTool(),
		docker.NewDockerLogsTool(),
		systemctl.NewSystemCtlStatusTool(),
		journal.NewJournalLogsTool(),
		git.NewGitStatusTool(),
		git.NewGitCurrentCommitTool(),
		git.NewGitBranchTool(),
		httptool.NewHTTPCheckTool(),
	}
	for _, tool := range readOnly {
		if tool.ConfirmationLevel() != agent.ConfirmationNone {
			t.Fatalf("tool %s should require no confirmation, got: %s", tool.Name(), tool.ConfirmationLevel())
		}
	}

	writeTools := []agent.Tool{
		pm2.NewPM2RestartTool(),
		docker.NewDockerRestartTool(),
		systemctl.NewSystemCtlRestartTool(),
		git.NewGitPullTool(),
	}
	for _, tool := range writeTools {
		if tool.ConfirmationLevel() != agent.ConfirmationRequired {
			t.Fatalf("tool %s should require confirmation, got: %s", tool.Name(), tool.ConfirmationLevel())
		}
	}
}

func TestToolAvailabilityContract(t *testing.T) {
	tools := []agent.Tool{
		system.NewUptimeTool(),
		system.NewMemoryTool(),
		system.NewCPUTool(),
		system.NewDiskTool(),
		system.NewProcessesTool(),
		pm2.NewPM2ListTool(),
		pm2.NewPM2LogsTool(),
		pm2.NewPM2RestartTool(),
		docker.NewDockerPsTool(),
		docker.NewDockerLogsTool(),
		docker.NewDockerRestartTool(),
		systemctl.NewSystemCtlStatusTool(),
		systemctl.NewSystemCtlRestartTool(),
		journal.NewJournalLogsTool(),
		git.NewGitStatusTool(),
		git.NewGitCurrentCommitTool(),
		git.NewGitBranchTool(),
		git.NewGitPullTool(),
		httptool.NewHTTPCheckTool(),
	}
	for _, tool := range tools {
		ok, reason := tool.Availability(context.Background())
		if ok && reason != "" {
			t.Fatalf("tool %s available but reason set: %q", tool.Name(), reason)
		}
		if !ok && reason == "" {
			t.Fatalf("tool %s unavailable without a reason", tool.Name())
		}
	}
}
