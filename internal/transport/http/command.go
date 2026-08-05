package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/opspilot/opspilot/internal/application/command"
)

type CommandHandler struct {
	create *command.CreateUseCase
	lease  *command.LeaseUseCase
}

func NewCommandHandler(create *command.CreateUseCase, lease *command.LeaseUseCase) *CommandHandler {
	return &CommandHandler{create: create, lease: lease}
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
