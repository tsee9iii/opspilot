package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	heartbeatInterval = 30 * time.Second
	registerTimeout   = 10 * time.Second
)

type Agent struct {
	cfg  *Config
	log  *zap.Logger
	http *http.Client
}

func New(cfg *Config, log *zap.Logger) *Agent {
	return &Agent{
		cfg:  cfg,
		log:  log,
		http: &http.Client{Timeout: registerTimeout},
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

	a.log.Info("agent heartbeat loop started", zap.Duration("interval", heartbeatInterval))
	return a.heartbeat(ctx)
}

func (a *Agent) heartbeat(ctx context.Context) error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("agent stopped")
			return nil
		case <-ticker.C:
			a.log.Info("agent heartbeat", zap.String("agent_id", a.cfg.AgentID))
		}
	}
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.CentralURL+"/api/v1/agents/register", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("agent: build register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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

	a.cfg.AgentID = result.AgentID
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
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
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
