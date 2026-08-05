package http

import "encoding/json"

type StartCommandRequest struct {
	AgentID   string `json:"agent_id"`
	CommandID string `json:"command_id"`
}

type CompleteCommandRequest struct {
	AgentID   string          `json:"agent_id"`
	CommandID string          `json:"command_id"`
	Result    json.RawMessage `json:"result"`
}

type FailCommandRequest struct {
	AgentID   string `json:"agent_id"`
	CommandID string `json:"command_id"`
	Error     string `json:"error"`
}
