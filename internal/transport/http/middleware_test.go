package http

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestOperatorAuthRequiresToken(t *testing.T) {
	h := chain(okHandler(), OperatorAuth(""))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestOperatorAuthRejectsWrongToken(t *testing.T) {
	h := chain(okHandler(), OperatorAuth("correct-token"))

	for _, auth := range []string{
		"Bearer wrong-token",
		"Bearer correct-token ",
		"correct-token",
		"",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/123", nil)
		req.Header.Set("Authorization", auth)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q: expected 401, got %d", auth, rec.Code)
		}
	}
}

func TestOperatorAuthAcceptsCorrectToken(t *testing.T) {
	h := chain(okHandler(), OperatorAuth("correct-token"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/123", nil)
	req.Header.Set("Authorization", "Bearer correct-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRecoveryConvertsPanicTo500(t *testing.T) {
	panicHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	h := chain(panicHandler, Recovery(zap.NewNop()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if want := "internal_error"; errorCode(t, rec) != want {
		t.Fatalf("expected error code %q, got %q", want, errorCode(t, rec))
	}
}

func TestRecoveryPassesThroughNormalRequests(t *testing.T) {
	h := chain(okHandler(), Recovery(zap.NewNop()))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMaxBodyBytesRejectsOversizedBody(t *testing.T) {
	var sawError error
	readHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawError = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	h := chain(readHandler, MaxBodyBytes(16))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader(strings.Repeat("x", 100)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if sawError == nil {
		t.Fatal("expected body read to fail")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(sawError, &maxErr) {
		t.Fatalf("expected *http.MaxBytesError, got %T", sawError)
	}
}

func TestMaxBodyBytesAllowsBodyWithinLimit(t *testing.T) {
	var body []byte
	readHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	h := chain(readHandler, MaxBodyBytes(16))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/register", strings.NewReader("short"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if string(body) != "short" {
		t.Fatalf("expected body to pass through, got %q", body)
	}
}
