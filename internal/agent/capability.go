package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type capabilityInfo struct {
	ToolName    string `json:"tool_name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type syncCapabilitiesRequest struct {
	AgentID      string           `json:"agent_id"`
	Secret       string           `json:"secret"`
	Capabilities []capabilityInfo `json:"capabilities"`
}

// registerCapabilities sends the metadata of every registered tool to Central.
func (a *Agent) registerCapabilities(ctx context.Context) error {
	names := a.registry.List()
	req := syncCapabilitiesRequest{
		AgentID:      a.cfg.AgentID,
		Secret:       a.cfg.Secret,
		Capabilities: make([]capabilityInfo, 0, len(names)),
	}
	for _, name := range names {
		tool, _ := a.registry.Find(name)
		req.Capabilities = append(req.Capabilities, capabilityInfo{
			ToolName:    tool.Name(),
			Version:     tool.Version(),
			Description: tool.Description(),
		})
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("agent: marshal capabilities: %w", err)
	}

	resp, err := a.postJSON(ctx, "/api/v1/capabilities", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent: capabilities sync failed: %s", readErrorMessage(resp))
	}
	return nil
}
