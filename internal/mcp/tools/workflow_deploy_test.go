package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

func TestWorkflowDeployToolAwaitingApproval(t *testing.T) {
	commandID := uuid.New()
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationPending},
	}
	tool := NewWorkflowDeployTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(),
		"project":  "merchant-api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got workflowResult
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CommandID != commandID.String() || got.Status != "awaiting_approval" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if repo.created[0].Tool != dispatch.WorkflowDeployTool {
		t.Fatalf("expected workflow.deploy dispatched, got %s", repo.created[0].Tool)
	}
	if string(repo.created[0].Payload) != `{"project":"merchant-api"}` {
		t.Fatalf("unexpected deploy payload: %s", repo.created[0].Payload)
	}
}

func TestWorkflowDeployToolCompleted(t *testing.T) {
	commandID := uuid.New()
	report := []byte(`{"workflow":"deploy","strategy":"pm2","status":"completed"}`)
	repo := &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             report,
		},
	}
	tool := NewWorkflowDeployTool(newDispatch(repo))

	out, err := tool.Call(context.Background(), map[string]any{
		"agent_id": uuid.New().String(),
		"project":  "merchant-api",
	})
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

func TestWorkflowDeployToolMissingProject(t *testing.T) {
	tool := NewWorkflowDeployTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"agent_id": uuid.New().String()})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestWorkflowDeployToolMissingAgentID(t *testing.T) {
	tool := NewWorkflowDeployTool(newDispatch(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"project": "merchant-api"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}
