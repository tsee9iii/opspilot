package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/opspilot/opspilot/internal/agent"
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
	registry.Register(agent.NewUptimeTool())

	a := agent.New(agentCfg, zl, agent.NewRegistryExecutor(registry, agentCfg.Policy()))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatalf("agent: %v", err)
	}
}
