package http

type CreateCommandResponse struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
}
