package http

type SyncCapabilitiesRequest struct {
	AgentID      string          `json:"agent_id"`
	Secret       string          `json:"secret"`
	Capabilities []CapabilityDTO `json:"capabilities"`
}

type CapabilityDTO struct {
	ToolName    string `json:"tool_name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}
