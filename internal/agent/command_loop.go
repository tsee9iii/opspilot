package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	// defaultPollInterval is the fallback command-poll interval used when the
	// config does not set poll_interval. With the SSE wake-up stream enabled
	// (the default) polling is a recovery mechanism for disconnections and
	// startup, so it is deliberately conservative. When SSE is disabled this
	// interval is the only delivery path and should be lowered.
	defaultPollInterval = 30 * time.Second
)

var errNoPendingCommands = errors.New("no pending commands")

type leasedCommand struct {
	ID      string          `json:"command_id"`
	Tool    string          `json:"tool"`
	Payload json.RawMessage `json:"payload"`
}

// pollCommands runs the lease -> start -> execute -> report loop until the
// context is cancelled. The wake channel carries SSE wake-up signals: each one
// triggers an immediate lease attempt. Wakes are coalesced (the channel holds
// at most one pending signal) and are only observed between pollOnce calls, so
// command execution stays strictly sequential — a wake during an active
// execution never causes a concurrent run. The fallback timer still fires on
// interval, which covers SSE loss and disabled SSE.
func (a *Agent) pollCommands(ctx context.Context, interval time.Duration, wake <-chan struct{}) error {
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
		case <-wake:
			if err := a.pollOnce(ctx); err != nil {
				a.log.Warn("command poll failed", zap.Error(err))
			}
		}
	}
}

// signalWake delivers a wake-up to the command loop without blocking. If a
// wake is already pending it is coalesced and dropped.
func signalWake(wake chan<- struct{}) {
	select {
	case wake <- struct{}{}:
	default:
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
	req, err := a.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}

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
