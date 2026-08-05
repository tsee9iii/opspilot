package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolHTTPCheck            = "http.check"
	toolHTTPCheckVersion     = "1.0.0"
	toolHTTPCheckDescription = "Perform an HTTP health check against an endpoint"
)

const toolHTTPCheckParameterSchema = `{
  "type": "object",
  "required": ["url"],
  "properties": {
    "url": {
      "type": "string",
      "description": "HTTP or HTTPS URL"
    },
    "expected_status": {
      "type": "integer",
      "minimum": 100,
      "maximum": 599,
      "default": 200
    },
    "timeout_seconds": {
      "type": "integer",
      "minimum": 1,
      "maximum": 60,
      "default": 10
    }
  },
  "additionalProperties": false
}`

type httpCheckRequest struct {
	URL            string `json:"url"`
	ExpectedStatus int    `json:"expected_status"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type httpCheckResult struct {
	URL            string `json:"url"`
	Reachable      bool   `json:"reachable"`
	StatusCode     int    `json:"status_code"`
	ExpectedStatus int    `json:"expected_status"`
	Healthy        bool   `json:"healthy"`
	DurationMs     int64  `json:"duration_ms"`
}

// HTTPCheckTool performs a read-only HTTP health check. It never follows
// redirects and never returns the response body or headers.
type HTTPCheckTool struct {
	buildClient func(timeout time.Duration) *http.Client
}

func NewHTTPCheckTool() *HTTPCheckTool {
	return &HTTPCheckTool{buildClient: buildClient}
}

func (t *HTTPCheckTool) Name() string {
	return ToolHTTPCheck
}

func (t *HTTPCheckTool) Version() string {
	return toolHTTPCheckVersion
}

func (t *HTTPCheckTool) Description() string {
	return toolHTTPCheckDescription
}

func (t *HTTPCheckTool) ParameterSchema() string {
	return toolHTTPCheckParameterSchema
}

func (t *HTTPCheckTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *HTTPCheckTool) Availability(_ context.Context) (bool, string) {
	return true, ""
}

func (t *HTTPCheckTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := parseHTTPCheckRequest(payload)
	if err != nil {
		return nil, err
	}

	validatedURL, err := validateURL(req.URL)
	if err != nil {
		return nil, fmt.Errorf("http.check: %w", err)
	}

	client := t.buildClient(time.Duration(req.TimeoutSeconds) * time.Second)
	resp, duration, err := performRequest(ctx, client, http.MethodGet, validatedURL)
	if err != nil {
		return nil, fmt.Errorf("http.check: %w", classifyRequestError(err))
	}
	defer resp.Body.Close()

	return json.Marshal(httpCheckResult{
		URL:            req.URL,
		Reachable:      true,
		StatusCode:     resp.StatusCode,
		ExpectedStatus: req.ExpectedStatus,
		Healthy:        resp.StatusCode == req.ExpectedStatus,
		DurationMs:     duration.Milliseconds(),
	})
}

// parseHTTPCheckRequest extracts and validates the payload. expected_status
// defaults to 200 and timeout_seconds to 10 when omitted; an explicit value
// outside the schema bounds is rejected.
func parseHTTPCheckRequest(payload []byte) (httpCheckRequest, error) {
	if len(payload) == 0 {
		return httpCheckRequest{}, errors.New("http.check: payload is required")
	}
	var raw struct {
		URL            string `json:"url"`
		ExpectedStatus *int   `json:"expected_status"`
		TimeoutSeconds *int   `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return httpCheckRequest{}, fmt.Errorf("http.check: invalid payload: %w", err)
	}
	if raw.URL == "" {
		return httpCheckRequest{}, errors.New("http.check: url is required")
	}

	req := httpCheckRequest{URL: raw.URL, ExpectedStatus: 200, TimeoutSeconds: 10}
	if raw.ExpectedStatus != nil {
		if *raw.ExpectedStatus < 100 || *raw.ExpectedStatus > 599 {
			return httpCheckRequest{}, errors.New("http.check: expected_status must be 100..599")
		}
		req.ExpectedStatus = *raw.ExpectedStatus
	}
	if raw.TimeoutSeconds != nil {
		if *raw.TimeoutSeconds < 1 || *raw.TimeoutSeconds > 60 {
			return httpCheckRequest{}, errors.New("http.check: timeout_seconds must be 1..60")
		}
		req.TimeoutSeconds = *raw.TimeoutSeconds
	}
	return req, nil
}
