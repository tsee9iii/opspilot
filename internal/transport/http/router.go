// Package http provides the central HTTP router.
//
// It owns route registration only. Endpoint handlers are wired by the
// application bootstrap; this package contains no business logic.
package http

import "net/http"

func NewRouter(agents *AgentHandler, commands *CommandHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("POST /api/v1/agents/register", agents.Register)
	mux.HandleFunc("POST /api/v1/agents/heartbeat", agents.Heartbeat)
	mux.HandleFunc("POST /api/v1/commands", commands.Create)
	mux.HandleFunc("POST /api/v1/commands/lease", commands.Lease)
	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
