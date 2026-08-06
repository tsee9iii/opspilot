package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

// fakeHeartbeatRepo models the repository contract required by the heartbeat
// use case: an accepted heartbeat updates last_heartbeat/updated_at and
// transitions offline -> online, while unregistered (and any other non-active)
// statuses are never touched.
type fakeHeartbeatRepo struct {
	agents map[uuid.UUID]*domainagent.Agent
}

func newHeartbeatFixture(status string) (uuid.UUID, *fakeHeartbeatRepo, *HeartbeatUseCase) {
	agentID := uuid.New()
	repo := &fakeHeartbeatRepo{agents: map[uuid.UUID]*domainagent.Agent{
		agentID: {ID: agentID, Secret: testSecretHash, Status: status},
	}}
	return agentID, repo, NewHeartbeatUseCase(repo)
}

func (r *fakeHeartbeatRepo) RegisterAgent(context.Context, RegisterAgentRequest) (RegisterAgentResponse, error) {
	return RegisterAgentResponse{}, nil
}

func (r *fakeHeartbeatRepo) GetAgentByID(_ context.Context, id uuid.UUID) (*domainagent.Agent, error) {
	if a, ok := r.agents[id]; ok {
		return a, nil
	}
	return nil, ErrAgentNotFound
}

func (r *fakeHeartbeatRepo) UpdateLastHeartbeat(_ context.Context, id uuid.UUID) error {
	a, ok := r.agents[id]
	if !ok {
		return ErrAgentNotFound
	}
	// Match the repository: only offline/online agents are touched.
	if a.Status != StatusOffline && a.Status != StatusOnline {
		return nil
	}
	now := time.Now()
	a.LastHeartbeat = &now
	a.UpdatedAt = now
	if a.Status == StatusOffline {
		a.Status = StatusOnline
	}
	return nil
}

func (r *fakeHeartbeatRepo) UnregisterAgent(context.Context, uuid.UUID) error { return nil }

func TestHeartbeatOfflineToOnline(t *testing.T) {
	agentID, repo, uc := newHeartbeatFixture(StatusOffline)
	if _, err := uc.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := repo.agents[agentID].Status; got != StatusOnline {
		t.Fatalf("expected %q after heartbeat, got %q", StatusOnline, got)
	}
}

func TestHeartbeatOnlineStaysOnline(t *testing.T) {
	agentID, repo, uc := newHeartbeatFixture(StatusOnline)
	if _, err := uc.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := repo.agents[agentID].Status; got != StatusOnline {
		t.Fatalf("expected %q to stay %q after heartbeat, got %q", StatusOnline, StatusOnline, got)
	}
}

func TestHeartbeatUnregisteredNotResurrected(t *testing.T) {
	agentID, repo, uc := newHeartbeatFixture(StatusUnregistered)
	if _, err := uc.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	}); !errors.Is(err, ErrAgentUnregistered) {
		t.Fatalf("expected ErrAgentUnregistered, got: %v", err)
	}
	if got := repo.agents[agentID].Status; got != StatusUnregistered {
		t.Fatalf("expected status to stay %q, got %q", StatusUnregistered, got)
	}
	if repo.agents[agentID].LastHeartbeat != nil {
		t.Fatal("unregistered agent must not have its last_heartbeat updated")
	}
}

func TestHeartbeatUpdatesLastHeartbeat(t *testing.T) {
	agentID, repo, uc := newHeartbeatFixture(StatusOnline)
	before := time.Now()
	if _, err := uc.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := repo.agents[agentID].LastHeartbeat
	if last == nil {
		t.Fatal("expected last_heartbeat to be set after heartbeat")
	}
	if !last.After(before) {
		t.Fatalf("expected last_heartbeat after %v, got %v", before, *last)
	}
}

func TestHeartbeatUpdatesUpdatedAt(t *testing.T) {
	agentID, repo, uc := newHeartbeatFixture(StatusOffline)
	before := time.Now()
	if _, err := uc.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.agents[agentID].UpdatedAt.After(before) {
		t.Fatalf("expected updated_at after %v, got %v", before, repo.agents[agentID].UpdatedAt)
	}
}
