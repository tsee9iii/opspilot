package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tsee9iii/opspilot/internal/application/command"
)

type CommandHandler struct {
	create    *command.CreateUseCase
	lease     *command.LeaseUseCase
	execution *command.ExecutionUseCase
	approval  *command.ApprovalUseCase
}

func NewCommandHandler(create *command.CreateUseCase, lease *command.LeaseUseCase, execution *command.ExecutionUseCase, approval *command.ApprovalUseCase) *CommandHandler {
	return &CommandHandler{create: create, lease: lease, execution: execution, approval: approval}
}

func (h *CommandHandler) Create(w http.ResponseWriter, r *http.Request) {
	var reqDTO CreateCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if err := validateCreateCommand(reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	resp, err := h.create.Create(r.Context(), command.CreateCommandRequest{
		AgentID: reqDTO.AgentID,
		Tool:    reqDTO.Tool,
		Payload: reqDTO.Payload,
	})
	if err != nil {
		if errors.Is(err, command.ErrInvalidAgentID) {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create command")
		return
	}

	writeJSON(w, http.StatusCreated, CreateCommandResponse{
		CommandID: resp.CommandID,
		Status:    resp.Status,
	})
}

func (h *CommandHandler) Lease(w http.ResponseWriter, r *http.Request) {
	var reqDTO LeaseCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if reqDTO.AgentID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "agent_id is required")
		return
	}

	resp, err := h.lease.Lease(r.Context(), command.LeaseCommandRequest{AgentID: reqDTO.AgentID})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrNoPendingCommands):
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, command.ErrInvalidAgentID):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to lease command")
		}
		return
	}

	writeJSON(w, http.StatusOK, LeaseCommandResponse{
		CommandID: resp.CommandID,
		Tool:      resp.Tool,
		Payload:   resp.Payload,
	})
}

func (h *CommandHandler) Start(w http.ResponseWriter, r *http.Request) {
	var reqDTO StartCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if err := validateCommandRef(reqDTO.AgentID, reqDTO.CommandID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	resp, err := h.execution.Start(r.Context(), command.StartCommandRequest{
		AgentID:   reqDTO.AgentID,
		CommandID: reqDTO.CommandID,
	})
	if err != nil {
		writeExecutionError(w, err, "start")
		return
	}
	writeJSON(w, http.StatusOK, StartCommandResponse{CommandID: resp.CommandID, Status: resp.Status})
}

func (h *CommandHandler) Complete(w http.ResponseWriter, r *http.Request) {
	var reqDTO CompleteCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if err := validateCommandRef(reqDTO.AgentID, reqDTO.CommandID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if len(reqDTO.Result) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "result is required")
		return
	}

	resp, err := h.execution.Complete(r.Context(), command.CompleteCommandRequest{
		AgentID:   reqDTO.AgentID,
		CommandID: reqDTO.CommandID,
		Result:    reqDTO.Result,
	})
	if err != nil {
		writeExecutionError(w, err, "complete")
		return
	}
	writeJSON(w, http.StatusOK, CompleteCommandResponse{CommandID: resp.CommandID, Status: resp.Status})
}

func (h *CommandHandler) Fail(w http.ResponseWriter, r *http.Request) {
	var reqDTO FailCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if err := validateCommandRef(reqDTO.AgentID, reqDTO.CommandID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if reqDTO.Error == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "error is required")
		return
	}

	resp, err := h.execution.Fail(r.Context(), command.FailCommandRequest{
		AgentID:   reqDTO.AgentID,
		CommandID: reqDTO.CommandID,
		Error:     reqDTO.Error,
	})
	if err != nil {
		writeExecutionError(w, err, "fail")
		return
	}
	writeJSON(w, http.StatusOK, FailCommandResponse{CommandID: resp.CommandID, Status: resp.Status})
}

func (h *CommandHandler) Approve(w http.ResponseWriter, r *http.Request) {
	var reqDTO ApproveCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if reqDTO.CommandID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "command_id is required")
		return
	}

	resp, err := h.approval.Approve(r.Context(), command.ApproveCommandRequest{CommandID: reqDTO.CommandID})
	if err != nil {
		switch {
		case errors.Is(err, command.ErrInvalidCommandID):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		case errors.Is(err, command.ErrCommandNotFound):
			writeError(w, http.StatusNotFound, "command_not_found", "command not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to approve command")
		}
		return
	}
	writeJSON(w, http.StatusOK, ApproveCommandResponse{Status: resp.Status})
}

func writeExecutionError(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, command.ErrInvalidAgentID), errors.Is(err, command.ErrInvalidCommandID):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, command.ErrCommandNotFound):
		writeError(w, http.StatusNotFound, "not_found", "command not found")
	case errors.Is(err, command.ErrCommandNotOwned):
		writeError(w, http.StatusForbidden, "command_not_owned", "command is not owned by agent")
	case errors.Is(err, command.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to "+action+" command")
	}
}

func validateCommandRef(agentID, commandID string) error {
	switch {
	case agentID == "":
		return errors.New("agent_id is required")
	case commandID == "":
		return errors.New("command_id is required")
	}
	return nil
}

func validateCreateCommand(req CreateCommandRequest) error {
	switch {
	case req.AgentID == "":
		return errors.New("agent_id is required")
	case req.Tool == "":
		return errors.New("tool is required")
	case len(req.Payload) == 0:
		return errors.New("payload is required")
	}
	return nil
}
