package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

const testSecretHash = "$argon2id$test"

type fakeUnregisterRepo struct {
	agents        map[uuid.UUID]*domainagent.Agent
	unregistered  map[uuid.UUID]bool
	heartbeats    map[uuid.UUID]int
	unregisterErr error
}

func newUnregisterFixture(status string) (uuid.UUID, *fakeUnregisterRepo, *UnregisterUseCase, *HeartbeatUseCase) {
	agentID := uuid.New()
	repo := &fakeUnregisterRepo{
		agents: map[uuid.UUID]*domainagent.Agent{
			agentID: {ID: agentID, Secret: testSecretHash, Status: status},
		},
		unregistered: map[uuid.UUID]bool{},
		heartbeats:   map[uuid.UUID]int{},
	}
	return agentID, repo, NewUnregisterUseCase(repo), NewHeartbeatUseCase(repo)
}

func (r *fakeUnregisterRepo) RegisterAgent(context.Context, RegisterAgentRequest) (RegisterAgentResponse, error) {
	return RegisterAgentResponse{}, nil
}

func (r *fakeUnregisterRepo) GetAgentByID(_ context.Context, id uuid.UUID) (*domainagent.Agent, error) {
	if a, ok := r.agents[id]; ok {
		return a, nil
	}
	return nil, ErrAgentNotFound
}

func (r *fakeUnregisterRepo) UpdateLastHeartbeat(_ context.Context, id uuid.UUID) error {
	r.heartbeats[id]++
	return nil
}

func (r *fakeUnregisterRepo) UnregisterAgent(_ context.Context, id uuid.UUID) error {
	if r.unregisterErr != nil {
		return r.unregisterErr
	}
	if _, ok := r.agents[id]; !ok {
		return ErrAgentNotFound
	}
	r.unregistered[id] = true
	r.agents[id].Status = StatusUnregistered
	return nil
}

type fakeHasher struct{}

func (h *fakeHasher) Hash(_ context.Context, _ string) (string, error) { return testSecretHash, nil }

func (h *fakeHasher) Verify(_ context.Context, encoded, secret string) (bool, error) {
	return encoded == testSecretHash && secret == "good-secret", nil
}

func TestUnregisterSuccess(t *testing.T) {
	agentID, repo, uc, _ := newUnregisterFixture(StatusOnline)
	resp, err := uc.Unregister(context.Background(), UnregisterRequest{
		AgentID: agentID.String(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusUnregistered {
		t.Fatalf("expected status %q, got %q", StatusUnregistered, resp.Status)
	}
	if !repo.unregistered[agentID] {
		t.Fatal("expected repository UnregisterAgent to be called")
	}
}

func TestUnregisterIdempotent(t *testing.T) {
	agentID, _, uc, _ := newUnregisterFixture(StatusUnregistered)
	resp, err := uc.Unregister(context.Background(), UnregisterRequest{
		AgentID: agentID.String(),
	})
	if err != nil {
		t.Fatalf("expected idempotent success, got: %v", err)
	}
	if resp.Status != StatusUnregistered {
		t.Fatalf("expected status %q, got %q", StatusUnregistered, resp.Status)
	}
}

func TestUnregisterInvalidAgentID(t *testing.T) {
	_, _, uc, _ := newUnregisterFixture(StatusOnline)
	_, err := uc.Unregister(context.Background(), UnregisterRequest{
		AgentID: "not-a-uuid",
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got: %v", err)
	}
}

func TestUnregisterUnknownAgent(t *testing.T) {
	_, _, uc, _ := newUnregisterFixture(StatusOnline)
	_, err := uc.Unregister(context.Background(), UnregisterRequest{
		AgentID: uuid.New().String(),
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got: %v", err)
	}
}

func TestUnregisterRepositoryError(t *testing.T) {
	agentID, repo, uc, _ := newUnregisterFixture(StatusOnline)
	repo.unregisterErr = errors.New("db down")
	_, err := uc.Unregister(context.Background(), UnregisterRequest{
		AgentID: agentID.String(),
	})
	if err == nil {
		t.Fatal("expected error from repository to propagate")
	}
}

func TestHeartbeatRejectsUnregistered(t *testing.T) {
	agentID, repo, _, heartbeat := newUnregisterFixture(StatusUnregistered)
	_, err := heartbeat.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	})
	if !errors.Is(err, ErrAgentUnregistered) {
		t.Fatalf("expected ErrAgentUnregistered, got: %v", err)
	}
	if repo.heartbeats[agentID] != 0 {
		t.Fatalf("expected no heartbeat update for unregistered agent, got %d", repo.heartbeats[agentID])
	}
}

func TestHeartbeatAcceptedForOnline(t *testing.T) {
	agentID, repo, _, heartbeat := newUnregisterFixture(StatusOnline)
	if _, err := heartbeat.Heartbeat(context.Background(), HeartbeatRequest{
		AgentID: agentID.String(),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.heartbeats[agentID] != 1 {
		t.Fatalf("expected one heartbeat update, got %d", repo.heartbeats[agentID])
	}
}
