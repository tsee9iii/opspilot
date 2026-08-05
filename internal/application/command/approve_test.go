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
