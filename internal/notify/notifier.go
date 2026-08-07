// Package notify implements the in-memory agent wake-up notifier used by
// central to signal a connected agent that a leasable command is available.
//
// The notifier is intentionally minimal: it is a wake-up channel only. Command
// payloads, credentials, approvals and results are never carried here — the
// agent always re-reads its queue through the authenticated lease endpoint, so
// this package can lose or coalesce events without any correctness impact.
//
// Single-Central limitation: this implementation lives entirely in one process.
// A central multi-instance deployment would need a shared notifier such as
// PostgreSQL LISTEN/NOTIFY or a message broker so that a command created on one
// instance can wake a stream served by another. For the single-owner personal
// deployment this repository targets, in-memory delivery is sufficient and the
// database queue plus fallback polling remains the source of truth.
package notify

import "sync"

// Notifier fans out wake-up signals to the single active SSE stream of each
// agent. All methods are safe for concurrent use.
type Notifier struct {
	mu   sync.Mutex
	subs map[string]*Subscription
	// closed is set by Close; after that, Subscribe returns an already-cancelled
	// subscription and Notify is a no-op.
	closed bool
}

// New returns an empty notifier.
func New() *Notifier {
	return &Notifier{subs: make(map[string]*Subscription)}
}

// Subscription is a live wake-up channel for one agent. It is created by
// Subscribe and consumed by the SSE handler.
type Subscription struct {
	n       *Notifier
	agentID string
	// wake carries coalesced wake-up signals. It is buffered with capacity one
	// so a Notify that arrives while a wake is already pending is dropped
	// (one pending wake-up is enough).
	wake chan struct{}
	// done is closed when the subscription is cancelled: replaced by a newer
	// stream for the same agent, closed by Notifier.Close, or Unsubscribe.
	done chan struct{}
	// closeOnce guarantees done is closed exactly once.
	closeOnce sync.Once
}

// Subscribe registers a new wake-up stream for an agent and returns its
// subscription. At most one subscription is active per agent: any existing one
// is cancelled first, so a new SSE stream for the same agent cleanly replaces
// the old one.
func (n *Notifier) Subscribe(agentID string) *Subscription {
	n.mu.Lock()
	defer n.mu.Unlock()

	if prev, ok := n.subs[agentID]; ok {
		prev.close()
	}

	s := &Subscription{
		n:       n,
		agentID: agentID,
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	if n.closed {
		// The notifier is shutting down: hand back a stream that is already
		// cancelled so a handler exits immediately.
		s.close()
	}
	n.subs[agentID] = s
	return s
}

// Notify signals the agent's active subscription (if any) that a leasable
// command may be available. It never blocks: a notification for an agent with
// no stream, or with a wake already pending, is dropped. Repeated wake-ups are
// coalesced so a burst of command creations produces at most one pending
// signal.
func (n *Notifier) Notify(agentID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return
	}
	s, ok := n.subs[agentID]
	if !ok {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Close cancels every active subscription and rejects future Notify calls. It
// is safe to call during shutdown and from any goroutine; it is idempotent.
func (n *Notifier) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return
	}
	n.closed = true
	for _, s := range n.subs {
		s.close()
	}
	n.subs = make(map[string]*Subscription)
}

// Wake receives a signal each time a wake-up is delivered. Signals are
// coalesced: if a wake is already pending, a new one is dropped. The channel is
// never closed.
func (s *Subscription) Wake() <-chan struct{} { return s.wake }

// Done is closed when the subscription is cancelled: when it is replaced by a
// newer stream for the same agent, when the notifier shuts down, or when
// Unsubscribe is called.
func (s *Subscription) Done() <-chan struct{} { return s.done }

// Unsubscribe removes the subscription from the notifier. It is safe to call
// multiple times and from any goroutine; a subscription that was already
// replaced is a no-op.
func (s *Subscription) Unsubscribe() {
	s.n.unsubscribe(s.agentID, s)
}

func (s *Subscription) close() {
	s.closeOnce.Do(func() { close(s.done) })
}

func (n *Notifier) unsubscribe(agentID string, s *Subscription) {
	n.mu.Lock()
	defer n.mu.Unlock()
	cur, ok := n.subs[agentID]
	if ok && cur == s {
		delete(n.subs, agentID)
		s.close()
	}
}
