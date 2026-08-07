package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCreateApprovedForNoneTool(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "none", nil
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 create, got %d", len(repo.created))
	}
	if repo.created[0].ConfirmationStatus != ConfirmationApproved {
		t.Fatalf("expected approved, got %q", repo.created[0].ConfirmationStatus)
	}
}

func TestCreatePendingForRequiredTool(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return ConfirmationRequiredLevel, nil
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "pm2.restart", Payload: []byte(`{"process":"web"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 create, got %d", len(repo.created))
	}
	if repo.created[0].ConfirmationStatus != ConfirmationPending {
		t.Fatalf("expected pending, got %q", repo.created[0].ConfirmationStatus)
	}
}

// TestCreateMCPAlwaysPending fails closed: a command created by the MCP path is
// always pending, even for tools whose capability metadata would otherwise be
// auto-approved. Only an independent operator approval can release it.
func TestCreateMCPAlwaysPending(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "none", nil
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
		Source: SourceMCP, RequestedBy: "hermes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created[0].Source != SourceMCP {
		t.Fatalf("expected source mcp, got %q", repo.created[0].Source)
	}
	if repo.created[0].RequestedBy != "hermes" {
		t.Fatalf("expected requested_by hermes, got %q", repo.created[0].RequestedBy)
	}
	if repo.created[0].ConfirmationStatus != ConfirmationPending {
		t.Fatalf("MCP commands must always be pending, got %q", repo.created[0].ConfirmationStatus)
	}
}

// TestCreateDefaultsSourceToAPI pins that an unset source is recorded as 'api'
// (an authenticated operator request) rather than passing through empty.
func TestCreateDefaultsSourceToAPI(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "none", nil
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.created[0].Source != SourceAPI {
		t.Fatalf("expected default source api, got %q", repo.created[0].Source)
	}
}

func TestCreateRejectsUnknownCapability(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "", nil
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "noop", Payload: []byte(`{}`),
	})
	if !errors.Is(err, ErrCapabilityNotFound) {
		t.Fatalf("expected ErrCapabilityNotFound, got: %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatalf("no command may be persisted for an unknown tool, got %d", len(repo.created))
	}
}

func TestCreatePropagatesCapabilityNotFound(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "", ErrCapabilityNotFound
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "unknown.tool", Payload: []byte(`{}`),
	})
	if !errors.Is(err, ErrCapabilityNotFound) {
		t.Fatalf("expected ErrCapabilityNotFound, got: %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatalf("no command may be persisted for an unknown tool, got %d", len(repo.created))
	}
}

func TestCreateRejectsUnavailableCapability(t *testing.T) {
	agentID := uuid.New()
	repo := &fakeRepo{}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "", ErrCapabilityUnavailable
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "pm2.restart", Payload: []byte(`{}`),
	})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("expected ErrCapabilityUnavailable, got: %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatalf("no command may be persisted for an unavailable tool, got %d", len(repo.created))
	}
}

func TestCreatePropagatesResolverError(t *testing.T) {
	agentID := uuid.New()
	uc := NewCreateUseCase(&fakeRepo{}, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "", errors.New("boom")
	}})

	_, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "pm2.restart", Payload: []byte(`{}`),
	})
	if err == nil || !strings.Contains(err.Error(), "resolve confirmation") {
		t.Fatalf("expected resolve confirmation error, got: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	uc := NewCreateUseCase(&fakeRepo{}, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "", nil
	}})

	tests := []struct {
		name string
		req  CreateCommandRequest
		want error
	}{
		{"invalid agent id", CreateCommandRequest{AgentID: "not-a-uuid", Tool: "t", Payload: []byte(`{}`)}, ErrInvalidAgentID},
		{"missing tool", CreateCommandRequest{AgentID: uuid.New().String(), Tool: "", Payload: []byte(`{}`)}, ErrToolRequired},
		{"missing payload", CreateCommandRequest{AgentID: uuid.New().String(), Tool: "t", Payload: nil}, ErrPayloadRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.Create(context.Background(), tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got: %v", tt.want, err)
			}
		})
	}
}

type recordingNotifier struct {
	notified []string
}

func (n *recordingNotifier) Notify(agentID string) {
	n.notified = append(n.notified, agentID)
}

// TestCreateNotifiesWhenLeasableNow proves a successfully persisted,
// operator-approved command wakes the target agent immediately.
func TestCreateNotifiesWhenLeasableNow(t *testing.T) {
	agentID := uuid.New()
	n := &recordingNotifier{}
	uc := NewCreateUseCase(&fakeRepo{}, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "none", nil
	}}, n)

	if _, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(n.notified) != 1 || n.notified[0] != agentID.String() {
		t.Fatalf("expected wake for %s, got %v", agentID, n.notified)
	}
}

// TestCreateDoesNotNotifyPendingApproval proves commands awaiting human
// approval never wake the agent at creation; the approval releases them later.
func TestCreateDoesNotNotifyPendingApproval(t *testing.T) {
	agentID := uuid.New()
	n := &recordingNotifier{}
	uc := NewCreateUseCase(&fakeRepo{}, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return ConfirmationRequiredLevel, nil
	}}, n)

	if _, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "pm2.restart", Payload: []byte(`{"process":"web"}`),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(n.notified) != 0 {
		t.Fatalf("pending-approval command must not wake the agent, got %v", n.notified)
	}
}

// TestCreateMCPNeverNotifiesAtCreation proves MCP-created commands are always
// pending and therefore never wake the agent at creation, even for tools whose
// capability metadata would auto-approve.
func TestCreateMCPNeverNotifiesAtCreation(t *testing.T) {
	agentID := uuid.New()
	n := &recordingNotifier{}
	uc := NewCreateUseCase(&fakeRepo{}, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "none", nil
	}}, n)

	if _, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
		Source: SourceMCP, RequestedBy: "hermes",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(n.notified) != 0 {
		t.Fatalf("MCP command must not wake at creation, got %v", n.notified)
	}
}

// TestCreateNotifiesOnlyAfterPersist proves a failed persistence never wakes
// the agent.
func TestCreateNotifiesOnlyAfterPersist(t *testing.T) {
	agentID := uuid.New()
	n := &recordingNotifier{}
	repo := &fakeRepo{createErr: errors.New("db down")}
	uc := NewCreateUseCase(repo, &fakeResolver{level: func(uuid.UUID, string) (string, error) {
		return "none", nil
	}}, n)

	if _, err := uc.Create(context.Background(), CreateCommandRequest{
		AgentID: agentID.String(), Tool: "system.uptime", Payload: []byte(`{}`),
	}); err == nil {
		t.Fatal("expected persistence error")
	}
	if len(n.notified) != 0 {
		t.Fatalf("no wake may be sent when persistence fails, got %v", n.notified)
	}
}
