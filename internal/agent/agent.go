package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/tsee9iii/opspilot/internal/agentsign"
)

const (
	heartbeatInterval = 30 * time.Second
	registerTimeout   = 10 * time.Second
)

type Agent struct {
	cfg      *Config
	log      *zap.Logger
	http     *http.Client
	executor Executor
	registry *Registry
}

func New(cfg *Config, log *zap.Logger, executor Executor, registry *Registry) *Agent {
	return &Agent{
		cfg:      cfg,
		log:      log,
		http:     &http.Client{Timeout: registerTimeout},
		executor: executor,
		registry: registry,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if a.cfg.AgentID == "" {
		if err := a.register(ctx); err != nil {
			return err
		}
		a.log.Info("agent registered", zap.String("agent_id", a.cfg.AgentID))
	} else {
		a.log.Info("agent identity present", zap.String("agent_id", a.cfg.AgentID))
	}

	if err := a.registerCapabilities(ctx); err != nil {
		a.log.Warn("capability registration failed", zap.Error(err))
	} else {
		a.log.Info("capabilities registered", zap.Int("count", len(a.registry.List())))
	}

	a.log.Info("agent heartbeat loop started", zap.Duration("interval", heartbeatInterval))
	go a.heartbeat(ctx)

	interval := a.pollInterval()
	a.log.Info("agent command loop started", zap.Duration("interval", interval))
	return a.pollCommands(ctx, interval)
}

func (a *Agent) heartbeat(ctx context.Context) error {
	interval := heartbeatInterval
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			next, err := a.sendHeartbeat(ctx)
			if err != nil {
				a.log.Warn("heartbeat failed", zap.Error(err))
				interval = heartbeatInterval
			} else {
				a.log.Info("heartbeat sent", zap.Duration("next", next))
				interval = next
			}
			timer.Reset(interval)
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) (time.Duration, error) {
	body, err := json.Marshal(heartbeatRequest{
		AgentID: a.cfg.AgentID,
	})
	if err != nil {
		return 0, fmt.Errorf("agent: marshal heartbeat: %w", err)
	}

	req, err := a.newRequest(ctx, http.MethodPost, "/api/v1/agents/heartbeat", body)
	if err != nil {
		return 0, err
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("agent: heartbeat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("agent: heartbeat failed: %s", readErrorMessage(resp))
	}

	var result heartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("agent: decode heartbeat response: %w", err)
	}
	if result.NextHeartbeat <= 0 {
		result.NextHeartbeat = int(heartbeatInterval.Seconds())
	}

	return time.Duration(result.NextHeartbeat) * time.Second, nil
}

type heartbeatRequest struct {
	AgentID string `json:"agent_id"`
}

type heartbeatResponse struct {
	Status        string `json:"status"`
	NextHeartbeat int    `json:"next_heartbeat"`
}

func (a *Agent) register(ctx context.Context) error {
	if err := a.cfg.ValidateRegistration(); err != nil {
		return err
	}

	body, err := json.Marshal(registerRequest{
		RegistrationToken: a.cfg.RegistrationToken,
		Secret:            a.cfg.Secret,
		Version:           a.cfg.Version,
		Server: serverInfo{
			Hostname:    a.cfg.Server.Hostname,
			Environment: a.cfg.Server.Environment,
		},
	})
	if err != nil {
		return fmt.Errorf("agent: marshal register request: %w", err)
	}

	req, err := a.newRequest(ctx, http.MethodPost, "/api/v1/agents/register", body)
	if err != nil {
		return err
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent: register request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("agent: register failed: %s", readErrorMessage(resp))
	}

	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("agent: decode register response: %w", err)
	}
	if result.AgentID == "" {
		return fmt.Errorf("agent: empty agent_id in register response")
	}
	if result.SigningKey == "" {
		return fmt.Errorf("agent: empty signing_key in register response")
	}

	a.cfg.AgentID = result.AgentID
	a.cfg.SigningKey = result.SigningKey
	return a.cfg.Save()
}

type registerRequest struct {
	RegistrationToken string     `json:"registration_token"`
	Secret            string     `json:"secret"`
	Version           string     `json:"version"`
	Server            serverInfo `json:"server"`
}

type serverInfo struct {
	Hostname    string `json:"hostname"`
	Environment string `json:"environment"`
}

type registerResponse struct {
	AgentID    string `json:"agent_id"`
	Status     string `json:"status"`
	SigningKey string `json:"signing_key"`
}

// newRequest builds an HTTP request to central. Once the agent has an identity
// and signing key (i.e. after registration), every request is HMAC-signed so
// central can authenticate it.
func (a *Agent) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.CentralURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("agent: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if a.cfg.AgentID != "" && a.cfg.SigningKey != "" {
		if err := a.sign(req, method, path, body); err != nil {
			return nil, err
		}
	}
	return req, nil
}

// sign attaches the HMAC request-signing headers to req.
func (a *Agent) sign(req *http.Request, method, path string, body []byte) error {
	nonce, err := agentsign.NewNonce()
	if err != nil {
		return fmt.Errorf("agent: generate nonce: %w", err)
	}
	timestamp := agentsign.Timestamp()
	canonical := agentsign.Canonical(a.cfg.AgentID, timestamp, nonce, method, path, string(body))

	req.Header.Set(agentsign.HeaderAgentID, a.cfg.AgentID)
	req.Header.Set(agentsign.HeaderAgentTimestamp, timestamp)
	req.Header.Set(agentsign.HeaderAgentNonce, nonce)
	req.Header.Set(agentsign.HeaderAgentSignature, agentsign.Sign(a.cfg.SigningKey, canonical))
	return nil
}

func readErrorMessage(resp *http.Response) string {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return resp.Status
	}
	return fmt.Sprintf("%s (%s)", envelope.Error.Message, envelope.Error.Code)
}
