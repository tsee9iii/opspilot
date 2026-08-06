package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolDockerInspect            = "docker.inspect"
	toolDockerInspectVersion     = "1.0.0"
	toolDockerInspectDescription = "Understand the runtime configuration of a container"
)

const toolDockerInspectParameterSchema = `{
  "type": "object",
  "required": ["container"],
  "properties": {
    "container": {
      "type": "string",
      "description": "Docker container name or ID"
    }
  },
  "additionalProperties": false
}`

// dockerPortBindingRaw is one published binding of a container port.
type dockerPortBindingRaw struct {
	HostIp   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// dockerMountRaw is a single mount entry of the inspect document. Source is
// populated for bind mounts and tmpfs; Name for named volumes.
type dockerMountRaw struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

// dockerInspectRaw holds only the fields of the `docker inspect` document that
// docker.inspect exposes. Unused fields are never decoded.
type dockerInspectRaw struct {
	Id           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	Config       struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		Paused     bool   `json:"Paused"`
		Restarting bool   `json:"Restarting"`
		ExitCode   int    `json:"ExitCode"`
		Error      string `json:"Error"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		Ports    map[string][]dockerPortBindingRaw `json:"Ports"`
		Networks map[string]struct{}               `json:"Networks"`
	} `json:"NetworkSettings"`
	Mounts []dockerMountRaw `json:"Mounts"`
}

// dockerInspectResult is the operations-focused view of a container.
type dockerInspectResult struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Image        string        `json:"image"`
	State        string        `json:"state"`
	Status       string        `json:"status"`
	RestartCount int           `json:"restart_count"`
	Health       string        `json:"health"`
	StartedAt    string        `json:"started_at"`
	Ports        []dockerPort  `json:"ports"`
	Mounts       []dockerMount `json:"mounts"`
	Networks     []string      `json:"networks"`
}

type dockerPort struct {
	Container string `json:"container"`
	Host      string `json:"host,omitempty"`
}

type dockerMount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// DockerInspectTool reports the runtime configuration of exactly one container
// via `docker inspect`. It never enumerates containers, never reads the full
// inspect document, and never modifies Docker state.
type DockerInspectTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewDockerInspectTool() *DockerInspectTool {
	return &DockerInspectTool{run: agent.RunCommand}
}

func (t *DockerInspectTool) Name() string {
	return ToolDockerInspect
}

func (t *DockerInspectTool) Version() string {
	return toolDockerInspectVersion
}

func (t *DockerInspectTool) Description() string {
	return toolDockerInspectDescription
}

func (t *DockerInspectTool) ParameterSchema() string {
	return toolDockerInspectParameterSchema
}

func (t *DockerInspectTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *DockerInspectTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategoryDocker,
		Domain:               "container",
		Tags:                 []string{"docker", "container", "configuration", "runtime"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationShort,
		SinceVersion:         toolDockerInspectVersion,
	}
}

func (t *DockerInspectTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "docker")
}

func (t *DockerInspectTool) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	container, err := parseInspectRequest(payload)
	if err != nil {
		return nil, err
	}

	if err := dockerInstalled(ctx, t.run, "docker.inspect"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, dockerNotAvailableError()
		}
		return nil, inspectionFailedError(err.Error())
	}

	out, err := t.run(ctx, "docker", "inspect", container)
	if err != nil {
		return nil, inspectionFailedError(err.Error())
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, inspectionFailedError(fmt.Sprintf("decode command result: %v", err))
	}
	if res.ExitCode != 0 {
		switch {
		case dockerPermissionDenied(res.Stderr):
			return nil, dockerPermissionDeniedError()
		case dockerNoSuchObject(res.Stderr):
			return nil, containerNotFoundError(container)
		default:
			return nil, inspectionFailedError(strings.TrimSpace(res.Stderr))
		}
	}

	var raws []dockerInspectRaw
	if err := json.Unmarshal([]byte(res.Stdout), &raws); err != nil {
		return nil, inspectionFailedError(fmt.Sprintf("decode docker inspect output: %v", err))
	}
	if len(raws) == 0 {
		return nil, containerNotFoundError(container)
	}
	if len(raws) != 1 {
		return nil, inspectionFailedError(fmt.Sprintf("expected 1 container, got %d", len(raws)))
	}

	return json.Marshal(buildInspectResult(raws[0]))
}

func buildInspectResult(raw dockerInspectRaw) dockerInspectResult {
	startedAt := formatInspectTime(raw.State.StartedAt)
	return dockerInspectResult{
		ID:           raw.Id,
		Name:         strings.TrimPrefix(raw.Name, "/"),
		Image:        raw.Config.Image,
		State:        raw.State.Status,
		Status:       dockerStatus(raw),
		RestartCount: raw.RestartCount,
		Health:       inspectHealth(raw),
		StartedAt:    startedAt,
		Ports:        buildPorts(raw.NetworkSettings.Ports),
		Mounts:       buildMounts(raw.Mounts),
		Networks:     buildNetworks(raw.NetworkSettings.Networks),
	}
}

// inspectHealth reports the healthcheck status, or "none" when the container
// has no healthcheck state.
func inspectHealth(raw dockerInspectRaw) string {
	if raw.State.Health != nil {
		return raw.State.Health.Status
	}
	return "none"
}

// buildPorts flattens the NetworkSettings.Ports map into host-reachable port
// entries, sorted by container port. Ports that are exposed but not published
// are reported without a host binding.
func buildPorts(ports map[string][]dockerPortBindingRaw) []dockerPort {
	keys := make([]string, 0, len(ports))
	for key := range ports {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]dockerPort, 0, len(keys))
	for _, key := range keys {
		bindings := ports[key]
		if len(bindings) == 0 {
			out = append(out, dockerPort{Container: key})
			continue
		}
		for _, b := range bindings {
			host := b.HostPort
			if b.HostIp != "" {
				host = b.HostIp + ":" + b.HostPort
			}
			out = append(out, dockerPort{Container: key, Host: host})
		}
	}
	return out
}

// buildMounts maps the inspect mount list to source/destination pairs. Bind and
// volume mounts are exposed; tmpfs mounts are not. For named volumes the
// volume name is the source.
func buildMounts(mounts []dockerMountRaw) []dockerMount {
	out := make([]dockerMount, 0, len(mounts))
	for _, m := range mounts {
		if m.Type != "bind" && m.Type != "volume" {
			continue
		}
		source := m.Source
		if m.Type == "volume" && m.Name != "" {
			source = m.Name
		}
		out = append(out, dockerMount{Source: source, Destination: m.Destination})
	}
	return out
}

// buildNetworks returns the sorted names of the container's attached networks.
func buildNetworks(networks map[string]struct{}) []string {
	out := make([]string, 0, len(networks))
	for name := range networks {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// dockerStatus renders a human-readable status from the inspect state, mirroring
// the format of `docker ps` (without embedding the health check result, which
// is exposed separately).
func dockerStatus(raw dockerInspectRaw) string {
	state := raw.State
	now := time.Now()

	if state.Running {
		if state.Restarting {
			return fmt.Sprintf("Restarting (%d) %s ago", raw.RestartCount, humanDuration(now.Sub(parseInspectTime(state.StartedAt))))
		}
		return "Up " + humanDuration(now.Sub(parseInspectTime(state.StartedAt)))
	}
	if state.Paused {
		return "Up " + humanDuration(now.Sub(parseInspectTime(state.StartedAt))) + " (Paused)"
	}

	switch state.Status {
	case "created":
		return "Created"
	case "exited":
		dur := now.Sub(parseInspectTime(state.FinishedAt))
		if dur < 0 {
			dur = 0
		}
		if state.Error != "" {
			return fmt.Sprintf("Exited (%d) %s ago (%s)", state.ExitCode, humanDuration(dur), state.Error)
		}
		return fmt.Sprintf("Exited (%d) %s ago", state.ExitCode, humanDuration(dur))
	case "dead":
		return "Dead"
	default:
		if state.Status == "" {
			return "Status unknown"
		}
		return state.Status
	}
}

// humanDuration renders a duration in docker's human format.
func humanDuration(d time.Duration) string {
	if seconds := int(d.Seconds()); seconds < 1 {
		return "Less than a second"
	} else if seconds == 1 {
		return "1 second"
	} else if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	} else if minutes := int(d.Minutes()); minutes == 1 {
		return "About a minute"
	} else if minutes < 60 {
		return fmt.Sprintf("%d minutes", minutes)
	} else if hours := int(d.Hours() + 0.5); hours == 1 {
		return "About an hour"
	} else if hours < 48 {
		return fmt.Sprintf("%d hours", hours)
	} else if hours < 24*7*2 {
		return fmt.Sprintf("%d days", hours/24)
	} else if hours < 24*30*2 {
		return fmt.Sprintf("%d weeks", hours/24/7)
	} else if hours < 24*365*2 {
		return fmt.Sprintf("%d months", hours/24/30)
	}
	return fmt.Sprintf("%d years", int(d.Hours())/24/365)
}

// parseInspectTime parses a docker inspect timestamp (RFC3339). A zero or
// unparseable value yields the zero time.
func parseInspectTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// formatInspectTime renders a docker inspect timestamp as RFC3339, or "" when
// the value is missing or zero (e.g. a created container).
func formatInspectTime(raw string) string {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil || t.Year() <= 1 {
		return ""
	}
	return t.Format(time.RFC3339)
}

// dockerNoSuchObject reports whether docker stderr indicates the container does
// not exist.
func dockerNoSuchObject(stderr string) bool {
	return strings.Contains(stderr, "No such object")
}

func parseInspectRequest(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("docker.inspect: payload is required")
	}
	var req struct {
		Container string `json:"container"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", fmt.Errorf("docker.inspect: invalid payload: %w", err)
	}
	if req.Container == "" {
		return "", invalidContainerError()
	}
	return req.Container, nil
}

func invalidContainerError() error {
	return &agent.ToolError{
		Code:       "invalid_container",
		Message:    "container is required",
		Suggestion: "Provide a container name or ID.",
	}
}

func containerNotFoundError(idOrName string) error {
	return &agent.ToolError{
		Code:       "container_not_found",
		Message:    fmt.Sprintf("container not found: %s", idOrName),
		Suggestion: "Check the container name or ID, or run docker.ps to list containers.",
	}
}

func dockerNotAvailableError() error {
	return &agent.ToolError{
		Code:       "docker_not_available",
		Message:    "The docker CLI is not available on this agent.",
		Suggestion: "Install the docker CLI or make sure it is on PATH.",
	}
}

func dockerPermissionDeniedError() error {
	return &agent.ToolError{
		Code:       "docker_permission_denied",
		Message:    "The opspilot user is not permitted to reach the docker daemon.",
		Suggestion: "Run: sudo usermod -aG docker opspilot && restart the agent.",
	}
}

func inspectionFailedError(detail string) error {
	return &agent.ToolError{
		Code:       "inspection_failed",
		Message:    fmt.Sprintf("docker inspect failed: %s", detail),
		Suggestion: "Check that the docker daemon is reachable and healthy.",
	}
}
