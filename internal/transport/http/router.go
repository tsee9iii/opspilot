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
	// Events is the agent SSE wake-up handler. When nil, the events endpoint is
	// not registered (used by tests that do not exercise SSE).
	Events *AgentEventHandler
}

func NewRouter(deps RouterDeps, agents *AgentHandler, commands *CommandHandler, capabilities *CapabilityHandler, health *HealthHandler, alerts *AlertHandler) http.Handler {
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
		// Every operator action is attributed to an actor recorded on the
		// request. This runs after auth so it never grants access on its own.
		ActorIdentity(),
	}

	mux.Handle("GET /healthz", chain(http.HandlerFunc(handleHealthz), global...))
	mux.Handle("POST /api/v1/agents/register", chain(http.HandlerFunc(agents.Register), global...))
	mux.Handle("POST /api/v1/agents/heartbeat", chain(http.HandlerFunc(agents.Heartbeat), agentAuth...))
	mux.Handle("POST /api/v1/agents/health", chain(http.HandlerFunc(health.Report), agentAuth...))
	mux.Handle("POST /api/v1/agents/unregister", chain(http.HandlerFunc(agents.Unregister), agentAuth...))
	if deps.Events != nil {
		// Authenticated SSE wake-up stream. It is a notification-only channel:
		// no command payload, credentials, approvals or results ever travel on
		// it. The agent reacts to `event: wakeup` by calling the authenticated
		// lease endpoint, so PostgreSQL remains authoritative.
		mux.Handle("GET /api/v1/agents/events", chain(http.HandlerFunc(deps.Events.Events), agentAuth...))
	}
	mux.Handle("POST /api/v1/commands", chain(http.HandlerFunc(commands.Create), operatorAuth...))
	mux.Handle("GET /api/v1/commands/{id}", chain(http.HandlerFunc(commands.Get), operatorAuth...))
	mux.Handle("POST /api/v1/commands/lease", chain(http.HandlerFunc(commands.Lease), agentAuth...))
	mux.Handle("POST /api/v1/commands/start", chain(http.HandlerFunc(commands.Start), agentAuth...))
	mux.Handle("POST /api/v1/commands/complete", chain(http.HandlerFunc(commands.Complete), agentAuth...))
	mux.Handle("POST /api/v1/commands/fail", chain(http.HandlerFunc(commands.Fail), agentAuth...))
	mux.Handle("POST /api/v1/commands/approve", chain(http.HandlerFunc(commands.Approve), operatorAuth...))
	mux.Handle("POST /api/v1/capabilities", chain(http.HandlerFunc(capabilities.Sync), agentAuth...))
	mux.Handle("GET /api/v1/health", chain(http.HandlerFunc(health.List), operatorAuth...))
	mux.Handle("GET /api/v1/alerts", chain(http.HandlerFunc(alerts.List), operatorAuth...))
	mux.Handle("POST /api/v1/alerts/{id}/acknowledge", chain(http.HandlerFunc(alerts.Acknowledge), operatorAuth...))
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
