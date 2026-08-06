// Package http provides the central HTTP router.
//
// It owns route registration and middleware boundaries only. Endpoint handlers
// are wired by the application bootstrap; this package contains no business
// logic.
package http

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agentsign"
)

// RouterDeps carries the middleware dependencies the router needs to enforce
// its authentication boundaries.
type RouterDeps struct {
	// Agents resolves an agent's signing key for HMAC request verification.
	Agents AgentStore
	// OperatorToken is the bearer token operator endpoints require.
	OperatorToken string
	// Logger is used by the panic recovery middleware.
	Logger *zap.Logger
	// TimestampWindow bounds the freshness of signed agent requests. Zero uses
	// the protocol default.
	TimestampWindow time.Duration
}

func NewRouter(deps RouterDeps, agents *AgentHandler, commands *CommandHandler, capabilities *CapabilityHandler) http.Handler {
	window := deps.TimestampWindow
	if window <= 0 {
		window = agentsign.DefaultTimestampWindow
	}

	mux := http.NewServeMux()

	global := []func(http.Handler) http.Handler{
		Recovery(deps.Logger),
		MaxBodyBytes(maxRequestBodyBytes),
	}

	agentAuth := []func(http.Handler) http.Handler{
		Recovery(deps.Logger),
		MaxBodyBytes(maxRequestBodyBytes),
		AgentAuth(deps.Agents, window),
	}

	operatorAuth := []func(http.Handler) http.Handler{
		Recovery(deps.Logger),
		MaxBodyBytes(maxRequestBodyBytes),
		OperatorAuth(deps.OperatorToken),
	}

	mux.Handle("GET /healthz", chain(http.HandlerFunc(handleHealthz), global...))
	mux.Handle("POST /api/v1/agents/register", chain(http.HandlerFunc(agents.Register), global...))
	mux.Handle("POST /api/v1/agents/heartbeat", chain(http.HandlerFunc(agents.Heartbeat), agentAuth...))
	mux.Handle("POST /api/v1/agents/unregister", chain(http.HandlerFunc(agents.Unregister), agentAuth...))
	mux.Handle("POST /api/v1/commands", chain(http.HandlerFunc(commands.Create), global...))
	mux.Handle("GET /api/v1/commands/{id}", chain(http.HandlerFunc(commands.Get), operatorAuth...))
	mux.Handle("POST /api/v1/commands/lease", chain(http.HandlerFunc(commands.Lease), agentAuth...))
	mux.Handle("POST /api/v1/commands/start", chain(http.HandlerFunc(commands.Start), agentAuth...))
	mux.Handle("POST /api/v1/commands/complete", chain(http.HandlerFunc(commands.Complete), agentAuth...))
	mux.Handle("POST /api/v1/commands/fail", chain(http.HandlerFunc(commands.Fail), agentAuth...))
	mux.Handle("POST /api/v1/commands/approve", chain(http.HandlerFunc(commands.Approve), operatorAuth...))
	mux.Handle("POST /api/v1/capabilities", chain(http.HandlerFunc(capabilities.Sync), agentAuth...))
	return mux
}

// chain applies middlewares in order: the first middleware is the outermost.
func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
