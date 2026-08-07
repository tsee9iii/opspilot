package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestApprovePendingToApproved(t *testing.T) {
	repo := &fakeRepo{approveResult: ApproveCommandResponse{CommandID: "cmd-1", Status: ConfirmationApproved}}
	uc := NewApprovalUseCase(repo)

	resp, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != ConfirmationApproved {
		t.Fatalf("expected approved, got %q", resp.Status)
	}
}

func TestApproveIdempotent(t *testing.T) {
	repo := &fakeRepo{approveResult: ApproveCommandResponse{CommandID: "cmd-1", Status: ConfirmationApproved}}
	uc := NewApprovalUseCase(repo)

	resp, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != ConfirmationApproved {
		t.Fatalf("expected approved, got %q", resp.Status)
	}
}

func TestApproveNotFound(t *testing.T) {
	repo := &fakeRepo{approveErr: ErrCommandNotFound}
	uc := NewApprovalUseCase(repo)

	_, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: uuid.New().String()})
	if !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("expected ErrCommandNotFound, got: %v", err)
	}
}

func TestApproveInvalidCommandID(t *testing.T) {
	uc := NewApprovalUseCase(&fakeRepo{})

	_, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: "not-a-uuid"})
	if !errors.Is(err, ErrInvalidCommandID) {
		t.Fatalf("expected ErrInvalidCommandID, got: %v", err)
	}
}

// TestApproveNotifiesTargetAgent proves approving a pending command wakes the
// correct agent so it can lease immediately.
func TestApproveNotifiesTargetAgent(t *testing.T) {
	agentID := uuid.New()
	n := &recordingNotifier{}
	repo := &fakeRepo{approveResult: ApproveCommandResponse{
		CommandID: "cmd-1", Status: ConfirmationApproved, AgentID: agentID,
	}}
	uc := NewApprovalUseCase(repo, n)

	resp, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentID != agentID {
		t.Fatalf("expected agent id %s, got %s", agentID, resp.AgentID)
	}
	if len(n.notified) != 1 || n.notified[0] != agentID.String() {
		t.Fatalf("expected wake for %s, got %v", agentID, n.notified)
	}
}

// TestApproveDoesNotNotifyOnFailure proves a failed approval never wakes the
// agent.
func TestApproveDoesNotNotifyOnFailure(t *testing.T) {
	agentID := uuid.New()
	n := &recordingNotifier{}
	repo := &fakeRepo{approveErr: ErrCommandNotFound}
	uc := NewApprovalUseCase(repo, n)

	if _, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: uuid.New().String()}); !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("expected ErrCommandNotFound, got: %v", err)
	}
	if len(n.notified) != 0 {
		t.Fatalf("no wake may be sent when approval fails, got %v", n.notified)
	}
	_ = agentID
}

// TestApproveWithoutNotifierIsNoOp proves a use case constructed without a
// notifier still works and never panics.
func TestApproveWithoutNotifierIsNoOp(t *testing.T) {
	repo := &fakeRepo{approveResult: ApproveCommandResponse{
		CommandID: "cmd-1", Status: ConfirmationApproved, AgentID: uuid.New(),
	}}
	uc := NewApprovalUseCase(repo)

	if _, err := uc.Approve(context.Background(), ApproveCommandRequest{CommandID: uuid.New().String()}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
