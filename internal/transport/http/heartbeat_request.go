package http

type HeartbeatRequest struct {
	AgentID string `json:"agent_id"`
	Secret  string `json:"secret"`
}
