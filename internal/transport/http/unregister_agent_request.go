package http

type UnregisterAgentRequest struct {
	AgentID string `json:"agent_id"`
	Secret  string `json:"secret"`
}
