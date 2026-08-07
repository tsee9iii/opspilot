package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
)

type fakeConfirmationResolver struct {
	level string
}

func (f *fakeConfirmationResolver) ConfirmationLevel(context.Context, uuid.UUID, string) (string, error) {
	level := f.level
	if level == "" {
		// A real resolver never returns an empty level for a known,
		// available capability (command creation fails closed).
		level = "none"
	}
	return level, nil
}

type fakeRepo struct {
	created   []appcommand.CreateCommandRequest
	createRes appcommand.CreateCommandResponse
	createErr error
	getRes    appcommand.GetCommandResponse
	getErr    error
}

func (f *fakeRepo) CreateCommand(_ context.Context, req appcommand.CreateCommandRequest) (appcommand.CreateCommandResponse, error) {
	f.created = append(f.created, req)
	if f.createErr != nil {
		return appcommand.CreateCommandResponse{}, f.createErr
	}
	return f.createRes, nil
}

func (f *fakeRepo) GetCommand(context.Context, appcommand.GetCommandRequest) (appcommand.GetCommandResponse, error) {
	return f.getRes, f.getErr
}

func (f *fakeRepo) LeaseNextCommand(context.Context, appcommand.LeaseCommandRequest) (appcommand.LeaseCommandResponse, error) {
	return appcommand.LeaseCommandResponse{}, errors.New("unused")
}

func (f *fakeRepo) StartCommand(context.Context, appcommand.StartCommandRequest) (appcommand.StartCommandResponse, error) {
	return appcommand.StartCommandResponse{}, errors.New("unused")
}

func (f *fakeRepo) CompleteCommand(context.Context, appcommand.CompleteCommandRequest) (appcommand.CompleteCommandResponse, error) {
	return appcommand.CompleteCommandResponse{}, errors.New("unused")
}

func (f *fakeRepo) FailCommand(context.Context, appcommand.FailCommandRequest) (appcommand.FailCommandResponse, error) {
	return appcommand.FailCommandResponse{}, errors.New("unused")
}

func (f *fakeRepo) ApproveCommand(context.Context, appcommand.ApproveCommandRequest) (appcommand.ApproveCommandResponse, error) {
	return appcommand.ApproveCommandResponse{}, errors.New("unused")
}

func newUseCase(t *testing.T, resolver *fakeConfirmationResolver, repo *fakeRepo) *DispatchUseCase {
	t.Helper()
	create := appcommand.NewCreateUseCase(repo, resolver)
	get := appcommand.NewGetCommandUseCase(repo)
	return NewDispatchUseCase(create, get)
}

func testPayload() []byte { return []byte(`{"service":"api"}`) }

func TestDispatchAwaitingApproval(t *testing.T) {
	agentID := uuid.New()
	commandID := uuid.New()
	repo := &fakeRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationPending},
	}
	uc := newUseCase(t, &fakeConfirmationResolver{level: appcommand.ConfirmationRequiredLevel}, repo)

	resp, err := uc.Dispatch(context.Background(), DispatchRequest{AgentID: agentID.String(), Tool: WorkflowDeployTool, Payload: testPayload()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.AwaitingApproval || resp.Status != "awaiting_approval" {
		t.Fatalf("expected awaiting approval: %+v", resp)
	}
	if resp.CommandID != commandID.String() {
		t.Fatalf("unexpected command id: %s", resp.CommandID)
	}
	if repo.created[0].Tool != WorkflowDeployTool || repo.created[0].ConfirmationStatus != appcommand.ConfirmationPending {
		t.Fatalf("unexpected create request: %+v", repo.created[0])
	}
}

func TestDispatchCompleted(t *testing.T) {
	agentID := uuid.New()
	commandID := uuid.New()
	result := []byte(`{"workflow":"diagnose","status":"completed","steps":[]}`)
	repo := &fakeRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationApproved, Status: appcommand.StatusCompleted, Result: result},
	}
	uc := newUseCase(t, &fakeConfirmationResolver{}, repo)
	uc.PollInterval = time.Millisecond

	resp, err := uc.Dispatch(context.Background(), DispatchRequest{AgentID: agentID.String(), Tool: WorkflowDiagnoseTool, Payload: testPayload()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "completed" || resp.AwaitingApproval {
		t.Fatalf("expected completed: %+v", resp)
	}
	if string(resp.Result) != string(result) {
		t.Fatalf("unexpected result: %s", resp.Result)
	}
}

func TestDispatchFailed(t *testing.T) {
	agentID := uuid.New()
	commandID := uuid.New()
	repo := &fakeRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationApproved, Status: appcommand.StatusFailed, Error: "compose up failed"},
	}
	uc := newUseCase(t, &fakeConfirmationResolver{}, repo)

	resp, err := uc.Dispatch(context.Background(), DispatchRequest{AgentID: agentID.String(), Tool: WorkflowDeployTool, Payload: testPayload()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "failed" || resp.Error != "compose up failed" {
		t.Fatalf("expected failed outcome: %+v", resp)
	}
}

func TestDispatchTimeout(t *testing.T) {
	agentID := uuid.New()
	commandID := uuid.New()
	repo := &fakeRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationApproved, Status: appcommand.StatusPending},
	}
	uc := newUseCase(t, &fakeConfirmationResolver{}, repo)
	uc.PollInterval = time.Millisecond

	_, err := uc.Dispatch(context.Background(), DispatchRequest{
		AgentID: agentID.String(),
		Tool:    WorkflowDiagnoseTool,
		Payload: testPayload(),
		Timeout: 5 * time.Millisecond,
	})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got: %v", err)
	}
}

func TestDispatchValidation(t *testing.T) {
	uc := newUseCase(t, &fakeConfirmationResolver{}, &fakeRepo{})

	cases := []struct {
		name string
		req  DispatchRequest
		want error
	}{
		{name: "invalid agent id", req: DispatchRequest{AgentID: "nope", Tool: WorkflowDiagnoseTool, Payload: testPayload()}, want: ErrInvalidAgentID},
		{name: "missing tool", req: DispatchRequest{AgentID: uuid.New().String(), Payload: testPayload()}, want: ErrToolRequired},
		{name: "missing payload", req: DispatchRequest{AgentID: uuid.New().String(), Tool: WorkflowDiagnoseTool}, want: ErrPayloadRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.Dispatch(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got: %v", tc.want, err)
			}
		})
	}
}

func TestDispatchCreateErrorPropagates(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("repo down")}
	uc := newUseCase(t, &fakeConfirmationResolver{}, repo)
	_, err := uc.Dispatch(context.Background(), DispatchRequest{
		AgentID: uuid.New().String(),
		Tool:    WorkflowDiagnoseTool,
		Payload: testPayload(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkflowToolConstants(t *testing.T) {
	if WorkflowDiagnoseTool != "workflow.diagnose" || WorkflowDeployTool != "workflow.deploy" {
		t.Fatalf("unexpected workflow tool constants: %q %q", WorkflowDiagnoseTool, WorkflowDeployTool)
	}
}
