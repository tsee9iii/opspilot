package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	appalert "github.com/tsee9iii/opspilot/internal/application/alert"
)

func newEvent() appalert.AlertEvent {
	return appalert.AlertEvent{
		EventID:     uuid.NewString(),
		EventType:   "alert_opened",
		AgentID:     uuid.New(),
		ServerID:    uuid.New(),
		RuleType:    appalert.RuleAgentOffline,
		Severity:    appalert.SeverityCritical,
		Message:     "agent is offline",
		FirstSeenAt: time.Now().UTC(),
		LastSeenAt:  time.Now().UTC(),
	}
}

func TestNewDisabledWhenNoURL(t *testing.T) {
	n, err := New(Options{Secret: "s3cret"}, zap.NewNop())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if n.Enabled() {
		t.Fatal("notifier with empty URL must be disabled")
	}
	if err := n.NotifyAlertEvent(context.Background(), newEvent()); err == nil {
		t.Fatal("expected ErrDisabled for a disabled notifier")
	}
}

func TestNewRejectsNonHTTPS(t *testing.T) {
	if _, err := New(Options{URL: "http://example.com/hook", Secret: "s3cret"}, zap.NewNop()); err == nil {
		t.Fatal("expected non-https URL to be rejected")
	}
}

// TestDeliverySignsAndDelivers proves the notifier posts the exact event JSON,
// signs it with HMAC-SHA256 over the raw body, carries the event id, and
// treats 2xx as success. The https check is relaxed for the httptest server.
func TestDeliverySignsAndDelivers(t *testing.T) {
	secret := "test-secret"
	event := newEvent()

	var gotBody []byte
	var gotSig, gotEventID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get(SignatureHeader)
		gotEventID = r.Header.Get(EventIDHeader)
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	enforceHTTPS = false
	defer func() { enforceHTTPS = true }()

	n, err := New(Options{URL: srv.URL, Secret: secret}, zap.NewNop())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if !n.Enabled() {
		t.Fatal("notifier must be enabled with a URL and secret")
	}
	if err := n.NotifyAlertEvent(context.Background(), event); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if gotEventID != event.EventID {
		t.Fatalf("event id header = %q, want %q", gotEventID, event.EventID)
	}
	wantBody, _ := json.Marshal(event)
	if string(gotBody) != string(wantBody) {
		t.Fatalf("body = %s, want %s", gotBody, wantBody)
	}
	if !VerifySignature(secret, gotBody, gotSig) {
		t.Fatal("signature must verify against the exact delivered body")
	}
}

// TestDeliveryRetriesTransientFailures proves a non-2xx response is retried and
// eventually succeeds, and that delivery returns an error when it never does.
func TestDeliveryRetriesTransientFailures(t *testing.T) {
	enforceHTTPS = false
	defer func() { enforceHTTPS = true }()

	t.Run("eventually succeeds", func(t *testing.T) {
		attempts := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		n, err := New(Options{URL: srv.URL, Secret: "s3cret", Timeout: 2 * time.Second}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		if err := n.NotifyAlertEvent(context.Background(), newEvent()); err != nil {
			t.Fatalf("deliver: %v", err)
		}
		if attempts != 3 {
			t.Fatalf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("gives up after retries", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		n, err := New(Options{URL: srv.URL, Secret: "s3cret", Timeout: 2 * time.Second}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		if err := n.NotifyAlertEvent(context.Background(), newEvent()); err == nil {
			t.Fatal("expected delivery to fail after exhausting retries")
		}
	})
}

// TestVerifySignatureRejectsTampering pins the exposed verification helper.
func TestVerifySignatureRejectsTampering(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"event_type":"alert_resolved"}`)
	n := &Notifier{secret: []byte(secret)}
	good := n.sign(payload)

	if !VerifySignature(secret, payload, good) {
		t.Fatal("valid signature must verify")
	}
	if VerifySignature(secret, []byte(`{"event_type":"tampered"}`), good) {
		t.Fatal("tampered payload must not verify")
	}
	if VerifySignature("wrong-secret", payload, good) {
		t.Fatal("wrong secret must not verify")
	}
}
