package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SSE wake-up stream tuning.
const (
	// sseInitialBackoff is the first reconnect delay.
	sseInitialBackoff = 1 * time.Second
	// sseMaxBackoff caps the exponential backoff between reconnects.
	sseMaxBackoff = 30 * time.Second
	// sseBackoffFactor is the exponential growth between attempts.
	sseBackoffFactor = 2.0
	// sseJitterFraction bounds the +/- jitter applied to each delay.
	sseJitterFraction = 0.3
	// sseStableConnected is how long a stream must stay connected before the
	// backoff resets to its initial value.
	sseStableConnected = 60 * time.Second
)

// runSSEListener maintains the wake-up stream to central until the context is
// cancelled. It reconnects with exponential backoff and bounded jitter after
// every EOF, network error or server restart, and signals the command loop's
// wake channel on every wake-up event and on every (re)connect so a command is
// never delayed waiting for the fallback poll. Reconnect attempts and logs are
// bounded by the backoff, so a flapping connection cannot log noisily.
func (a *Agent) runSSEListener(ctx context.Context, wake chan<- struct{}) {
	backoff := a.sseInitialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		err := a.consumeSSE(ctx, wake)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.log.Warn("SSE wake-up stream disconnected",
				zap.Error(err), zap.Duration("retry_in", backoff))
		}
		// Immediately attempt a lease so a command created around a reconnect
		// is not delayed until the next fallback poll.
		signalWake(wake)
		if !a.sleep(ctx, backoff) {
			return
		}
		if time.Since(start) >= a.sseStable {
			backoff = a.sseInitialBackoff
		} else {
			backoff = nextSSEBackoff(backoff, a.sseMaxBackoff)
		}
	}
}

// consumeSSE connects to the central wake-up endpoint and reads the event
// stream until it breaks or the context is cancelled. It returns an error when
// the stream must be re-established, and nil on a clean context cancellation.
func (a *Agent) consumeSSE(ctx context.Context, wake chan<- struct{}) error {
	resp, err := a.openSSEStream(ctx)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent: SSE stream rejected: %s", readErrorMessage(resp))
	}

	reader := bufio.NewReader(resp.Body)
	event := ""
	var data []byte
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("agent: read SSE: %w", err)
		}
		eof := errors.Is(err, io.EOF) && len(line) == 0

		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			// End of an event. Only wakeups matter; everything else on the
			// stream is a keepalive or noise and is ignored.
			if event == "wakeup" && len(data) > 0 {
				signalWake(wake)
			}
			event, data = "", nil
		case strings.HasPrefix(line, ":"):
			// Comment/heartbeat. Ignored.
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			d := strings.TrimPrefix(line, "data:")
			if len(d) > 0 && d[0] == ' ' {
				d = d[1:]
			}
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, d...)
		}

		if eof {
			return errors.New("agent: SSE stream closed by central")
		}
	}
}

// openSSEStream opens the authenticated SSE GET request. The existing
// newRequest/sign helpers are method-agnostic, so a GET with an empty body is
// signed exactly like any other agent request.
func (a *Agent) openSSEStream(ctx context.Context) (*http.Response, error) {
	req, err := a.newRequest(ctx, http.MethodGet, "/api/v1/agents/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := a.sseHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent: SSE connect: %w", err)
	}
	return resp, nil
}

// sleep waits d or until ctx is cancelled. It reports whether the full delay
// elapsed (false means the caller should exit).
func (a *Agent) sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// nextSSEBackoff grows the delay exponentially, applies +/- jitter, and caps
// it at max. The result never exceeds max, keeping reconnect delays bounded.
func nextSSEBackoff(current, max time.Duration) time.Duration {
	next := float64(current) * sseBackoffFactor
	if next > float64(max) {
		next = float64(max)
	}
	jittered := next * (1 - sseJitterFraction + rand.Float64()*2*sseJitterFraction)
	if jittered > float64(max) {
		jittered = float64(max)
	}
	if jittered < 1 {
		jittered = 1
	}
	return time.Duration(jittered)
}
