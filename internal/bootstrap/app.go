// Package bootstrap wires the central application together.
//
// It owns application lifecycle: configuration, logging, database
// connectivity, HTTP serving and graceful shutdown. It contains no business
// logic, routing or transport handling.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/application/agent"
	appcapability "github.com/tsee9iii/opspilot/internal/application/capability"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/infrastructure/postgres"
	"github.com/tsee9iii/opspilot/internal/infrastructure/security"
	"github.com/tsee9iii/opspilot/internal/migration"
	httpx "github.com/tsee9iii/opspilot/internal/transport/http"
	"github.com/tsee9iii/opspilot/pkg/config"
	"github.com/tsee9iii/opspilot/pkg/logger"
	"github.com/tsee9iii/opspilot/sql/migrations"
)

type App struct {
	cfg  *config.Config
	log  *zap.Logger
	pool *pgxpool.Pool
}

func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: load config: %w", err)
	}

	log, err := logger.New(cfg.Logger.Level, cfg.Env == "production")
	if err != nil {
		return nil, fmt.Errorf("bootstrap: init logger: %w", err)
	}

	pool, err := postgres.New(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: init postgres: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("bootstrap: ping postgres: %w", err)
	}

	log.Info("database connectivity verified")

	migrationRunner := migration.NewRunner(migrations.FS, migration.NewStorage(pool))
	applied, err := migrationRunner.Run(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("bootstrap: run migrations: %w", err)
	}
	if len(applied) > 0 {
		log.Info("database migrations applied", zap.Int("count", len(applied)))
	}

	return &App{cfg: cfg, log: log, pool: pool}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() { _ = a.log.Sync() }()
	defer a.pool.Close()

	handler := a.buildHandler()

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", a.cfg.HTTP.Host, a.cfg.HTTP.Port),
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErr := make(chan error, 1)
	go func() {
		a.log.Info("http server listening", zap.String("addr", server.Addr))
		serverErr <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("bootstrap: http server: %w", err)
		}
		return nil
	case sig := <-sigCh:
		a.log.Info("shutdown signal received", zap.String("signal", sig.String()))
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("bootstrap: graceful shutdown: %w", err)
	}

	a.log.Info("application stopped")
	return nil
}

func (a *App) buildHandler() http.Handler {
	agentRepo := postgres.NewAgentRepository(a.pool)
	tokenRepo := postgres.NewRegistrationTokenRepository(a.pool)
	secretHasher := security.NewArgon2idHasher()
	registerUC := agent.NewRegisterUseCase(
		agentRepo,
		tokenRepo,
		security.NewHMACHasher(a.cfg.Auth.ServerSecret),
		secretHasher,
	)
	heartbeatUC := agent.NewHeartbeatUseCase(agentRepo)
	unregisterUC := agent.NewUnregisterUseCase(agentRepo)
	commandRepo := postgres.NewCommandRepository(a.pool)
	capabilityRepo := postgres.NewCapabilityRepository(a.pool)
	createCommandUC := appcommand.NewCreateUseCase(commandRepo, capabilityRepo)
	leaseCommandUC := appcommand.NewLeaseUseCase(commandRepo, time.Duration(a.cfg.Commands.LeaseTTLSeconds)*time.Second)
	executionCommandUC := appcommand.NewExecutionUseCase(commandRepo)
	approvalCommandUC := appcommand.NewApprovalUseCase(commandRepo)
	getCommandUC := appcommand.NewGetCommandUseCase(commandRepo)
	capabilityUC := appcapability.NewSyncUseCase(agentRepo, capabilityRepo)
	return httpx.NewRouter(
		httpx.RouterDeps{
			Agents:        agentRepo,
			OperatorToken: a.cfg.Auth.OperatorToken,
			Logger:        a.log,
		},
		httpx.NewAgentHandler(registerUC, heartbeatUC, unregisterUC),
		httpx.NewCommandHandler(createCommandUC, leaseCommandUC, executionCommandUC, approvalCommandUC, getCommandUC),
		httpx.NewCapabilityHandler(capabilityUC),
	)
}
