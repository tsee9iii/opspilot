package alert

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Evaluator runs the configured alert rules in-process against the latest
// agent signals. It is the only place alerts are opened and resolved; operator
// and MCP paths only ever acknowledge or read.
type Evaluator struct {
	log      *zap.Logger
	repo     Repository
	notifier Notifier
	config   *Config
	wg       sync.WaitGroup
}

func NewEvaluator(log *zap.Logger, repo Repository, notifier Notifier, config *Config) *Evaluator {
	return &Evaluator{log: log, repo: repo, notifier: notifier, config: config}
}

// Run starts the evaluation loop in the background. When the evaluator is
// disabled (or has no interval), it does nothing. Run returns immediately.
func (e *Evaluator) Run(ctx context.Context) {
	if !e.config.Enabled || e.config.Interval <= 0 {
		e.log.Info("alert evaluator disabled")
		return
	}
	e.wg.Add(1)
	go e.loop(ctx)
	e.log.Info("alert evaluator started", zap.Duration("interval", e.config.Interval), zap.Int("rules", len(e.config.Rules)))
}

// Wait blocks until a running evaluation loop exits (i.e. its context is
// cancelled and the current sweep finished). Call it during shutdown.
func (e *Evaluator) Wait() { e.wg.Wait() }

func (e *Evaluator) loop(ctx context.Context) {
	defer e.wg.Done()
	timer := time.NewTimer(e.config.Interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := e.EvaluateOnce(ctx); err != nil {
				e.log.Warn("alert evaluation failed", zap.Error(err))
			}
			timer.Reset(e.config.Interval)
		}
	}
}

// EvaluateOnce runs a single full sweep: for every active agent, every
// configured rule is applied and the alert state for each (agent, rule) pair
// is opened, advanced or resolved. It is safe to call concurrently with Run.
func (e *Evaluator) EvaluateOnce(ctx context.Context) error {
	signals, err := e.repo.ListAgentsForEvaluation(ctx)
	if err != nil {
		return fmt.Errorf("alert: list evaluation signals: %w", err)
	}
	now := time.Now()
	for _, sig := range signals {
		for _, rule := range e.config.Rules {
			if msg, ok := ruleFires(rule, sig, now); ok {
				if err := e.open(ctx, sig, rule, msg); err != nil {
					e.log.Warn("alert: open failed",
						zap.String("agent_id", sig.AgentID.String()),
						zap.String("rule", rule.Type),
						zap.Error(err),
					)
				}
			} else {
				if err := e.resolve(ctx, sig, rule); err != nil {
					e.log.Warn("alert: resolve failed",
						zap.String("agent_id", sig.AgentID.String()),
						zap.String("rule", rule.Type),
						zap.Error(err),
					)
				}
			}
		}
	}
	return nil
}

func (e *Evaluator) open(ctx context.Context, sig AgentSignal, rule Rule, message string) error {
	created, err := e.repo.UpsertOpenAlert(ctx, sig.AgentID, sig.ServerID, rule.Type, rule.Severity, message)
	if err != nil {
		return err
	}
	if created && e.notifier != nil {
		return e.notifier.NotifyAlertEvent(ctx, AlertEvent{
			EventID:     uuid.NewString(),
			EventType:   "alert_opened",
			AgentID:     sig.AgentID,
			ServerID:    sig.ServerID,
			RuleType:    rule.Type,
			Severity:    rule.Severity,
			Message:     message,
			FirstSeenAt: time.Now(),
			LastSeenAt:  time.Now(),
		})
	}
	return nil
}

func (e *Evaluator) resolve(ctx context.Context, sig AgentSignal, rule Rule) error {
	alertRow, err := e.repo.ResolveOpenAlert(ctx, sig.AgentID, rule.Type)
	if err != nil {
		return err
	}
	if alertRow != nil && e.notifier != nil {
		return e.notifier.NotifyAlertEvent(ctx, AlertEvent{
			EventID:     uuid.NewString(),
			EventType:   "alert_resolved",
			AgentID:     alertRow.AgentID,
			ServerID:    alertRow.ServerID,
			RuleType:    alertRow.RuleType,
			Severity:    alertRow.Severity,
			Message:     alertRow.Message,
			FirstSeenAt: alertRow.FirstSeenAt,
			LastSeenAt:  alertRow.LastSeenAt,
		})
	}
	return nil
}

// ruleFires evaluates a single rule against an agent's signal at time now. It
// returns a human-readable message and whether the rule condition holds.
// Rules with zero thresholds or durations are inert and never fire.
func ruleFires(rule Rule, sig AgentSignal, now time.Time) (string, bool) {
	switch rule.Type {
	case RuleAgentOffline:
		if rule.MaxOffline <= 0 {
			return "", false
		}
		if sig.AgentStatus == "offline" {
			return "agent is offline", true
		}
		if sig.LastHeartbeat == nil {
			return "agent has never heartbeated", true
		}
		if now.Sub(*sig.LastHeartbeat) > rule.MaxOffline {
			return fmt.Sprintf("no heartbeat for more than %s", rule.MaxOffline), true
		}
		return "", false
	case RuleDiskUsage:
		if rule.DiskThreshold <= 0 || sig.DiskUsedPercent == nil {
			return "", false
		}
		if *sig.DiskUsedPercent >= rule.DiskThreshold {
			return fmt.Sprintf("disk usage %.1f%% exceeds threshold %.1f%%", *sig.DiskUsedPercent, rule.DiskThreshold), true
		}
		return "", false
	case RuleHealthReportStale:
		if rule.MaxReportAge <= 0 {
			return "", false
		}
		if sig.LastHealthAt == nil {
			return "no health report received", true
		}
		if now.Sub(*sig.LastHealthAt) > rule.MaxReportAge {
			return fmt.Sprintf("health report older than %s", rule.MaxReportAge), true
		}
		return "", false
	case RuleProjectUnhealthy:
		if !rule.ProjectHealth {
			return "", false
		}
		probe := ProjectHealth(sig.Snapshot)
		if probe == nil {
			// No probe data yet is not an unhealthy signal.
			return "", false
		}
		if !probe.Healthy {
			msg := "project is unhealthy"
			if probe.Project != "" {
				msg = fmt.Sprintf("project %q is unhealthy", probe.Project)
			}
			if probe.Error != "" {
				msg += ": " + probe.Error
			}
			return msg, true
		}
		return "", false
	}
	return "", false
}
