package http

import "encoding/json"

type CreateCommandRequest struct {
	AgentID string          `json:"agent_id"`
	Tool    string          `json:"tool"`
	Payload json.RawMessage `json:"payload"`
}
