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
	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
	appcapability "github.com/tsee9iii/opspilot/internal/application/capability"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	apphealth "github.com/tsee9iii/opspilot/internal/application/health"
	"github.com/tsee9iii/opspilot/internal/infrastructure/postgres"
	"github.com/tsee9iii/opspilot/internal/infrastructure/security"
	"github.com/tsee9iii/opspilot/internal/infrastructure/webhook"
	"github.com/tsee9iii/opspilot/internal/migration"
	"github.com/tsee9iii/opspilot/internal/notify"
	httpx "github.com/tsee9iii/opspilot/internal/transport/http"
	"github.com/tsee9iii/opspilot/pkg/config"
	"github.com/tsee9iii/opspilot/pkg/logger"
	"github.com/tsee9iii/opspilot/sql/migrations"
)

type App struct {
	cfg  *config.Config
	log  *zap.Logger
	pool *pgxpool.Pool

	// evaluator runs the in-process alert rules. It is nil when alerting is
	// disabled.
	evaluator *appalert.Evaluator
	// notifier is the in-memory agent wake-up channel. It is closed during
	// shutdown so active SSE streams end cleanly.
	notifier *notify.Notifier
}

func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("bootstrap: config validation: %w", err)
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

	return &App{cfg: cfg, log: log, pool: pool, notifier: notify.New()}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() { _ = a.log.Sync() }()
	defer a.pool.Close()

	handler := a.buildHandler()
	if a.evaluator != nil {
		a.evaluator.Run(ctx)
		defer a.evaluator.Wait()
	}

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

	// Cancel all SSE streams first so their handlers return and the connections
	// become idle; then the graceful drain below closes them promptly instead of
	// waiting for the Shutdown timeout.
	a.notifier.Close()

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
	createCommandUC := appcommand.NewCreateUseCase(commandRepo, capabilityRepo, a.notifier)
	leaseCommandUC := appcommand.NewLeaseUseCase(commandRepo, time.Duration(a.cfg.Commands.LeaseTTLSeconds)*time.Second)
	executionCommandUC := appcommand.NewExecutionUseCase(commandRepo)
	approvalCommandUC := appcommand.NewApprovalUseCase(commandRepo, a.notifier)
	getCommandUC := appcommand.NewGetCommandUseCase(commandRepo)
	capabilityUC := appcapability.NewSyncUseCase(agentRepo, capabilityRepo)

	healthRepo := postgres.NewHealthRepository(a.pool)
	healthReportUC := apphealth.NewReportUseCase(healthRepo)
	healthGetUC := apphealth.NewGetUseCase(healthRepo)

	alertRepo := postgres.NewAlertRepository(a.pool)
	alertListUC := appalert.NewListUseCase(alertRepo)
	alertAckUC := appalert.NewAcknowledgeUseCase(alertRepo)

	notifier := a.buildAlertNotifier()
	a.evaluator = appalert.NewEvaluator(a.log, alertRepo, notifier, &appalert.Config{
		Enabled:  a.cfg.Alerts.Enabled,
		Interval: time.Duration(a.cfg.Alerts.IntervalSeconds) * time.Second,
		Rules:    appalertRules(a.cfg),
	})

	return httpx.NewRouter(
		httpx.RouterDeps{
			Agents:        agentRepo,
			OperatorToken: a.cfg.Auth.OperatorToken,
			Logger:        a.log,
			Events:        httpx.NewAgentEventHandler(a.notifier),
		},
		httpx.NewAgentHandler(registerUC, heartbeatUC, unregisterUC),
		httpx.NewCommandHandler(createCommandUC, leaseCommandUC, executionCommandUC, approvalCommandUC, getCommandUC),
		httpx.NewCapabilityHandler(capabilityUC),
		httpx.NewHealthHandler(healthReportUC, healthGetUC),
		httpx.NewAlertHandler(alertListUC, alertAckUC),
	)
}

// buildAlertNotifier constructs the outbound webhook delivery boundary, or a
// disabled notifier when webhooks are not configured. It is nil-safe for the
// evaluator.
func (a *App) buildAlertNotifier() appalert.Notifier {
	cfg := a.cfg.Webhook
	if !cfg.Enabled {
		return nil
	}
	notifier, err := webhook.New(webhook.Options{
		URL:     cfg.URL,
		Secret:  cfg.Secret,
		Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
	}, a.log)
	if err != nil {
		a.log.Warn("alert webhook disabled due to invalid configuration", zap.Error(err))
		return nil
	}
	return notifier
}

// appalertRules converts the configured alert section into evaluator rules.
// Only enabled rules with valid parameters are returned.
func appalertRules(cfg *config.Config) []appalert.Rule {
	var rules []appalert.Rule

	if cfg.Alerts.AgentOffline.Enabled && cfg.Alerts.AgentOffline.MaxOfflineSeconds > 0 {
		rules = append(rules, appalert.Rule{
			Type:       appalert.RuleAgentOffline,
			Severity:   severityOrDefault(cfg.Alerts.AgentOffline.Severity, appalert.SeverityCritical),
			MaxOffline: time.Duration(cfg.Alerts.AgentOffline.MaxOfflineSeconds) * time.Second,
		})
	}
	if cfg.Alerts.DiskUsage.Enabled && cfg.Alerts.DiskUsage.ThresholdPercent > 0 {
		rules = append(rules, appalert.Rule{
			Type:          appalert.RuleDiskUsage,
			Severity:      severityOrDefault(cfg.Alerts.DiskUsage.Severity, appalert.SeverityWarning),
			DiskThreshold: cfg.Alerts.DiskUsage.ThresholdPercent,
		})
	}
	if cfg.Alerts.HealthReportStale.Enabled && cfg.Alerts.HealthReportStale.MaxReportAgeSeconds > 0 {
		rules = append(rules, appalert.Rule{
			Type:         appalert.RuleHealthReportStale,
			Severity:     severityOrDefault(cfg.Alerts.HealthReportStale.Severity, appalert.SeverityWarning),
			MaxReportAge: time.Duration(cfg.Alerts.HealthReportStale.MaxReportAgeSeconds) * time.Second,
		})
	}
	if cfg.Alerts.ProjectUnhealthy.Enabled {
		rules = append(rules, appalert.Rule{
			Type:          appalert.RuleProjectUnhealthy,
			Severity:      severityOrDefault(cfg.Alerts.ProjectUnhealthy.Severity, appalert.SeverityCritical),
			ProjectHealth: true,
		})
	}
	return rules
}

func severityOrDefault(v, fallback string) string {
	if v == appalert.SeverityCritical || v == appalert.SeverityWarning {
		return v
	}
	return fallback
}
