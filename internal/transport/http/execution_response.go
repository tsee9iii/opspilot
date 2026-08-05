package http

type StartCommandResponse struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
}

type CompleteCommandResponse struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
}

type FailCommandResponse struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
}
