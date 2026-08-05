package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/opspilot/opspilot/internal/application/agent"
)

type AgentHandler struct {
	register *agent.RegisterUseCase
}

func NewAgentHandler(register *agent.RegisterUseCase) *AgentHandler {
	return &AgentHandler{register: register}
}

func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	var reqDTO RegisterAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if err := validateRegisterAgent(reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	resp, err := h.register.Register(r.Context(), agent.RegisterAgentRequest{
		Secret:      reqDTO.Secret,
		Version:     reqDTO.Version,
		Hostname:    reqDTO.Server.Hostname,
		Environment: reqDTO.Server.Environment,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to register agent")
		return
	}

	writeJSON(w, http.StatusCreated, RegisterAgentResponse{
		AgentID: resp.AgentID,
		Status:  resp.Status,
	})
}

func validateRegisterAgent(req RegisterAgentRequest) error {
	switch {
	case req.Secret == "":
		return errors.New("secret is required")
	case req.Version == "":
		return errors.New("version is required")
	case req.Server.Hostname == "":
		return errors.New("server.hostname is required")
	case req.Server.Environment == "":
		return errors.New("server.environment is required")
	}
	return nil
}
