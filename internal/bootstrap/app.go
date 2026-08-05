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

	"github.com/opspilot/opspilot/internal/application/agent"
	"github.com/opspilot/opspilot/internal/infrastructure/postgres"
	httpx "github.com/opspilot/opspilot/internal/transport/http"
	"github.com/opspilot/opspilot/pkg/config"
	"github.com/opspilot/opspilot/pkg/logger"
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

	return &App{cfg: cfg, log: log, pool: pool}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() { _ = a.log.Sync() }()
	defer a.pool.Close()

	handler := a.buildHandler()

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.cfg.HTTP.Host, a.cfg.HTTP.Port),
		Handler: handler,
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
	registerUC := agent.NewRegisterUseCase(agentRepo)
	agentHandler := httpx.NewAgentHandler(registerUC)
	return httpx.NewRouter(agentHandler)
}
