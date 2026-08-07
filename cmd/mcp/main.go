// Command mcp runs the OpsPilot MCP server: a standalone integration layer
// that exposes the platform's application use cases as stable MCP tools over
// stdio. It is an adapter only — it contains no business logic and never calls
// the Central REST API.
//
// # Least-privilege database role
//
// This process connects to PostgreSQL directly. The MCP must run with a
// read-only (or minimally-scoped) database role that can SELECT from the
// platform tables and INSERT into commands for read-only dispatches, but must
// NOT be a superuser or own the schema. Never point the MCP at a role that can
// mutate platform metadata or drop data.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	appdispatch "github.com/tsee9iii/opspilot/internal/application/dispatch"
	appinventory "github.com/tsee9iii/opspilot/internal/application/inventory"
	"github.com/tsee9iii/opspilot/internal/infrastructure/postgres"
	"github.com/tsee9iii/opspilot/internal/mcp"
	"github.com/tsee9iii/opspilot/internal/mcp/tools"
	"github.com/tsee9iii/opspilot/pkg/config"
	"github.com/tsee9iii/opspilot/pkg/logger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("godotenv: no .env file found, relying on environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("mcp: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("mcp: config validation: %v", err)
	}

	zl, err := logger.New(cfg.Logger.Level, cfg.Env == "production")
	if err != nil {
		log.Fatalf("mcp: %v", err)
	}
	defer func() { _ = zl.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.New(ctx, cfg)
	if err != nil {
		log.Fatalf("mcp: %v", err)
	}
	defer pool.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		log.Fatalf("mcp: ping postgres: %v", err)
	}

	commandRepo := postgres.NewCommandRepository(pool)
	capabilityRepo := postgres.NewCapabilityRepository(pool)
	inventoryRepo := postgres.NewInventoryRepository(pool)

	createUC := appcommand.NewCreateUseCase(commandRepo, capabilityRepo)
	getUC := appcommand.NewGetCommandUseCase(commandRepo)
	dispatchUC := appdispatch.NewDispatchUseCase(createUC, getUC)

	toolSet := tools.Build(tools.Dependencies{
		Servers:               appinventory.NewListServersUseCase(inventoryRepo),
		Agents:                appinventory.NewListAgentsUseCase(inventoryRepo),
		Commands:              appinventory.NewListCommandsUseCase(inventoryRepo),
		GetCommand:            getUC,
		Dispatch:              dispatchUC,
		Pinger:                pool,
		DefaultTimeoutSeconds: cfg.MCP.ExecutionTimeoutSeconds,
		// Read-only mode (default) strips the execution tools (workflow_deploy,
		// workflow_diagnose) from the tool set so Hermes cannot dispatch remote
		// execution through the MCP process. It is the safe default.
		ReadOnly: cfg.MCP.ReadOnly,
	})

	server := mcp.NewServer(toolSet, os.Stdin, os.Stdout)
	server.SetPinger(pool)
	zl.Info("mcp server listening on stdio")

	if err := server.Run(ctx); err != nil {
		log.Fatalf("mcp: %v", err)
	}
	zl.Info("mcp server stopped")
}
