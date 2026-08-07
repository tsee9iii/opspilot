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
