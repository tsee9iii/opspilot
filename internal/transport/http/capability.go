package http

import (
	"encoding/json"
	"errors"
	"net/http"

	appagent "github.com/opspilot/opspilot/internal/application/agent"
	"github.com/opspilot/opspilot/internal/application/capability"
)

type CapabilityHandler struct {
	sync *capability.SyncUseCase
}

func NewCapabilityHandler(sync *capability.SyncUseCase) *CapabilityHandler {
	return &CapabilityHandler{sync: sync}
}

func (h *CapabilityHandler) Sync(w http.ResponseWriter, r *http.Request) {
	var reqDTO SyncCapabilitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if err := validateSyncCapabilities(reqDTO); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	req := capability.SyncRequest{
		AgentID:      reqDTO.AgentID,
		Secret:       reqDTO.Secret,
		Capabilities: make([]capability.Capability, 0, len(reqDTO.Capabilities)),
	}
	for _, c := range reqDTO.Capabilities {
		req.Capabilities = append(req.Capabilities, capability.Capability{
			ToolName:        c.ToolName,
			Version:         c.Version,
			Description:     c.Description,
			ParameterSchema: c.ParameterSchema,
			Confirmation:    c.Confirmation,
		})
	}

	resp, err := h.sync.Sync(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, capability.ErrInvalidAgentID),
			errors.Is(err, capability.ErrCapabilitiesRequired):
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		case errors.Is(err, appagent.ErrAgentNotFound),
			errors.Is(err, appagent.ErrAgentSecretMismatch):
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid agent credentials")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to sync capabilities")
		}
		return
	}

	writeJSON(w, http.StatusOK, SyncCapabilitiesResponse{Status: "ok", Count: resp.Count})
}

func validateSyncCapabilities(req SyncCapabilitiesRequest) error {
	switch {
	case req.AgentID == "":
		return errors.New("agent_id is required")
	case req.Secret == "":
		return errors.New("secret is required")
	case len(req.Capabilities) == 0:
		return errors.New("capabilities is required")
	}
	for _, c := range req.Capabilities {
		switch {
		case c.ToolName == "":
			return errors.New("capability tool_name is required")
		case c.Version == "":
			return errors.New("capability version is required")
		case c.Description == "":
			return errors.New("capability description is required")
		case len(c.ParameterSchema) == 0:
			return errors.New("capability parameter_schema is required")
		case c.Confirmation != "none" && c.Confirmation != "required":
			return errors.New("capability confirmation_level must be one of: none, required")
		}
	}
	return nil
}
