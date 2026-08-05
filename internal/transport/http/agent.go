package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tsee9iii/opspilot/internal/application/agent"
)

type AgentHandler struct {
	register   *agent.RegisterUseCase
	heartbeat  *agent.HeartbeatUseCase
	unregister *agent.UnregisterUseCase
}

func NewAgentHandler(register *agent.RegisterUseCase, heartbeat *agent.HeartbeatUseCase, unregister *agent.UnregisterUseCase) *AgentHandler {
	return &AgentHandler{
		register:   register,
		heartbeat:  heartbeat,
		unregister: unregister,
	}
}

func (h *AgentHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var reqDTO HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if err := validateHeartbeat(reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	resp, err := h.heartbeat.Heartbeat(r.Context(), agent.HeartbeatRequest{
		AgentID: reqDTO.AgentID,
		Secret:  reqDTO.Secret,
	})
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrAgentNotFound),
			errors.Is(err, agent.ErrAgentSecretMismatch),
			errors.Is(err, agent.ErrAgentUnregistered):
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid agent credentials")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to process heartbeat")
		}
		return
	}

	writeJSON(w, http.StatusOK, HeartbeatResponse{
		Status:        "ok",
		NextHeartbeat: int(resp.NextHeartbeat.Seconds()),
	})
}

func validateHeartbeat(req HeartbeatRequest) error {
	switch {
	case req.AgentID == "":
		return errors.New("agent_id is required")
	case req.Secret == "":
		return errors.New("secret is required")
	}
	return nil
}

func (h *AgentHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	var reqDTO UnregisterAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if err := validateUnregisterAgent(reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	resp, err := h.unregister.Unregister(r.Context(), agent.UnregisterRequest{
		AgentID: reqDTO.AgentID,
		Secret:  reqDTO.Secret,
	})
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrAgentSecretMismatch):
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid agent credentials")
		case errors.Is(err, agent.ErrAgentNotFound):
			writeError(w, http.StatusNotFound, "agent_not_found", "agent not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to unregister agent")
		}
		return
	}

	writeJSON(w, http.StatusOK, UnregisterAgentResponse{Status: resp.Status})
}

func validateUnregisterAgent(req UnregisterAgentRequest) error {
	switch {
	case req.AgentID == "":
		return errors.New("agent_id is required")
	case req.Secret == "":
		return errors.New("secret is required")
	}
	return nil
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
		RegistrationToken: reqDTO.RegistrationToken,
		Secret:            reqDTO.Secret,
		Version:           reqDTO.Version,
		Hostname:          reqDTO.Server.Hostname,
		Environment:       reqDTO.Server.Environment,
	})
	if err != nil {
		switch {
		case errors.Is(err, agent.ErrTokenNotFound),
			errors.Is(err, agent.ErrTokenExpired),
			errors.Is(err, agent.ErrTokenRevoked):
			writeError(w, http.StatusUnauthorized, "invalid_token", err.Error())
		case errors.Is(err, agent.ErrTokenUsed):
			writeError(w, http.StatusConflict, "token_already_used", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to register agent")
		}
		return
	}

	writeJSON(w, http.StatusCreated, RegisterAgentResponse{
		AgentID: resp.AgentID,
		Status:  resp.Status,
	})
}

func validateRegisterAgent(req RegisterAgentRequest) error {
	switch {
	case req.RegistrationToken == "":
		return errors.New("registration_token is required")
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
