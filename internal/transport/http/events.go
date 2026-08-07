package http

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tsee9iii/opspilot/internal/agentsign"
	"github.com/tsee9iii/opspilot/internal/notify"
)

// SSE stream tuning. Heartbeats keep the stream alive through NATs and reverse
// proxies that idle-close connections; the per-write deadline bounds a client
// that stops reading so a handler goroutine can never block forever.
const (
	sseHeartbeatInterval = 15 * time.Second
	ssePerWriteTimeout   = 30 * time.Second
)

// AgentEventHandler streams wake-up notifications to a single connected agent.
//
// Streaming timeout design: the central http.Server sets a global WriteTimeout
// (30s) that would kill any long-lived response. A dedicated second listener
// was deliberately avoided; instead this handler uses http.NewResponseController
// to clear the global write deadline for its own response only, while every
// other endpoint keeps its safe limit. A bounded per-write deadline is re-armed
// before each write, so a stalled client still cannot pin the goroutine.
type AgentEventHandler struct {
	notifier *notify.Notifier
	// heartbeatInterval is the keepalive period (overridable in tests).
	heartbeatInterval time.Duration
	// writeTimeout bounds a single SSE write to a stalled client (overridable
	// in tests).
	writeTimeout time.Duration
}

// NewAgentEventHandler builds the SSE handler backed by the given notifier.
func NewAgentEventHandler(n *notify.Notifier) *AgentEventHandler {
	return &AgentEventHandler{
		notifier:          n,
		heartbeatInterval: sseHeartbeatInterval,
		writeTimeout:      ssePerWriteTimeout,
	}
}

// Events streams SSE wake-up notifications for the authenticated agent.
//
// The agent identity is taken from the X-Agent-Id header, which AgentAuth has
// already verified against the signing key of a registered agent; the handler
// never reads a body and carries no command data — the agent reacts to a wake
// by calling the authenticated lease endpoint, so PostgreSQL remains the source
// of truth.
func (h *AgentEventHandler) Events(w http.ResponseWriter, r *http.Request) {
	agentID := r.Header.Get(agentsign.HeaderAgentID)

	rc := http.NewResponseController(w)
	// Drop the server-wide write deadline for this stream; every write below
	// re-arms a bounded per-write deadline instead.
	_ = rc.SetWriteDeadline(time.Time{})

	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return
	}

	sub := h.notifier.Subscribe(agentID)
	defer sub.Unsubscribe()

	if !h.writeEvent(w, rc, "connected", fmt.Sprintf(`{"agent_id":%q,"status":"connected"}`, agentID)) {
		return
	}

	heartbeat := time.NewTicker(h.heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			// Client went away or the server is shutting down.
			return
		case <-sub.Done():
			// The stream was replaced by a newer stream for the same agent, or
			// the notifier was closed during shutdown.
			return
		case <-heartbeat.C:
			if !h.writeComment(w, rc) {
				return
			}
		case <-sub.Wake():
			if !h.writeEvent(w, rc, "wakeup", fmt.Sprintf(`{"agent_id":%q,"reason":"command_available"}`, agentID)) {
				return
			}
		}
	}
}

// writeEvent writes a single SSE event and flushes it. It returns false when
// the stream is no longer writable, which terminates the handler.
func (h *AgentEventHandler) writeEvent(w io.Writer, rc *http.ResponseController, event, data string) bool {
	_ = rc.SetWriteDeadline(time.Now().Add(h.writeTimeout))
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// writeComment writes an SSE comment (`: ping`), which is ignored by SSE
// parsers but keeps the connection alive. Returns false on write failure.
func (h *AgentEventHandler) writeComment(w io.Writer, rc *http.ResponseController) bool {
	_ = rc.SetWriteDeadline(time.Now().Add(h.writeTimeout))
	if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
		return false
	}
	return rc.Flush() == nil
}
