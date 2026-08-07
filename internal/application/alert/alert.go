// Package alert implements alert rule evaluation and lifecycle management.
//
// Alert rules are declarative conditions that run in-process inside central on
// a configured interval. Each evaluation snapshots the current signal for every
// active agent (heartbeat freshness, latest health report, disk usage, project
// health) and opens or resolves alerts per (agent, rule) pair. An agent in a
// degraded state that would reopen an existing open alert instead advances the
// alert's last_seen_at, so no duplicate alert is ever created for a continuous
// condition.
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Rule types, matched against the (agent, rule_type) partial unique index.
const (
	RuleAgentOffline      = "agent_offline"
	RuleDiskUsage         = "disk_usage"
	RuleHealthReportStale = "health_report_stale"
	RuleProjectUnhealthy  = "project_unhealthy"
)

// Rule severities.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
)

// Alert lifecycle states.
const (
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
)

var (
	// ErrAlertNotFound is returned when an alert id does not exist.
	ErrAlertNotFound = errors.New("alert not found")
	// ErrInvalidAlertID is returned when an alert id cannot be parsed.
	ErrInvalidAlertID = errors.New("invalid alert id")
	// ErrInvalidAgentID is returned when an agent id cannot be parsed.
	ErrInvalidAgentID = errors.New("invalid agent id")
	// ErrInvalidServerID is returned when a server id cannot be parsed.
	ErrInvalidServerID = errors.New("invalid server id")
	// ErrInvalidRuleType is returned for an unknown rule type.
	ErrInvalidRuleType = errors.New("invalid rule type")
	// ErrAcknowledgedByRequired is returned when acknowledging without an actor.
	ErrAcknowledgedByRequired = errors.New("acknowledged_by is required")
)

// Rule is a single declarative alert rule. A rule with a zero threshold never
// fires. All durations are zero by default, which disables the rule.
type Rule struct {
	Type          string        `json:"type"`
	Severity      string        `json:"severity"`
	MaxOffline    time.Duration `json:"max_offline"`
	DiskThreshold float64       `json:"disk_threshold_percent"`
	MaxReportAge  time.Duration `json:"max_report_age"`
	// ProjectHealth checks the project health probe in the health snapshot. A
	// probe reported as unhealthy opens the alert; a healthy probe resolves it.
	ProjectHealth bool `json:"project_health"`
}

// Config carries the evaluator's runtime settings.
type Config struct {
	// Enabled turns the evaluator loop on or off. When disabled, no alert rules
	// are evaluated and existing alerts are left untouched.
	Enabled bool `json:"enabled"`
	// Interval is how often the evaluator runs. Zero disables the loop.
	Interval time.Duration `json:"interval"`
	// Rules are the active alert rules. Empty rules never fire anything.
	Rules []Rule `json:"rules"`
}

// Alert is the persisted alert state.
type Alert struct {
	ID             uuid.UUID
	AgentID        uuid.UUID
	ServerID       uuid.UUID
	RuleType       string
	Severity       string
	Status         string
	Message        string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	ResolvedAt     *time.Time
	AcknowledgedAt *time.Time
	AcknowledgedBy *string
}

// AgentSignal is the per-agent evaluation input produced by a single sweep of
// ListAgentsForEvaluation.
type AgentSignal struct {
	AgentID       uuid.UUID
	ServerID      uuid.UUID
	AgentStatus   string
	LastHeartbeat *time.Time
	LastHealthAt  *time.Time
	// HealthStatus is the latest report's status, nil when the agent has never
	// reported.
	HealthStatus    *string
	DiskUsedPercent *float64
	Snapshot        []byte
}

// EvaluateReport is the outcome of one rule evaluation for one agent.
type EvaluateReport struct {
	RuleType string
	Message  string
}

// Repository persists alerts and reads evaluation signals.
type Repository interface {
	// UpsertOpenAlert opens or advances the open alert for an (agent, rule).
	// Created reports whether a brand-new alert was inserted.
	UpsertOpenAlert(ctx context.Context, agentID uuid.UUID, serverID uuid.UUID, ruleType, severity, message string) (created bool, err error)
	// ResolveOpenAlert resolves the open/acknowledged alert for an (agent,
	// rule) pair. It returns the resolved alert, or (nil, nil) when nothing was
	// open to resolve.
	ResolveOpenAlert(ctx context.Context, agentID uuid.UUID, ruleType string) (*Alert, error)
	// ListAgentsForEvaluation returns every active agent with its evaluation
	// signal, including agents that have never reported health.
	ListAgentsForEvaluation(ctx context.Context) ([]AgentSignal, error)
}

// ReadRepository lists alerts for operators and the MCP.
type ReadRepository interface {
	List(ctx context.Context, status, severity, agentID, serverID string, limit int) ([]Alert, error)
	GetByID(ctx context.Context, id string) (Alert, error)
	Acknowledge(ctx context.Context, id, acknowledgedBy string) (Alert, error)
}

// Notifier is the outbound delivery boundary for alert events. It is optional:
// when nil, alert lifecycle changes are only persisted.
type Notifier interface {
	// NotifyAlertEvent delivers an alert lifecycle event. Implementations must
	// return quickly and never block evaluation; the evaluator treats a
	// delivery error as non-fatal.
	NotifyAlertEvent(ctx context.Context, event AlertEvent) error
}

// AlertEvent is the outbound description of an alert lifecycle change.
type AlertEvent struct {
	EventID     string    `json:"event_id"`
	EventType   string    `json:"event_type"`
	AgentID     uuid.UUID `json:"agent_id"`
	ServerID    uuid.UUID `json:"server_id"`
	RuleType    string    `json:"rule_type"`
	Severity    string    `json:"severity"`
	Message     string    `json:"message"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// ProjectHealth extracts the project health probe from a health snapshot. It
// returns nil when the snapshot carries no probe data.
func ProjectHealth(snapshot []byte) *struct {
	Project string `json:"project"`
	Healthy bool   `json:"healthy"`
	URL     string `json:"url"`
	Error   string `json:"error"`
} {
	if len(snapshot) == 0 {
		return nil
	}
	var probe struct {
		ProjectHealth *struct {
			Project string `json:"project"`
			Healthy bool   `json:"healthy"`
			URL     string `json:"url"`
			Error   string `json:"error"`
		} `json:"project_health"`
	}
	if err := json.Unmarshal(snapshot, &probe); err != nil {
		return nil
	}
	return probe.ProjectHealth
}
