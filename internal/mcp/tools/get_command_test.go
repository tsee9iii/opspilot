package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

func TestGetCommandToolSuccess(t *testing.T) {
	id := uuid.New()
	agentID := uuid.New()
	created := time.Now().UTC()
	repo := &dispatchRepo{getRes: appcommand.GetCommandResponse{
		ID:                 id,
		AgentID:            agentID,
		Status:             appcommand.StatusCompleted,
		ConfirmationStatus: appcommand.ConfirmationApproved,
		Tool:               "workflow.diagnose",
		Parameters:         []byte(`{"service":"api"}`),
		Result:             []byte(`{"workflow":"diagnose","status":"completed"}`),
		CreatedAt:          created,
	}}
	tool := NewGetCommandTool(appcommand.NewGetCommandUseCase(repo))

	out, err := tool.Call(context.Background(), map[string]any{"command_id": id.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got struct {
		Command commandResult `json:"command"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	c := got.Command
	if c.ID != id.String() || c.AgentID != agentID.String() || c.Status != appcommand.StatusCompleted ||
		c.Tool != "workflow.diagnose" || c.CreatedAt != created.Format(time.RFC3339) {
		t.Fatalf("unexpected command: %+v", c)
	}
	if string(c.Parameters) != `{"service":"api"}` || string(c.Result) != `{"workflow":"diagnose","status":"completed"}` {
		t.Fatalf("parameters/result not passed through: %s %s", c.Parameters, c.Result)
	}
}

func TestGetCommandToolInvalidID(t *testing.T) {
	tool := NewGetCommandTool(appcommand.NewGetCommandUseCase(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{"command_id": "nope"})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_command_id" {
		t.Fatalf("expected invalid_command_id, got: %v", err)
	}
}

func TestGetCommandToolNotFound(t *testing.T) {
	tool := NewGetCommandTool(appcommand.NewGetCommandUseCase(&dispatchRepo{getErr: appcommand.ErrCommandNotFound}))
	_, err := tool.Call(context.Background(), map[string]any{"command_id": uuid.New().String()})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "command_not_found" {
		t.Fatalf("expected command_not_found, got: %v", err)
	}
}

func TestGetCommandToolMissingID(t *testing.T) {
	tool := NewGetCommandTool(appcommand.NewGetCommandUseCase(&dispatchRepo{}))
	_, err := tool.Call(context.Background(), map[string]any{})
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != "invalid_args" {
		t.Fatalf("expected invalid_args, got: %v", err)
	}
}

func TestGetCommandToolInternalError(t *testing.T) {
	tool := NewGetCommandTool(appcommand.NewGetCommandUseCase(&dispatchRepo{getErr: errors.New("db down")}))
	_, err := tool.Call(context.Background(), map[string]any{"command_id": uuid.New().String()})
	if err == nil {
		t.Fatal("expected error")
	}
	var te *mcp.ToolError
	if errors.As(err, &te) {
		t.Fatalf("generic repo errors surface as internal_error in the server, not as ToolError: %v", err)
	}
}
