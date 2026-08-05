// Package http provides the central HTTP router.
//
// It owns route registration only. Endpoint handlers are wired by the
// application bootstrap; this package contains no business logic.
package http

import "net/http"

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("POST /api/v1/agents/register", handleRegisterAgent)
	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
