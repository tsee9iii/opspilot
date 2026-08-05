package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opspilot/opspilot/internal/agent"
)

// checkToolWithServer returns an HTTPCheckTool whose client routes to srv.
func checkToolWithServer(srv *httptest.Server) *HTTPCheckTool {
	tool := NewHTTPCheckTool()
	tool.buildClient = func(timeout time.Duration) *http.Client {
		client := srv.Client()
		client.Timeout = timeout
		client.CheckRedirect = noRedirect
		return client
	}
	return tool
}

func executeCheck(t *testing.T, tool *HTTPCheckTool, payload string) (httpCheckResult, error) {
	t.Helper()
	out, err := tool.Execute(context.Background(), []byte(payload))
	if err != nil {
		return httpCheckResult{}, err
	}
	var res httpCheckResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res, nil
}

func TestHTTPCheckToolMetadata(t *testing.T) {
	tool := NewHTTPCheckTool()
	if tool.Name() != ToolHTTPCheck {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() != "1.0.0" {
		t.Fatalf("unexpected version: %s", tool.Version())
	}
	if tool.Description() == "" {
		t.Fatal("missing description")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestHTTPCheckToolAvailability(t *testing.T) {
	ok, reason := NewHTTPCheckTool().Availability(context.Background())
	if !ok || reason != "" {
		t.Fatalf("expected always available, got ok=%v reason=%q", ok, reason)
	}
}

func TestHTTPCheckParameterSchema(t *testing.T) {
	tool := NewHTTPCheckTool()
	var schema struct {
		Type                 string   `json:"type"`
		Required             []string `json:"required"`
		AdditionalProperties bool     `json:"additionalProperties"`
		Properties           map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
			Minimum     *int   `json:"minimum"`
			Maximum     *int   `json:"maximum"`
			Default     *int   `json:"default"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("unexpected schema type: %s", schema.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "url" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	urlProp, ok := schema.Properties["url"]
	if !ok || urlProp.Type != "string" || urlProp.Description == "" {
		t.Fatalf("unexpected url property: %+v", urlProp)
	}
	status, ok := schema.Properties["expected_status"]
	if !ok || status.Type != "integer" || *status.Minimum != 100 || *status.Maximum != 599 || *status.Default != 200 {
		t.Fatalf("unexpected expected_status property: %+v", status)
	}
	timeout, ok := schema.Properties["timeout_seconds"]
	if !ok || timeout.Type != "integer" || *timeout.Minimum != 1 || *timeout.Maximum != 60 || *timeout.Default != 10 {
		t.Fatalf("unexpected timeout_seconds property: %+v", timeout)
	}
	if schema.AdditionalProperties {
		t.Fatal("expected additionalProperties: false")
	}
}

func TestHTTPCheckHealthyEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res, err := executeCheck(t, checkToolWithServer(srv), `{"url":"`+srv.URL+`/health"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Reachable || !res.Healthy {
		t.Fatalf("expected reachable and healthy: %+v", res)
	}
	if res.StatusCode != 200 || res.ExpectedStatus != 200 {
		t.Fatalf("unexpected statuses: %+v", res)
	}
	if res.URL != srv.URL+"/health" {
		t.Fatalf("unexpected url: %s", res.URL)
	}
	if res.DurationMs < 0 {
		t.Fatalf("unexpected duration: %d", res.DurationMs)
	}
}

func TestHTTPCheckUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	res, err := executeCheck(t, checkToolWithServer(srv), `{"url":"`+srv.URL+`","expected_status":200}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Reachable || res.Healthy {
		t.Fatalf("expected reachable but unhealthy: %+v", res)
	}
	if res.StatusCode != 503 {
		t.Fatalf("unexpected status code: %d", res.StatusCode)
	}
}

func TestHTTPCheckRedirectNotFollowed(t *testing.T) {
	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	res, err := executeCheck(t, checkToolWithServer(redirector), `{"url":"`+redirector.URL+`"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected the 302 to be reported unfollowed, got %d", res.StatusCode)
	}
	if res.Healthy {
		t.Fatal("expected unhealthy for a redirect")
	}
	if atomic.LoadInt32(&targetHits) != 0 {
		t.Fatal("redirect target should never have been hit")
	}
}

func TestHTTPCheckTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	tool := NewHTTPCheckTool()
	tool.buildClient = func(timeout time.Duration) *http.Client {
		client := srv.Client()
		client.Timeout = timeout
		client.CheckRedirect = noRedirect
		return client
	}
	_, err := tool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`","timeout_seconds":1}`))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestHTTPCheckInvalidURL(t *testing.T) {
	tool := NewHTTPCheckTool()
	_, err := tool.Execute(context.Background(), []byte(`{"url":"http://exa mple.com"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid URL") {
		t.Fatalf("expected invalid URL error, got: %v", err)
	}
}

func TestHTTPCheckUnsupportedScheme(t *testing.T) {
	tool := NewHTTPCheckTool()
	_, err := tool.Execute(context.Background(), []byte(`{"url":"ftp://example.com/file"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("expected unsupported scheme error, got: %v", err)
	}
}

func TestHTTPCheckConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.Listener.Addr().String()
	srv.Close()

	tool := NewHTTPCheckTool()
	_, err := tool.Execute(context.Background(), []byte(`{"url":"http://`+addr+`"}`))
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected connection refused error, got: %v", err)
	}
}

type dnsFailTransport struct{}

func (dnsFailTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, &net.DNSError{Err: "no such host", Name: req.URL.Host, IsNotFound: true}
}

func TestHTTPCheckDNSFailure(t *testing.T) {
	tool := NewHTTPCheckTool()
	tool.buildClient = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout, Transport: dnsFailTransport{}}
	}
	_, err := tool.Execute(context.Background(), []byte(`{"url":"http://example.invalid/"}`))
	if err == nil || !strings.Contains(err.Error(), "DNS lookup failed") {
		t.Fatalf("expected DNS lookup error, got: %v", err)
	}
}

func TestHTTPCheckTLSFailure(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := NewHTTPCheckTool()
	tool.buildClient = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout} // default transport does not trust srv's cert
	}
	_, err := tool.Execute(context.Background(), []byte(`{"url":"`+srv.URL+`"}`))
	if err == nil || !strings.Contains(err.Error(), "TLS failure") {
		t.Fatalf("expected TLS failure error, got: %v", err)
	}
}

func TestHTTPCheckParseRequestErrors(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"url":""}`,
		`not json`,
		`{"url":"http://example.com","expected_status":99}`,
		`{"url":"http://example.com","expected_status":600}`,
		`{"url":"http://example.com","timeout_seconds":0}`,
		`{"url":"http://example.com","timeout_seconds":-1}`,
		`{"url":"http://example.com","timeout_seconds":61}`,
	}
	for _, c := range cases {
		if _, err := parseHTTPCheckRequest([]byte(c)); err == nil {
			t.Fatalf("expected error for payload: %q", c)
		}
	}
}

func TestHTTPCheckParseRequestDefaults(t *testing.T) {
	req, err := parseHTTPCheckRequest([]byte(`{"url":"http://example.com"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ExpectedStatus != 200 || req.TimeoutSeconds != 10 {
		t.Fatalf("unexpected defaults: %+v", req)
	}
}
