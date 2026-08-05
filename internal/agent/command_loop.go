package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const defaultPollInterval = 5 * time.Second

var errNoPendingCommands = errors.New("no pending commands")

type leasedCommand struct {
	ID      string          `json:"command_id"`
	Tool    string          `json:"tool"`
	Payload json.RawMessage `json:"payload"`
}

// pollCommands runs the lease -> start -> execute -> report loop until the
// context is cancelled.
func (a *Agent) pollCommands(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info("agent stopped")
			return nil
		case <-timer.C:
			if err := a.pollOnce(ctx); err != nil {
				a.log.Warn("command poll failed", zap.Error(err))
			}
			timer.Reset(interval)
		}
	}
}

func (a *Agent) pollOnce(ctx context.Context) error {
	cmd, err := a.leaseCommand(ctx)
	if errors.Is(err, errNoPendingCommands) {
		return nil
	}
	if err != nil {
		return err
	}

	a.log.Info("command leased", zap.String("command_id", cmd.ID), zap.String("tool", cmd.Tool))

	if err := a.startCommand(ctx, cmd.ID); err != nil {
		return fmt.Errorf("agent: start command %s: %w", cmd.ID, err)
	}

	result, execErr := a.executor.Execute(ctx, cmd.Tool, cmd.Payload)
	if execErr != nil {
		a.log.Info("command failed", zap.String("command_id", cmd.ID), zap.Error(execErr))
		if err := a.failCommand(ctx, cmd.ID, execErr.Error()); err != nil {
			return fmt.Errorf("agent: fail command %s: %w", cmd.ID, err)
		}
		return nil
	}

	a.log.Info("command completed", zap.String("command_id", cmd.ID))
	if err := a.completeCommand(ctx, cmd.ID, result); err != nil {
		return fmt.Errorf("agent: complete command %s: %w", cmd.ID, err)
	}
	return nil
}

func (a *Agent) leaseCommand(ctx context.Context) (leasedCommand, error) {
	body, err := json.Marshal(leaseRequest{AgentID: a.cfg.AgentID})
	if err != nil {
		return leasedCommand{}, fmt.Errorf("agent: marshal lease request: %w", err)
	}

	resp, err := a.postJSON(ctx, "/api/v1/commands/lease", body)
	if err != nil {
		return leasedCommand{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return leasedCommand{}, errNoPendingCommands
	}
	if resp.StatusCode != http.StatusOK {
		return leasedCommand{}, fmt.Errorf("agent: lease failed: %s", readErrorMessage(resp))
	}

	var cmd leasedCommand
	if err := json.NewDecoder(resp.Body).Decode(&cmd); err != nil {
		return leasedCommand{}, fmt.Errorf("agent: decode lease response: %w", err)
	}
	if cmd.ID == "" {
		return leasedCommand{}, fmt.Errorf("agent: empty command_id in lease response")
	}
	return cmd, nil
}

func (a *Agent) startCommand(ctx context.Context, commandID string) error {
	return a.reportCommand(ctx, "/api/v1/commands/start", reportRequest{
		AgentID:   a.cfg.AgentID,
		CommandID: commandID,
	})
}

func (a *Agent) completeCommand(ctx context.Context, commandID string, result []byte) error {
	return a.reportCommand(ctx, "/api/v1/commands/complete", reportRequest{
		AgentID:   a.cfg.AgentID,
		CommandID: commandID,
		Result:    json.RawMessage(result),
	})
}

func (a *Agent) failCommand(ctx context.Context, commandID, errMsg string) error {
	return a.reportCommand(ctx, "/api/v1/commands/fail", reportRequest{
		AgentID:   a.cfg.AgentID,
		CommandID: commandID,
		Error:     errMsg,
	})
}

func (a *Agent) reportCommand(ctx context.Context, path string, req reportRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("agent: marshal request: %w", err)
	}

	resp, err := a.postJSON(ctx, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent: %s failed: %s", path, readErrorMessage(resp))
	}
	return nil
}

func (a *Agent) postJSON(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.CentralURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("agent: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent: request %s: %w", path, err)
	}
	return resp, nil
}

func (a *Agent) pollInterval() time.Duration {
	if a.cfg.PollInterval <= 0 {
		return defaultPollInterval
	}
	return time.Duration(a.cfg.PollInterval) * time.Second
}

type leaseRequest struct {
	AgentID string `json:"agent_id"`
}

type reportRequest struct {
	AgentID   string          `json:"agent_id"`
	CommandID string          `json:"command_id"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}
