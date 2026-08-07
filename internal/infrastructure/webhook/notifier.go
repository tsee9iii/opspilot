// Package webhook delivers alert lifecycle events to an outbound HTTPS
// endpoint. It is the boundary between central's internal alert state and the
// outside world: every delivery is HMAC-signed, time-bounded, idempotent by
// event id and (in production) HTTPS-only.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
)

// Delivery boundary constants.
const (
	// SignatureHeader carries the hex HMAC-SHA256 of the canonical payload.
	SignatureHeader = "X-OpsPilot-Signature"
	// EventIDHeader echoes the event id so a receiver can deduplicate retries.
	EventIDHeader = "X-OpsPilot-Event-ID"
	// maxRetries bounds the number of delivery attempts per event.
	maxRetries = 3
	// maxPayloadBytes bounds the outbound body.
	maxPayloadBytes = 1 << 20
)

var (
	// ErrDisabled is returned when delivery is attempted while the notifier is
	// not configured (disabled).
	ErrDisabled = errors.New("webhook notifier disabled")
	// ErrNotHTTPS is returned when a non-HTTPS URL is configured.
	ErrNotHTTPS = errors.New("webhook URL must use https")
)

// enforceHTTPS requires the https scheme. It is overridden to http only in
// tests so real delivery can be exercised against an httptest server.
var enforceHTTPS = true

// Options configures the notifier. URL must be HTTPS in production (the caller
// enforces this during config validation; the notifier re-checks defensively).
type Options struct {
	// URL is the outbound endpoint.
	URL string
	// Secret signs payloads with HMAC-SHA256.
	Secret string
	// Timeout bounds each delivery attempt. Zero uses the default.
	Timeout time.Duration
}

// Notifier delivers alert events via HTTP. It satisfies appalert.Notifier.
type Notifier struct {
	url     *url.URL
	secret  []byte
	timeout time.Duration
	client  *http.Client
	log     *zap.Logger
}

// New returns a delivery-capable notifier. When url is empty the notifier is
// disabled: NotifyAlertEvent returns ErrDisabled and never makes a request.
func New(opts Options, log *zap.Logger) (*Notifier, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	n := &Notifier{
		secret:  []byte(opts.Secret),
		timeout: timeout,
		log:     log,
	}
	if opts.URL != "" {
		u, err := url.Parse(opts.URL)
		if err != nil {
			return nil, fmt.Errorf("webhook: parse url: %w", err)
		}
		if enforceHTTPS && u.Scheme != "https" {
			return nil, ErrNotHTTPS
		}
		n.url = u
	}
	n.client = &http.Client{Timeout: timeout}
	return n, nil
}

// Enabled reports whether the notifier will actually deliver events.
func (n *Notifier) Enabled() bool { return n.url != nil && len(n.secret) > 0 }

// NotifyAlertEvent delivers a signed, idempotent webhook payload. It returns
// immediately on a failed delivery (the evaluator never blocks on webhooks);
// bounded retries happen synchronously with short timeouts.
func (n *Notifier) NotifyAlertEvent(ctx context.Context, event appalert.AlertEvent) error {
	if !n.Enabled() {
		return ErrDisabled
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("webhook: marshal event: %w", err)
	}
	if len(payload) > maxPayloadBytes {
		return fmt.Errorf("webhook: payload exceeds %d bytes", maxPayloadBytes)
	}

	signature := n.sign(payload)
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		lastErr = n.deliver(ctx, payload, event.EventID, signature)
		if lastErr == nil {
			return nil
		}
		n.log.Warn("webhook delivery failed",
			zap.String("event_id", event.EventID),
			zap.String("event_type", event.EventType),
			zap.Int("attempt", attempt),
			zap.Error(lastErr),
		)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	return lastErr
}

func (n *Notifier) deliver(ctx context.Context, payload []byte, eventID, signature string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, signature)
	req.Header.Set(EventIDHeader, eventID)

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request: %w", err)
	}
	defer resp.Body.Close()
	// The body is drained and discarded; it is never logged or parsed.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
}

// sign computes the hex HMAC-SHA256 of the payload. The entire raw payload is
// signed so a receiver can verify byte-for-byte integrity.
func (n *Notifier) sign(payload []byte) string {
	mac := hmac.New(sha256.New, n.secret)
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature reports whether signature is valid for payload. It is exposed
// for receivers and tests.
func VerifySignature(secret string, payload []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if len(signature) != len(expected) {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(expected))
}

// isHTTPS reports whether the URL uses the https scheme. Used defensively.
func isHTTPS(raw string) bool {
	return strings.HasPrefix(raw, "https://")
}
