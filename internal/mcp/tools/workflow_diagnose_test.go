package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

func TestWorkflowDiagnoseToolAwaitingApproval(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationPending},
	}
	tool := NewWorkflowDiagnoseTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got workflowResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CommandID != commandID.String() || got.Status != "awaiting_approval" || got.Message == "" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if repo.created[0].Tool != dispatch.WorkflowDiagnoseTool {
		t.Fatalf("expected workflow.diagnose dispatched, got %s", repo.created[0].Tool)
	}
}

func TestWorkflowDiagnoseToolCompleted(t *testing.T) {
	commandID := uuid.New()
	report := []byte(`{"workflow":"diagnose","status":"completed","steps":[]}`)
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             report,
		},
	}
	tool := NewWorkflowDiagnoseTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "service": "api"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got workflowResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "completed" || string(got.Report) != string(report) {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestWorkflowDiagnoseToolFailed(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusFailed,
			Error:              "systemctl failed",
		},
	}
	tool := NewWorkflowDiagnoseTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got workflowResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "failed" || got.Error != "systemctl failed" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestWorkflowDiagnoseToolInvalidAgentID(t *testing.T) {
	tool := NewWorkflowDiagnoseTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": "nope"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_agent_id" {
		t.Fatalf("expected invalid_agent_id, got: %v", err)
	}
}

func TestWorkflowDiagnoseToolInvalidTimeout(t *testing.T) {
	tool := NewWorkflowDiagnoseTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String(), "timeout_seconds": 99999})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestWorkflowDiagnoseToolMapsInvalidAgentID(t *testing.T) {
	repo := &dispatchRepo{createErr: appcommand.ErrInvalidAgentID}
	tool := NewWorkflowDiagnoseTool(newDispatch(repo))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_agent_id" {
		t.Fatalf("expected invalid_agent_id, got: %v", err)
	}
}

func TestWorkflowDiagnoseToolMapsTimeout(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusPending,
		},
	}
	uc := newDispatch(repo)
	uc.PollInterval = time.Millisecond
	tool := NewWorkflowDiagnoseTool(uc)

	_, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(), "timeout_seconds": 1,
	})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "command_timeout" {
		t.Fatalf("expected command_timeout, got: %v", err)
	}
}
