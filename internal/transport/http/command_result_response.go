package http

import "encoding/json"

// CommandResultResponse is the current state and final result of a command.
// Parameters and Result are opaque JSON returned exactly as stored.
type CommandResultResponse struct {
	ID                 string          `json:"id"`
	AgentID            string          `json:"agent_id"`
	Status             string          `json:"status"`
	ConfirmationStatus string          `json:"confirmation_status"`
	Tool               string          `json:"tool"`
	Parameters         json.RawMessage `json:"parameters"`
	Result             json.RawMessage `json:"result,omitempty"`
	Error              string          `json:"error,omitempty"`
	CreatedAt          string          `json:"created_at"`
	LeasedAt           *string         `json:"leased_at,omitempty"`
	CompletedAt        *string         `json:"completed_at,omitempty"`
}
