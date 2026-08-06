package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/deploy"
	deploytools "github.com/tsee9iii/opspilot/internal/agent/tools/deploy"
	"github.com/tsee9iii/opspilot/internal/agent/tools/diagnose"
	"github.com/tsee9iii/opspilot/internal/agent/tools/docker"
	filetool "github.com/tsee9iii/opspilot/internal/agent/tools/file"
	"github.com/tsee9iii/opspilot/internal/agent/tools/filesystem"
	"github.com/tsee9iii/opspilot/internal/agent/tools/git"
	httptool "github.com/tsee9iii/opspilot/internal/agent/tools/http"
	"github.com/tsee9iii/opspilot/internal/agent/tools/journal"
	"github.com/tsee9iii/opspilot/internal/agent/tools/pm2"
	"github.com/tsee9iii/opspilot/internal/agent/tools/system"
	"github.com/tsee9iii/opspilot/internal/agent/tools/systemctl"
	"github.com/tsee9iii/opspilot/pkg/config"
	"github.com/tsee9iii/opspilot/pkg/logger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("godotenv: no .env file found, relying on environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	zl, err := logger.New(cfg.Logger.Level, cfg.Env == "production")
	if err != nil {
		log.Fatalf("agent: %v", err)
	}
	defer func() { _ = zl.Sync() }()

	configPath := os.Getenv("OPSPILOT_AGENT_CONFIG")
	if configPath == "" {
		configPath = "agent.yaml"
	}

	agentCfg, err := agent.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	registry := agent.NewRegistry()
	for _, tool := range []agent.Tool{
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
		docker.NewDockerInspectTool(),
		systemctl.NewSystemCtlStatusTool(),
		systemctl.NewSystemCtlRestartTool(),
		journal.NewJournalLogsTool(),
		git.NewGitStatusTool(),
		git.NewGitCurrentCommitTool(),
		git.NewGitBranchTool(),
		git.NewGitPullTool(),
		httptool.NewHTTPCheckTool(),
		filetool.NewFileReadTool(agentCfg.Profiles()),
		filesystem.NewFilesystemListTool(agentCfg.Profiles()),
	} {
		if err := registry.Register(tool); err != nil {
			log.Fatalf("agent: register tool: %v", err)
		}
	}

	exec := agent.NewRegistryExecutor(registry, agentCfg.Policy())
	if err := registry.Register(diagnose.NewDiagnoseTool(exec, agentCfg.Version)); err != nil {
		log.Fatalf("agent: register tool: %v", err)
	}

	strategies := deploy.NewRegistry()
	strategies.Register(deploy.NewDockerComposeStrategy())
	strategies.Register(deploy.NewPM2Strategy())
	strategies.Register(deploy.NewScriptStrategy())

	if err := registry.Register(deploytools.NewDeployProjectTool(agentCfg.Profiles(), strategies)); err != nil {
		log.Fatalf("agent: register tool: %v", err)
	}
	if err := registry.Register(deploytools.NewDeployTool(exec, agentCfg.Profiles(), agentCfg.Version)); err != nil {
		log.Fatalf("agent: register tool: %v", err)
	}

	a := agent.New(agentCfg, zl, exec, registry)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatalf("agent: %v", err)
	}
}
