package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	ErrInvalidAgentID  = errors.New("invalid agent id")
	ErrToolRequired    = errors.New("tool is required")
	ErrPayloadRequired = errors.New("payload is required")
	ErrResultRequired  = errors.New("result is required")
	ErrErrorRequired   = errors.New("error is required")
	// ErrCapabilityNotFound rejects a command whose target agent has not
	// advertised the requested tool. Unknown tools are never silently treated
	// as approved.
	ErrCapabilityNotFound = errors.New("agent has not advertised this tool")
	// ErrCapabilityUnavailable rejects a command whose target agent advertised
	// the tool but reported it as currently unavailable.
	ErrCapabilityUnavailable = errors.New("tool is not currently available on the agent")
)

// Command origin sources. The source is immutable audit data: it records which
// pathway created the command. 'mcp' commands are always created pending
// confirmation and never approved by the MCP path.
const (
	SourceAPI    = "api"
	SourceMCP    = "mcp"
	SourceSystem = "system"
)

type CreateCommandRequest struct {
	AgentID            string
	Tool               string
	Payload            []byte
	ConfirmationStatus string
	// Source is the immutable origin of the command: SourceAPI, SourceMCP or
	// SourceSystem. An empty source defaults to SourceAPI.
	Source string
	// RequestedBy is the actor identifier that requested the command. For MCP
	// commands this is the integration identity; for API commands it is the
	// authenticated operator actor.
	RequestedBy string
}

type CreateCommandResponse struct {
	CommandID string
	Status    string
}

// ConfirmationResolver resolves a tool's confirmation level from the agent's
// registered capabilities.
type ConfirmationResolver interface {
	ConfirmationLevel(ctx context.Context, agentID uuid.UUID, toolName string) (string, error)
}

// Notifier signals a target agent that a leasable command may be available so
// it can immediately call the authenticated lease endpoint. Implementations
// must be non-blocking and safe for concurrent use; delivery is best-effort.
// The notifier is optional: nil (or no notifier) disables wake-ups and the
// agent falls back to polling.
type Notifier interface {
	Notify(agentID string)
}

// Repository defines the persistence contract required by the command use cases.
type Repository interface {
	CreateCommand(ctx context.Context, req CreateCommandRequest) (CreateCommandResponse, error)
	LeaseNextCommand(ctx context.Context, req LeaseCommandRequest) (LeaseCommandResponse, error)
	StartCommand(ctx context.Context, req StartCommandRequest) (StartCommandResponse, error)
	CompleteCommand(ctx context.Context, req CompleteCommandRequest) (CompleteCommandResponse, error)
	FailCommand(ctx context.Context, req FailCommandRequest) (FailCommandResponse, error)
	ApproveCommand(ctx context.Context, req ApproveCommandRequest) (ApproveCommandResponse, error)
	GetCommand(ctx context.Context, req GetCommandRequest) (GetCommandResponse, error)
}

type CreateUseCase struct {
	repo     Repository
	confirm  ConfirmationResolver
	notifier Notifier
}

// NewCreateUseCase builds the creation use case. A notifier may be passed to
// wake the target agent once a leasable command is persisted; it is optional.
func NewCreateUseCase(repo Repository, confirm ConfirmationResolver, notifiers ...Notifier) *CreateUseCase {
	uc := &CreateUseCase{repo: repo, confirm: confirm}
	if len(notifiers) > 0 {
		uc.notifier = notifiers[0]
	}
	return uc
}

func (uc *CreateUseCase) Create(ctx context.Context, req CreateCommandRequest) (CreateCommandResponse, error) {
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		return CreateCommandResponse{}, ErrInvalidAgentID
	}
	if req.Tool == "" {
		return CreateCommandResponse{}, ErrToolRequired
	}
	if len(req.Payload) == 0 {
		return CreateCommandResponse{}, ErrPayloadRequired
	}

	level, err := uc.confirm.ConfirmationLevel(ctx, agentID, req.Tool)
	if err != nil {
		return CreateCommandResponse{}, fmt.Errorf("create command: resolve confirmation: %w", err)
	}
	if level == "" {
		// Fail closed: a resolver that returns an empty level (e.g. a missing
		// capability row) must never default the command to approved.
		return CreateCommandResponse{}, fmt.Errorf("create command: resolve confirmation: %w", ErrCapabilityNotFound)
	}

	req.ConfirmationStatus = ConfirmationApproved
	if level == ConfirmationRequiredLevel {
		req.ConfirmationStatus = ConfirmationPending
	}
	if req.Source == SourceMCP {
		// Fail closed: a command created by the Hermes integration is never
		// auto-approved, regardless of the tool's capability metadata. Only an
		// independent operator approval can transition it.
		req.ConfirmationStatus = ConfirmationPending
	}
	if req.Source == "" {
		req.Source = SourceAPI
	}
	resp, err := uc.repo.CreateCommand(ctx, req)
	if err != nil {
		return CreateCommandResponse{}, err
	}
	// Wake the agent only after the command is durably persisted AND actually
	// leasable now. Commands awaiting operator approval are never announced at
	// creation; the operator's later approval releases them.
	if req.ConfirmationStatus == ConfirmationApproved && uc.notifier != nil {
		uc.notifier.Notify(agentID.String())
	}
	return resp, nil
}
