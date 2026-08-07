package tools

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	apphealth "github.com/tsee9iii/opspilot/internal/application/health"
	"github.com/tsee9iii/opspilot/internal/application/inventory"
)

type fakeServerRepo struct {
	servers []inventory.ServerSummary
	err     error
}

func (f *fakeServerRepo) ListServers(context.Context) ([]inventory.ServerSummary, error) {
	return f.servers, f.err
}

type fakeAgentRepo struct {
	agents []inventory.AgentSummary
	err    error
	got    inventory.ListAgentsRequest
}

func (f *fakeAgentRepo) ListAgents(_ context.Context, req inventory.ListAgentsRequest) ([]inventory.AgentSummary, error) {
	f.got = req
	return f.agents, f.err
}

type fakeCommandRepo struct {
	commands []inventory.CommandSummary
	err      error
	got      inventory.ListCommandsRequest
}

func (f *fakeCommandRepo) ListCommands(_ context.Context, req inventory.ListCommandsRequest) ([]inventory.CommandSummary, error) {
	f.got = req
	return f.commands, f.err
}

// fakeHealthRepo fakes the health read repository for tool tests.
type fakeHealthRepo struct {
	summaries []apphealth.Summary
	signals   []apphealth.Signal
	summary   apphealth.Summary
	err       error
}

func (f *fakeHealthRepo) ListHealth(context.Context) ([]apphealth.Summary, error) {
	return f.summaries, f.err
}

func (f *fakeHealthRepo) GetHealthByAgentID(context.Context, string) (apphealth.Summary, error) {
	return f.summary, f.err
}

func (f *fakeHealthRepo) ListHealthSignals(context.Context) ([]apphealth.Signal, error) {
	return f.signals, f.err
}

// fakeAlertRepo fakes the alert read repository for tool tests.
type fakeAlertRepo struct {
	alerts []appalert.Alert
	alert  appalert.Alert
	err    error
}

func (f *fakeAlertRepo) List(context.Context, string, string, string, string, int) ([]appalert.Alert, error) {
	return f.alerts, f.err
}

func (f *fakeAlertRepo) GetByID(context.Context, string) (appalert.Alert, error) {
	return f.alert, f.err
}

func (f *fakeAlertRepo) Acknowledge(context.Context, string, string) (appalert.Alert, error) {
	return appalert.Alert{}, errors.New("acknowledge is never exposed through MCP tools")
}

// dispatchRepo fakes the command Repository so the real dispatch use case can
// be exercised end to end.
type dispatchRepo struct {
	createRes appcommand.CreateCommandResponse
	createErr error
	getRes    appcommand.GetCommandResponse
	getErr    error
	created   []appcommand.CreateCommandRequest
}

func (f *dispatchRepo) CreateCommand(_ context.Context, req appcommand.CreateCommandRequest) (appcommand.CreateCommandResponse, error) {
	f.created = append(f.created, req)
	if f.createErr != nil {
		return appcommand.CreateCommandResponse{}, f.createErr
	}
	return f.createRes, nil
}

func (f *dispatchRepo) GetCommand(context.Context, appcommand.GetCommandRequest) (appcommand.GetCommandResponse, error) {
	return f.getRes, f.getErr
}

func (f *dispatchRepo) LeaseNextCommand(context.Context, appcommand.LeaseCommandRequest) (appcommand.LeaseCommandResponse, error) {
	return appcommand.LeaseCommandResponse{}, errors.New("unused")
}

func (f *dispatchRepo) StartCommand(context.Context, appcommand.StartCommandRequest) (appcommand.StartCommandResponse, error) {
	return appcommand.StartCommandResponse{}, errors.New("unused")
}

func (f *dispatchRepo) CompleteCommand(context.Context, appcommand.CompleteCommandRequest) (appcommand.CompleteCommandResponse, error) {
	return appcommand.CompleteCommandResponse{}, errors.New("unused")
}

func (f *dispatchRepo) FailCommand(context.Context, appcommand.FailCommandRequest) (appcommand.FailCommandResponse, error) {
	return appcommand.FailCommandResponse{}, errors.New("unused")
}

func (f *dispatchRepo) ApproveCommand(context.Context, appcommand.ApproveCommandRequest) (appcommand.ApproveCommandResponse, error) {
	return appcommand.ApproveCommandResponse{}, errors.New("unused")
}

type fakeConfirmationResolver struct {
	level string
}

func (f *fakeConfirmationResolver) ConfirmationLevel(context.Context, uuid.UUID, string) (string, error) {
	level := f.level
	if level == "" {
		// Real resolvers fail closed; tests default to a known available
		// capability that needs no confirmation.
		level = "none"
	}
	return level, nil
}

// newDispatch builds a real DispatchUseCase backed by fakes.
func newDispatch(repo *dispatchRepo) *dispatch.DispatchUseCase {
	create := appcommand.NewCreateUseCase(repo, &fakeConfirmationResolver{})
	get := appcommand.NewGetCommandUseCase(repo)
	return dispatch.NewDispatchUseCase(create, get)
}

func timePtr(t time.Time) *time.Time { return &t }
