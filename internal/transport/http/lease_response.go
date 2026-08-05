package http

import "encoding/json"

type LeaseCommandResponse struct {
	CommandID string          `json:"command_id"`
	Tool      string          `json:"tool"`
	Payload   json.RawMessage `json:"payload"`
}
