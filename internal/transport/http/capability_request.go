package http

import "encoding/json"

type SyncCapabilitiesRequest struct {
	AgentID      string          `json:"agent_id"`
	Secret       string          `json:"secret"`
	Capabilities []CapabilityDTO `json:"capabilities"`
}

type CapabilityDTO struct {
	ToolName          string          `json:"tool_name"`
	Version           string          `json:"version"`
	Description       string          `json:"description"`
	ParameterSchema   json.RawMessage `json:"parameter_schema"`
	Confirmation      string          `json:"confirmation_level"`
	Available         bool            `json:"available"`
	UnavailableReason string          `json:"unavailable_reason"`
}
