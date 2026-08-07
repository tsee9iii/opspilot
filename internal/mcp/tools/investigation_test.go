package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
	"github.com/tsee9iii/opspilot/internal/application/dispatch"
	"github.com/tsee9iii/opspilot/internal/mcp"
)

// errConfirmationResolver returns a fixed error from ConfirmationLevel, letting
// tests exercise the fail-closed capability path through the real dispatch and
// create use cases.
type errConfirmationResolver struct{ err error }

func (f errConfirmationResolver) ConfirmationLevel(context.Context, uuid.UUID, string) (string, error) {
	return "", f.err
}

// newDispatchResolver builds a DispatchUseCase with an explicit confirmation
// resolver.
func newDispatchResolver(repo *dispatchRepo, resolver appcommand.ConfirmationResolver) *dispatch.DispatchUseCase {
	create := appcommand.NewCreateUseCase(repo, resolver)
	get := appcommand.NewGetCommandUseCase(repo)
	return dispatch.NewDispatchUseCase(create, get)
}

// wantToolError asserts err is a *mcp.ToolError with the given code.
func wantToolError(t *testing.T, err error, code string) {
	t.Helper()
	var te *mcp.ToolError
	if !errors.As(err, &te) || te.Code != code {
		t.Fatalf("expected tool error %q, got: %v", code, err)
	}
}

// assertInvestigationDefinition pins the stable metadata of a read-only
// investigation tool: name, category, an investigation-only description, and an
// input schema that requires the listed keys.
func assertInvestigationDefinition(t *testing.T, tool mcp.Tool, name string, required ...string) {
	t.Helper()
	if tool.Name() != name {
		t.Fatalf("name = %q, want %q", tool.Name(), name)
	}
	if tool.Category() != CategoryInvestigation {
		t.Fatalf("%s category = %q, want investigation", name, tool.Category())
	}
	if tool.Description() == "" || !strings.Contains(tool.Description(), "Investigation only") {
		t.Fatalf("%s description must state it is investigation-only: %q", name, tool.Description())
	}
	if len(tool.InputSchema()) == 0 || len(tool.OutputSchema()) == 0 {
		t.Fatalf("%s missing input/output schema", name)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatalf("%s invalid input schema: %v", name, err)
	}
	for _, key := range required {
		if !contains(schema.Required, key) {
			t.Fatalf("%s input schema must require %s", name, key)
		}
	}
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// investigationCompletedRepo seeds a dispatch repo whose next command reaches a
// completed state carrying result.
func investigationCompletedRepo(commandID uuid.UUID, result []byte) *dispatchRepo {
	return &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusCompleted,
			Result:             result,
		},
	}
}

// investigationFailedRepo seeds a dispatch repo whose next command fails with
// the given error.
func investigationFailedRepo(commandID uuid.UUID, errMsg string) *dispatchRepo {
	return &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes: appcommand.GetCommandResponse{
			ID:                 commandID,
			ConfirmationStatus: appcommand.ConfirmationApproved,
			Status:             appcommand.StatusFailed,
			Error:              errMsg,
		},
	}
}

// investigationPendingApprovalRepo seeds a dispatch repo whose next command
// awaits operator approval.
func investigationPendingApprovalRepo(commandID uuid.UUID) *dispatchRepo {
	return &dispatchRepo{
		createRes: appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending},
		getRes:    appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationPending},
	}
}

// createdResponse is a create outcome for a command that stays pending.
func createdResponse(commandID uuid.UUID) appcommand.CreateCommandResponse {
	return appcommand.CreateCommandResponse{CommandID: commandID.String(), Status: appcommand.StatusPending}
}

// pendingCommand is a read outcome for a command that never reaches a terminal
// state (used to exercise the dispatch timeout).
func pendingCommand(commandID uuid.UUID) appcommand.GetCommandResponse {
	return appcommand.GetCommandResponse{ID: commandID, ConfirmationStatus: appcommand.ConfirmationApproved, Status: appcommand.StatusPending}
}

// assertInvestigationResult decodes an investigation envelope and asserts its
// status plus the semantically preserved raw result (JSON key order and
// rune-escapes such as \u003e are irrelevant).
func assertInvestigationResult(t *testing.T, out []byte, wantStatus string, wantResult []byte) {
	t.Helper()
	var got investigationOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode investigation result: %v", err)
	}
	if got.Status != wantStatus {
		t.Fatalf("status = %q, want %q", got.Status, wantStatus)
	}
	if wantResult != nil && !jsonEquivalent(got.Result, wantResult) {
		t.Fatalf("result = %s, want %s", got.Result, wantResult)
	}
	if wantStatus == "awaiting_approval" && got.Message == "" {
		t.Fatalf("awaiting approval result must carry a message: %+v", got)
	}
	if wantStatus == "failed" && got.Error == "" {
		t.Fatalf("failed result must carry an error: %+v", got)
	}
}

// jsonEquivalent reports whether two JSON documents decode to the same value.
func jsonEquivalent(a, b []byte) bool {
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return jsonEqual(av, bv)
}

// jsonEqual deep-compares two decoded JSON values; arrays are order-sensitive.
func jsonEqual(a, b any) bool {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		if len(am) != len(bm) {
			return false
		}
		for k, av := range am {
			bv, ok := bm[k]
			if !ok || !jsonEqual(av, bv) {
				return false
			}
		}
		return true
	}
	as, aok := a.([]any)
	bs, bok := b.([]any)
	if aok && bok {
		if len(as) != len(bs) {
			return false
		}
		for i := range as {
			if !jsonEqual(as[i], bs[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}
