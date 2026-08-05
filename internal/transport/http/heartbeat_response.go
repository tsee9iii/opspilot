package http

type HeartbeatResponse struct {
	Status        string `json:"status"`
	NextHeartbeat int    `json:"next_heartbeat"`
}
