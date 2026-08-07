// Package dispatch sends workflow commands to agents through the existing
// command pipeline and waits for a terminal state. It contains no workflow
// logic of its own: the actual diagnose/deploy workflows run on the agent; this
// package only orchestrates create -> wait, reusing the command use cases.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	appcommand "github.com/tsee9iii/opspilot/internal/application/command"
)

// Workflow tool names as registered by the agent. These are the wire contract
// between central and agent.
const (
	WorkflowDiagnoseTool = "workflow.diagnose"
	WorkflowDeployTool   = "workflow.deploy"
	FileReadTool         = "file.read"
	FilesystemListTool   = "filesystem.list"
	DockerInspectTool    = "docker.inspect"
	PM2ListTool          = "pm2.list"
	PM2LogsTool          = "pm2.logs"
	DockerPsTool         = "docker.ps"
	DockerLogsTool       = "docker.logs"
	JournalLogsTool      = "journal.logs"
	GitStatusTool        = "git.status"
	GitCurrentCommitTool = "git.current_commit"
	GitBranchTool        = "git.branch"
)

const (
	// defaultTimeout bounds how long a dispatched command may take to reach a
	// terminal state before the call fails.
	defaultTimeout = 5 * time.Minute
	// defaultPollInterval is how often a dispatched command is re-read.
	defaultPollInterval = 500 * time.Millisecond
)

var (
	ErrInvalidAgentID  = errors.New("invalid agent id")
	ErrToolRequired    = errors.New("tool is required")
	ErrPayloadRequired = errors.New("payload is required")
	// ErrTimeout is returned when a dispatched command does not reach a
	// terminal state before the timeout elapses.
	ErrTimeout = errors.New("command did not complete before timeout")
)

// DispatchRequest asks for a workflow tool to be dispatched to an agent.
type DispatchRequest struct {
	AgentID string
	Tool    string
	Payload []byte
	Timeout time.Duration
}

// DispatchResponse describes the outcome of a dispatched command. Status is
// one of "awaiting_approval", "completed" or "failed". Result carries the raw
// tool output JSON exactly as stored when the command completed.
type DispatchResponse struct {
	CommandID        string
	Status           string
	AwaitingApproval bool
	Result           json.RawMessage
	Error            string
}

// DispatchUseCase creates a command through the command use cases and waits for
// its terminal state. Approval is never bypassed: a command requiring
// confirmation returns immediately as awaiting_approval.
type DispatchUseCase struct {
	create *appcommand.CreateUseCase
	get    *appcommand.GetCommandUseCase
	// PollInterval overrides the re-read interval; zero uses the default.
	// Exposed for tests.
	PollInterval time.Duration
}

func NewDispatchUseCase(create *appcommand.CreateUseCase, get *appcommand.GetCommandUseCase) *DispatchUseCase {
	return &DispatchUseCase{create: create, get: get}
}

// Dispatch sends the workflow command and waits for a terminal state.
func (uc *DispatchUseCase) Dispatch(ctx context.Context, req DispatchRequest) (DispatchResponse, error) {
	if _, err := uuid.Parse(req.AgentID); err != nil {
		return DispatchResponse{}, ErrInvalidAgentID
	}
	if req.Tool == "" {
		return DispatchResponse{}, ErrToolRequired
	}
	if len(req.Payload) == 0 {
		return DispatchResponse{}, ErrPayloadRequired
	}
	if req.Timeout <= 0 {
		req.Timeout = defaultTimeout
	}

	created, err := uc.create.Create(ctx, appcommand.CreateCommandRequest{
		AgentID:     req.AgentID,
		Tool:        req.Tool,
		Payload:     req.Payload,
		Source:      appcommand.SourceMCP,
		RequestedBy: "hermes",
	})
	if err != nil {
		return DispatchResponse{}, fmt.Errorf("dispatch: create command: %w", err)
	}
	return uc.wait(ctx, created.CommandID, req.Timeout)
}

func (uc *DispatchUseCase) wait(ctx context.Context, commandID string, timeout time.Duration) (DispatchResponse, error) {
	deadline := time.Now().Add(timeout)
	interval := uc.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	for {
		cmd, err := uc.get.Get(ctx, appcommand.GetCommandRequest{CommandID: commandID})
		if err != nil {
			return DispatchResponse{}, fmt.Errorf("dispatch: read command: %w", err)
		}

		if cmd.ConfirmationStatus == appcommand.ConfirmationPending {
			return DispatchResponse{CommandID: commandID, Status: "awaiting_approval", AwaitingApproval: true}, nil
		}

		switch cmd.Status {
		case appcommand.StatusCompleted:
			return DispatchResponse{CommandID: commandID, Status: "completed", Result: json.RawMessage(cmd.Result)}, nil
		case appcommand.StatusFailed:
			return DispatchResponse{CommandID: commandID, Status: "failed", Error: cmd.Error}, nil
		}

		if time.Now().After(deadline) {
			return DispatchResponse{CommandID: commandID, Status: "pending"}, ErrTimeout
		}
		select {
		case <-ctx.Done():
			return DispatchResponse{CommandID: commandID, Status: "pending"}, ctx.Err()
		case <-time.After(interval):
		}
	}
}
