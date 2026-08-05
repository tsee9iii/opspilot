package capability

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	appagent "github.com/tsee9iii/opspilot/internal/application/agent"
	domainagent "github.com/tsee9iii/opspilot/internal/domain/agent"
)

const secretHash = "$argon2id$test"

type fakeAgentRepo struct {
	agents map[uuid.UUID]*domainagent.Agent
}

func (r *fakeAgentRepo) GetAgentByID(_ context.Context, id uuid.UUID) (*domainagent.Agent, error) {
	if a, ok := r.agents[id]; ok {
		return a, nil
	}
	return nil, appagent.ErrAgentNotFound
}

type fakeCapabilityRepo struct {
	upserted []Capability
}

func (r *fakeCapabilityRepo) Upsert(_ context.Context, _ uuid.UUID, cap Capability) error {
	r.upserted = append(r.upserted, cap)
	return nil
}

type fakeHasher struct{}

func (h *fakeHasher) Hash(_ context.Context, _ string) (string, error) { return secretHash, nil }

func (h *fakeHasher) Verify(_ context.Context, encoded, secret string) (bool, error) {
	return encoded == secretHash && secret == "good-secret", nil
}

func newFixture() (uuid.UUID, *SyncUseCase, *fakeCapabilityRepo) {
	agentID := uuid.New()
	agents := &fakeAgentRepo{agents: map[uuid.UUID]*domainagent.Agent{
		agentID: {ID: agentID, Secret: secretHash},
	}}
	caps := &fakeCapabilityRepo{}
	uc := NewSyncUseCase(agents, caps, &fakeHasher{})
	return agentID, uc, caps
}

func TestSyncInvalidAgentID(t *testing.T) {
	_, uc, _ := newFixture()
	_, err := uc.Sync(context.Background(), SyncRequest{
		AgentID: "not-a-uuid", Secret: "good-secret",
		Capabilities: []Capability{{ToolName: "system.uptime"}},
	})
	if !errors.Is(err, ErrInvalidAgentID) {
		t.Fatalf("expected ErrInvalidAgentID, got: %v", err)
	}
}

func TestSyncEmptyCapabilities(t *testing.T) {
	agentID, uc, _ := newFixture()
	_, err := uc.Sync(context.Background(), SyncRequest{AgentID: agentID.String(), Secret: "good-secret"})
	if !errors.Is(err, ErrCapabilitiesRequired) {
		t.Fatalf("expected ErrCapabilitiesRequired, got: %v", err)
	}
}

func TestSyncAgentNotFound(t *testing.T) {
	_, uc, _ := newFixture()
	_, err := uc.Sync(context.Background(), SyncRequest{
		AgentID: uuid.New().String(), Secret: "good-secret",
		Capabilities: []Capability{{ToolName: "system.uptime"}},
	})
	if !errors.Is(err, appagent.ErrAgentNotFound) {
		t.Fatalf("expected ErrAgentNotFound, got: %v", err)
	}
}

func TestSyncSecretMismatch(t *testing.T) {
	agentID, uc, _ := newFixture()
	_, err := uc.Sync(context.Background(), SyncRequest{
		AgentID: agentID.String(), Secret: "wrong",
		Capabilities: []Capability{{ToolName: "system.uptime"}},
	})
	if !errors.Is(err, appagent.ErrAgentSecretMismatch) {
		t.Fatalf("expected ErrAgentSecretMismatch, got: %v", err)
	}
}

func TestSyncRejectsUnregistered(t *testing.T) {
	agentID := uuid.New()
	agents := &fakeAgentRepo{agents: map[uuid.UUID]*domainagent.Agent{
		agentID: {ID: agentID, Secret: secretHash, Status: appagent.StatusUnregistered},
	}}
	caps := &fakeCapabilityRepo{}
	uc := NewSyncUseCase(agents, caps, &fakeHasher{})

	_, err := uc.Sync(context.Background(), SyncRequest{
		AgentID: agentID.String(), Secret: "good-secret",
		Capabilities: []Capability{{ToolName: "system.uptime"}},
	})
	if !errors.Is(err, appagent.ErrAgentUnregistered) {
		t.Fatalf("expected ErrAgentUnregistered, got: %v", err)
	}
	if len(caps.upserted) != 0 {
		t.Fatalf("expected no upserts for unregistered agent, got %d", len(caps.upserted))
	}
}

func TestSyncSuccess(t *testing.T) {
	agentID, uc, caps := newFixture()
	resp, err := uc.Sync(context.Background(), SyncRequest{
		AgentID: agentID.String(), Secret: "good-secret",
		Capabilities: []Capability{
			{ToolName: "system.uptime", Version: "1.0.0", Description: "uptime", ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "none"},
			{ToolName: "pm2.restart", Version: "1.0.0", Description: "restart", ParameterSchema: []byte(`{"type":"object","required":["process"],"properties":{"process":{"type":"string"}}}`), Confirmation: "required"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("unexpected count: %d", resp.Count)
	}
	if len(caps.upserted) != 2 {
		t.Fatalf("unexpected upserts: %d", len(caps.upserted))
	}
	if caps.upserted[0].ToolName != "system.uptime" {
		t.Fatalf("unexpected first tool: %v", caps.upserted[0])
	}
	if string(caps.upserted[0].ParameterSchema) != `{"type":"object","properties":{}}` {
		t.Fatalf("unexpected first schema: %s", caps.upserted[0].ParameterSchema)
	}
	if string(caps.upserted[1].ParameterSchema) != `{"type":"object","required":["process"],"properties":{"process":{"type":"string"}}}` {
		t.Fatalf("unexpected second schema: %s", caps.upserted[1].ParameterSchema)
	}
	if caps.upserted[0].Confirmation != "none" {
		t.Fatalf("unexpected first confirmation: %s", caps.upserted[0].Confirmation)
	}
	if caps.upserted[1].Confirmation != "required" {
		t.Fatalf("unexpected second confirmation: %s", caps.upserted[1].Confirmation)
	}
}

func TestSyncPersistsAvailability(t *testing.T) {
	agentID, uc, caps := newFixture()
	resp, err := uc.Sync(context.Background(), SyncRequest{
		AgentID: agentID.String(), Secret: "good-secret",
		Capabilities: []Capability{
			{ToolName: "docker.ps", Version: "1.0.0", Description: "ps", ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "none", Available: true},
			{ToolName: "pm2.list", Version: "1.0.0", Description: "list", ParameterSchema: []byte(`{"type":"object","properties":{}}`), Confirmation: "none", Available: false, UnavailableReason: "pm2 is not installed"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("unexpected count: %d", resp.Count)
	}
	if !caps.upserted[0].Available || caps.upserted[0].UnavailableReason != "" {
		t.Fatalf("expected docker.ps available, got %+v", caps.upserted[0])
	}
	if caps.upserted[1].Available || caps.upserted[1].UnavailableReason != "pm2 is not installed" {
		t.Fatalf("expected pm2.list unavailable with reason, got %+v", caps.upserted[1])
	}
}
