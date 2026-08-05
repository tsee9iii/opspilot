package http

type RegisterAgentRequest struct {
	Secret  string    `json:"secret"`
	Version string    `json:"version"`
	Server  ServerDTO `json:"server"`
}

type ServerDTO struct {
	Hostname    string `json:"hostname"`
	Environment string `json:"environment"`
}
