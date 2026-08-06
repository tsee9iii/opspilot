package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeServerRepo struct {
	servers []ServerSummary
	err     error
}

func (f *fakeServerRepo) ListServers(context.Context) ([]ServerSummary, error) {
	return f.servers, f.err
}

type fakeAgentRepo struct {
	agents []AgentSummary
	err    error
	got    ListAgentsRequest
}

func (f *fakeAgentRepo) ListAgents(_ context.Context, req ListAgentsRequest) ([]AgentSummary, error) {
	f.got = req
	return f.agents, f.err
}

type fakeCommandRepo struct {
	commands []CommandSummary
	err      error
	got      ListCommandsRequest
}

func (f *fakeCommandRepo) ListCommands(_ context.Context, req ListCommandsRequest) ([]CommandSummary, error) {
	f.got = req
	return f.commands, f.err
}

func TestListServers(t *testing.T) {
	repo := &fakeServerRepo{servers: []ServerSummary{{Name: "edge-1", AgentCount: 2, OnlineAgentCount: 1}}}
	uc := NewListServersUseCase(repo)

	got, err := uc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "edge-1" || got[0].AgentCount != 2 || got[0].OnlineAgentCount != 1 {
		t.Fatalf("unexpected servers: %+v", got)
	}
}

func TestListServersPropagatesError(t *testing.T) {
	uc := NewListServersUseCase(&fakeServerRepo{err: errors.New("boom")})
	if _, err := uc.List(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestListAgentsDefaultFilters(t *testing.T) {
	repo := &fakeAgentRepo{agents: []AgentSummary{{ID: uuid.New(), Status: "online"}}}
	uc := NewListAgentsUseCase(repo)

	got, err := uc.List(context.Background(), ListAgentsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Status != "online" {
		t.Fatalf("unexpected agents: %+v", got)
	}
	if repo.got.ServerID != nil || repo.got.Status != "" {
		t.Fatalf("expected empty filters, got: %+v", repo.got)
	}
}

func TestListAgentsRejectsZeroServerID(t *testing.T) {
	zero := uuid.Nil
	uc := NewListAgentsUseCase(&fakeAgentRepo{})
	_, err := uc.List(context.Background(), ListAgentsRequest{ServerID: &zero})
	if !errors.Is(err, ErrInvalidServerID) {
		t.Fatalf("expected ErrInvalidServerID, got: %v", err)
	}
}

func TestListAgentsValidServerID(t *testing.T) {
	id := uuid.New()
	repo := &fakeAgentRepo{}
	uc := NewListAgentsUseCase(repo)
	if _, err := uc.List(context.Background(), ListAgentsRequest{ServerID: &id, Status: "offline"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got.ServerID == nil || *repo.got.ServerID != id || repo.got.Status != "offline" {
		t.Fatalf("filters not forwarded: %+v", repo.got)
	}
}

func TestListCommandsDefaultsLimit(t *testing.T) {
	repo := &fakeCommandRepo{}
	uc := NewListCommandsUseCase(repo)
	if _, err := uc.List(context.Background(), ListCommandsRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got.Limit != DefaultLimit {
		t.Fatalf("expected default limit %d, got %d", DefaultLimit, repo.got.Limit)
	}
}

func TestListCommandsAppliesGivenLimit(t *testing.T) {
	repo := &fakeCommandRepo{}
	uc := NewListCommandsUseCase(repo)
	if _, err := uc.List(context.Background(), ListCommandsRequest{Limit: 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got.Limit != 10 {
		t.Fatalf("expected limit 10, got %d", repo.got.Limit)
	}
}

func TestListCommandsRejectsOverLimit(t *testing.T) {
	uc := NewListCommandsUseCase(&fakeCommandRepo{})
	_, err := uc.List(context.Background(), ListCommandsRequest{Limit: MaxLimit + 1})
	if !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("expected ErrInvalidLimit, got: %v", err)
	}
}

func TestListCommandsRejectsZeroAgentID(t *testing.T) {
	zero := uuid.Nil
	uc := NewListCommandsUseCase(&fakeCommandRepo{})
	_, err := uc.List(context.Background(), ListCommandsRequest{AgentID: &zero})
	if !errors.Is(err, ErrInvalidAgentID) {
		t.Fatalf("expected ErrInvalidAgentID, got: %v", err)
	}
}

func TestListCommandsForwardFilters(t *testing.T) {
	repo := &fakeCommandRepo{commands: []CommandSummary{{ID: uuid.New(), Tool: "system.uptime", CreatedAt: time.Now()}}}
	uc := NewListCommandsUseCase(repo)
	id := uuid.New()
	got, err := uc.List(context.Background(), ListCommandsRequest{Status: "completed", AgentID: &id, Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Tool != "system.uptime" {
		t.Fatalf("unexpected commands: %+v", got)
	}
	if repo.got.Status != "completed" || repo.got.AgentID == nil || *repo.got.AgentID != id || repo.got.Limit != 5 {
		t.Fatalf("filters not forwarded: %+v", repo.got)
	}
}

func TestListCommandsPropagatesError(t *testing.T) {
	uc := NewListCommandsUseCase(&fakeCommandRepo{err: errors.New("boom")})
	if _, err := uc.List(context.Background(), ListCommandsRequest{}); err == nil {
		t.Fatal("expected error")
	}
}
