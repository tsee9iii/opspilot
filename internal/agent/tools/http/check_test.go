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

	"github.com/tsee9iii/opspilot/internal/agent"
)

// checkToolWithServer returns an HTTPCheckTool whose dial is routed to srv,
// with private ranges allowed so the local test server is reachable.
func checkToolWithServer(srv *httptest.Server) *HTTPCheckTool {
	tool := NewHTTPCheckToolWithPolicy(Policy{AllowPrivate: true})
	tool.dial = func(_ context.Context, network, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).Dial(network, srv.Listener.Addr().String())
	}
	return tool
}

// checkToolDialingDial returns a tool that dials through dial. Used for tests
// that exercise the real transport (TLS, connection refused).
func checkToolDialing() *HTTPCheckTool {
	return NewHTTPCheckToolWithPolicy(Policy{AllowPrivate: true})
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

	tool := checkToolWithServer(srv)
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

	tool := NewHTTPCheckToolWithPolicy(Policy{AllowPrivate: true})
	_, err := tool.Execute(context.Background(), []byte(`{"url":"http://`+addr+`"}`))
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected connection refused error, got: %v", err)
	}
}

func TestHTTPCheckDNSFailure(t *testing.T) {
	tool := NewHTTPCheckTool()
	tool.resolveHost = func(context.Context, string) ([]net.IP, error) {
		return nil, &net.DNSError{Err: "no such host", Name: "example.invalid", IsNotFound: true}
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

	// Default transport does not trust the self-signed test certificate.
	_, err := executeCheck(t, checkToolDialing(), `{"url":"`+srv.URL+`"}`)
	if err == nil || !strings.Contains(err.Error(), "TLS failure") {
		t.Fatalf("expected TLS failure error, got: %v", err)
	}
}

// --- SSRF hardening ----------------------------------------------------------

// urlHost renders a literal IP for use inside a URL (IPv6 requires brackets).
func urlHost(ip string) string {
	if strings.Contains(ip, ":") {
		return "[" + ip + "]"
	}
	return ip
}

func TestSSRFDeniesRestrictedDestinations(t *testing.T) {
	tool := NewHTTPCheckTool() // empty policy: restricted ranges denied
	for _, ip := range []string{
		"127.0.0.1",
		"127.0.0.2",
		"0.0.0.0",
		"::1",
		"10.1.2.3",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.169.254",
	} {
		t.Run(ip, func(t *testing.T) {
			_, err := tool.validateTarget(context.Background(), "http://"+urlHost(ip)+"/health")
			if err == nil {
				t.Fatalf("expected %s to be denied", ip)
			}
			if !strings.Contains(err.Error(), "restricted network range") {
				t.Fatalf("expected restricted network error, got: %v", err)
			}
		})
	}
}

func TestSSRFDeniesHostnameResolvingToRestrictedIP(t *testing.T) {
	tool := NewHTTPCheckTool()
	tool.resolveHost = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}
	_, err := tool.validateTarget(context.Background(), "http://metadata.example/")
	if err == nil {
		t.Fatal("expected hostname resolving to cloud metadata to be denied")
	}
}

func TestSSRFDeniesLoopbackEndToEnd(t *testing.T) {
	tool := NewHTTPCheckTool() // empty policy
	_, err := tool.Execute(context.Background(), []byte(`{"url":"http://127.0.0.1:9999/"}`))
	if err == nil || !strings.Contains(err.Error(), "restricted network range") {
		t.Fatalf("expected loopback denied before dialing, got: %v", err)
	}
}

func TestSSRFAllowsPublicDestinationByDefault(t *testing.T) {
	tool := NewHTTPCheckTool()
	target, err := tool.validateTarget(context.Background(), "http://93.184.216.34/health")
	if err != nil {
		t.Fatalf("public IP literal must be allowed by default: %v", err)
	}
	if target.ip != "93.184.216.34" {
		t.Fatalf("unexpected pinned ip: %s", target.ip)
	}
}

func TestSSRFAllowsPublicHostnameByDefault(t *testing.T) {
	tool := NewHTTPCheckTool()
	tool.resolveHost = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	target, err := tool.validateTarget(context.Background(), "http://public.example/health")
	if err != nil {
		t.Fatalf("public hostname must be allowed by default: %v", err)
	}
	if target.hostHeader != "public.example" {
		t.Fatalf("unexpected host header: %s", target.hostHeader)
	}
}

func TestSSRFAllowsExplicitlyConfiguredPrivateEndpoint(t *testing.T) {
	tool := NewHTTPCheckToolWithPolicy(Policy{
		AllowedEndpoints: []string{"http://127.0.0.1:8080/health"},
	})
	target, err := tool.validateTarget(context.Background(), "http://127.0.0.1:8080/health")
	if err != nil {
		t.Fatalf("explicitly configured private endpoint must be allowed: %v", err)
	}
	if target.port != "8080" {
		t.Fatalf("unexpected port: %s", target.port)
	}
}

func TestSSRFAllowsHostnameInAllowlist(t *testing.T) {
	tool := NewHTTPCheckToolWithPolicy(Policy{AllowedHosts: []string{"int.example"}})
	tool.resolveHost = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}
	if _, err := tool.validateTarget(context.Background(), "http://int.example/health"); err != nil {
		t.Fatalf("allowlisted hostname resolving to private IP must be allowed: %v", err)
	}
}

func TestSSRFRejectsUnlistedHostnameWhenAllowlistConfigured(t *testing.T) {
	tool := NewHTTPCheckToolWithPolicy(Policy{AllowedHosts: []string{"int.example"}})
	tool.resolveHost = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	if _, err := tool.validateTarget(context.Background(), "http://other.example/health"); err == nil {
		t.Fatal("expected unlisted public hostname to be denied when an allowlist is configured")
	}
}

func TestSSRFAllowsCIDR(t *testing.T) {
	tool := NewHTTPCheckToolWithPolicy(Policy{AllowedCIDRs: []string{"10.0.0.0/8"}})
	if _, err := tool.validateTarget(context.Background(), "http://10.1.2.3/health"); err != nil {
		t.Fatalf("CIDR-allowlisted private address must be allowed: %v", err)
	}
}

func TestSSRFAllowPrivateOptIn(t *testing.T) {
	tool := NewHTTPCheckToolWithPolicy(Policy{AllowPrivate: true})
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "::1"} {
		if _, err := tool.validateTarget(context.Background(), "http://"+urlHost(ip)+"/"); err != nil {
			t.Fatalf("AllowPrivate must permit %s, got: %v", ip, err)
		}
	}
}

func TestSSRFConfiguredEndpointReachableEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := NewHTTPCheckToolWithPolicy(Policy{AllowedEndpoints: []string{srv.URL + "/health"}})
	tool.dial = func(_ context.Context, network, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).Dial(network, srv.Listener.Addr().String())
	}
	res, err := executeCheck(t, tool, `{"url":"`+srv.URL+`/health"}`)
	if err != nil {
		t.Fatalf("configured endpoint must be reachable: %v", err)
	}
	if !res.Healthy {
		t.Fatalf("expected healthy: %+v", res)
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
