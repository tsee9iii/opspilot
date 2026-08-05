package http

import (
	"encoding/json"
	"errors"
	"net/http"
)

func handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req RegisterAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if err := validateRegisterAgent(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	writeError(w, http.StatusNotImplemented, "not_implemented", "agent registration is not implemented yet")
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
