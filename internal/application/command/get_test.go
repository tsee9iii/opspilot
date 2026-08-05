package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetReturnsCommand(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{getResult: GetCommandResponse{
		ID:                 uuid.New(),
		AgentID:            uuid.New(),
		Status:             StatusCompleted,
		ConfirmationStatus: ConfirmationApproved,
		Tool:               "system.uptime",
		Parameters:         []byte(`{"interval":5}`),
		Result:             []byte(`{"uptime_seconds":42}`),
		CreatedAt:          now,
	}}
	uc := NewGetCommandUseCase(repo)

	resp, err := uc.Get(context.Background(), GetCommandRequest{CommandID: uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusCompleted || resp.Tool != "system.uptime" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetRejectsInvalidCommandID(t *testing.T) {
	uc := NewGetCommandUseCase(&fakeRepo{})

	_, err := uc.Get(context.Background(), GetCommandRequest{CommandID: "not-a-uuid"})
	if !errors.Is(err, ErrInvalidCommandID) {
		t.Fatalf("expected ErrInvalidCommandID, got: %v", err)
	}
}

func TestGetPropagatesNotFound(t *testing.T) {
	repo := &fakeRepo{getErr: ErrCommandNotFound}
	uc := NewGetCommandUseCase(repo)

	_, err := uc.Get(context.Background(), GetCommandRequest{CommandID: uuid.New().String()})
	if !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("expected ErrCommandNotFound, got: %v", err)
	}
}
