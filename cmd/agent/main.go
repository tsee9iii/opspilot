package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/opspilot/opspilot/internal/agent"
	"github.com/opspilot/opspilot/internal/agent/tools/docker"
	"github.com/opspilot/opspilot/internal/agent/tools/git"
	"github.com/opspilot/opspilot/internal/agent/tools/journal"
	"github.com/opspilot/opspilot/internal/agent/tools/pm2"
	"github.com/opspilot/opspilot/internal/agent/tools/system"
	"github.com/opspilot/opspilot/internal/agent/tools/systemctl"
	"github.com/opspilot/opspilot/pkg/config"
	"github.com/opspilot/opspilot/pkg/logger"
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
	registry.Register(system.NewUptimeTool())
	registry.Register(system.NewMemoryTool())
	registry.Register(system.NewCPUTool())
	registry.Register(system.NewDiskTool())
	registry.Register(system.NewProcessesTool())
	registry.Register(pm2.NewPM2ListTool())
	registry.Register(pm2.NewPM2LogsTool())
	registry.Register(pm2.NewPM2RestartTool())
	registry.Register(docker.NewDockerPsTool())
	registry.Register(docker.NewDockerLogsTool())
	registry.Register(docker.NewDockerRestartTool())
	registry.Register(systemctl.NewSystemCtlStatusTool())
	registry.Register(systemctl.NewSystemCtlRestartTool())
	registry.Register(journal.NewJournalLogsTool())
	registry.Register(git.NewGitStatusTool())

	a := agent.New(agentCfg, zl, agent.NewRegistryExecutor(registry, agentCfg.Policy()), registry)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatalf("agent: %v", err)
	}
}
