package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// noRedirect stops http.Client from following redirects.
func noRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// performRequest sends a single request and returns the response and the
// elapsed duration.
func performRequest(ctx context.Context, client *http.Client, method, rawURL string) (*http.Response, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	return resp, time.Since(start), err
}

// classifyRequestError maps transport-level failures to stable messages.
func classifyRequestError(err error) error {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var recordErr *tls.RecordHeaderError
	switch {
	case isTimeout(err):
		return errors.New("request timed out")
	case errors.Is(err, syscall.ECONNREFUSED):
		return errors.New("connection refused")
	case errors.As(err, &dnsErr):
		return fmt.Errorf("DNS lookup failed: %s", dnsErr.Name)
	case errors.As(err, &certErr), errors.As(err, &recordErr):
		return errors.New("TLS failure")
	default:
		return fmt.Errorf("request failed: %w", err)
	}
}

// isTimeout reports whether err is a timeout of any kind.
func isTimeout(err error) bool {
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return errors.Is(err, context.DeadlineExceeded)
}
