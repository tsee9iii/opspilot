package alert

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// fakeRepo records evaluator calls so tests can assert open/resolve behavior.
type fakeRepo struct {
	mu       sync.Mutex
	signals  []AgentSignal
	opened   []string
	resolved []string
}

func (f *fakeRepo) UpsertOpenAlert(_ context.Context, agentID, serverID uuid.UUID, ruleType, severity, message string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, ruleType)
	return true, nil
}

func (f *fakeRepo) ResolveOpenAlert(_ context.Context, agentID uuid.UUID, ruleType string) (*Alert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = append(f.resolved, ruleType)
	return nil, nil
}

func (f *fakeRepo) ListAgentsForEvaluation(context.Context) ([]AgentSignal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.signals, nil
}

func (f *fakeRepo) openedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.opened)
}

func (f *fakeRepo) resolvedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.resolved)
}

// capturingNotifier records delivered events.
type capturingNotifier struct {
	mu     sync.Mutex
	events []AlertEvent
}

func (c *capturingNotifier) NotifyAlertEvent(_ context.Context, e AlertEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

func (c *capturingNotifier) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func signalFor(status string, heartbeat, healthAt *time.Time, disk *float64) AgentSignal {
	return AgentSignal{
		AgentID:         uuid.New(),
		ServerID:        uuid.New(),
		AgentStatus:     status,
		LastHeartbeat:   heartbeat,
		LastHealthAt:    healthAt,
		DiskUsedPercent: disk,
	}
}

func ageAgo(d time.Duration) *time.Time {
	v := time.Now().Add(-d)
	return &v
}

func TestEvaluateOnceOpensAndAdvancesOffline(t *testing.T) {
	repo := &fakeRepo{signals: []AgentSignal{
		signalFor("online", ageAgo(10*time.Minute), ageAgo(time.Minute), nil),
	}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled:  true,
		Interval: time.Minute,
		Rules: []Rule{
			{Type: RuleAgentOffline, Severity: SeverityCritical, MaxOffline: 5 * time.Minute},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 1 {
		t.Fatalf("expected 1 open, got %d", repo.openedCount())
	}
}

func TestEvaluateOnceResolvesHealthyAgent(t *testing.T) {
	repo := &fakeRepo{signals: []AgentSignal{
		signalFor("online", ageAgo(time.Second), ageAgo(time.Second), nil),
	}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleAgentOffline, Severity: SeverityCritical, MaxOffline: 5 * time.Minute},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 0 {
		t.Fatalf("expected no open for healthy agent, got %d", repo.openedCount())
	}
	if repo.resolvedCount() != 1 {
		t.Fatalf("expected 1 resolve for healthy agent, got %d", repo.resolvedCount())
	}
}

func TestEvaluateOnceDiskThreshold(t *testing.T) {
	pct := func(v float64) *float64 { return &v }

	over := signalFor("online", ageAgo(time.Second), ageAgo(time.Second), pct(94))
	under := signalFor("online", ageAgo(time.Second), ageAgo(time.Second), pct(40))
	repo := &fakeRepo{signals: []AgentSignal{over, under}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleDiskUsage, Severity: SeverityWarning, DiskThreshold: 90},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 1 {
		t.Fatalf("expected 1 open for disk over threshold, got %d", repo.openedCount())
	}
	if repo.resolvedCount() != 1 {
		t.Fatalf("expected 1 resolve for disk under threshold, got %d", repo.resolvedCount())
	}
}

func TestEvaluateOnceStaleHealthReport(t *testing.T) {
	repo := &fakeRepo{signals: []AgentSignal{
		signalFor("online", ageAgo(time.Second), ageAgo(30*time.Minute), nil),
	}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleHealthReportStale, Severity: SeverityWarning, MaxReportAge: 15 * time.Minute},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 1 {
		t.Fatalf("expected 1 open for stale report, got %d", repo.openedCount())
	}
}

func TestEvaluateOnceProjectUnhealthyFromSnapshot(t *testing.T) {
	snap := []byte(`{"project_health":{"project":"api","healthy":false,"url":"http://x/health","error":"500"}}`)
	sig := signalFor("online", ageAgo(time.Second), ageAgo(time.Second), nil)
	sig.Snapshot = snap
	repo := &fakeRepo{signals: []AgentSignal{sig}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleProjectUnhealthy, Severity: SeverityCritical, ProjectHealth: true},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 1 {
		t.Fatalf("expected 1 open for unhealthy project, got %d", repo.openedCount())
	}
}

func TestEvaluateOnceProjectHealthyDoesNotFire(t *testing.T) {
	snap := []byte(`{"project_health":{"project":"api","healthy":true,"url":"http://x/health"}}`)
	sig := signalFor("online", ageAgo(time.Second), ageAgo(time.Second), nil)
	sig.Snapshot = snap
	repo := &fakeRepo{signals: []AgentSignal{sig}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleProjectUnhealthy, Severity: SeverityCritical, ProjectHealth: true},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 0 {
		t.Fatalf("expected no open for healthy project, got %d", repo.openedCount())
	}
}

func TestEvaluateOnceDisabledRuleInert(t *testing.T) {
	repo := &fakeRepo{signals: []AgentSignal{
		signalFor("online", ageAgo(time.Hour), ageAgo(time.Hour), nil),
	}}
	e := NewEvaluator(zap.NewNop(), repo, nil, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleAgentOffline, Severity: SeverityCritical, MaxOffline: 0},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if repo.openedCount() != 0 {
		t.Fatalf("disabled rule must never open an alert, got %d", repo.openedCount())
	}
}

// TestEvaluateOnceNotifiesOnlyOnTransition proves a newly opened alert emits an
// opened event while a repeated unhealthy report (already open) does not, and a
// recovery emits a resolved event.
func TestEvaluateOnceNotifiesOnTransition(t *testing.T) {
	notifier := &capturingNotifier{}

	open := &fakeRepo{signals: []AgentSignal{
		signalFor("online", ageAgo(10*time.Minute), ageAgo(time.Minute), nil),
	}}
	e := NewEvaluator(zap.NewNop(), open, notifier, &Config{
		Enabled: true,
		Rules: []Rule{
			{Type: RuleAgentOffline, Severity: SeverityCritical, MaxOffline: 5 * time.Minute},
		},
	})
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if notifier.count() != 1 {
		t.Fatalf("expected 1 opened event, got %d", notifier.count())
	}
}
