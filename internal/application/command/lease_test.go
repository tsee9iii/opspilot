package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLeaseForwardsConfiguredTTL(t *testing.T) {
	repo := &fakeRepo{}
	uc := NewLeaseUseCase(repo, 45*time.Second)

	if _, err := uc.Lease(context.Background(), LeaseCommandRequest{AgentID: uuid.New().String()}); err != nil {
		t.Fatalf("lease: %v", err)
	}
	if repo.leasedReq.LeaseTTL != 45*time.Second {
		t.Fatalf("expected lease TTL to be forwarded, got %v", repo.leasedReq.LeaseTTL)
	}
}

func TestLeaseRejectsInvalidAgentID(t *testing.T) {
	uc := NewLeaseUseCase(&fakeRepo{}, time.Second)

	_, err := uc.Lease(context.Background(), LeaseCommandRequest{AgentID: "not-a-uuid"})
	if !errors.Is(err, ErrInvalidAgentID) {
		t.Fatalf("expected ErrInvalidAgentID, got: %v", err)
	}
}
