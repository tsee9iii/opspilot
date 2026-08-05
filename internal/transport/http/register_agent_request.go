package http

type RegisterAgentRequest struct {
	RegistrationToken string    `json:"registration_token"`
	Secret            string    `json:"secret"`
	Version           string    `json:"version"`
	Server            ServerDTO `json:"server"`
}

type ServerDTO struct {
	Hostname    string `json:"hostname"`
	Environment string `json:"environment"`
}
