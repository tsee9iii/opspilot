package http

type RegisterAgentResponse struct {
	AgentID    string `json:"agent_id"`
	Status     string `json:"status"`
	SigningKey string `json:"signing_key"`
}
