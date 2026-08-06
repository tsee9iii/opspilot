package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolDockerPs            = "docker.ps"
	toolDockerPsVersion     = "1.0.0"
	toolDockerPsDescription = "List running Docker containers"
)

// dockerPsRaw is a single line of `docker ps --format '{{json .}}'`.
type dockerPsRaw struct {
	ID     string   `json:"ID"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
	Ports  string   `json:"Ports"`
}

// dockerContainer is one running container reported by docker.ps.
type dockerContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

type dockerPsResponse struct {
	Containers []dockerContainer `json:"containers"`
}

// DockerPsTool reports running Docker containers via `docker ps`.
type DockerPsTool struct {
	run func(context.Context, string, ...string) ([]byte, error)
}

func NewDockerPsTool() *DockerPsTool {
	return &DockerPsTool{run: agent.RunCommand}
}

func (t *DockerPsTool) Name() string {
	return ToolDockerPs
}

func (t *DockerPsTool) Version() string {
	return toolDockerPsVersion
}

func (t *DockerPsTool) Description() string {
	return toolDockerPsDescription
}

func (t *DockerPsTool) ParameterSchema() string {
	return agent.EmptyParameterSchema
}

func (t *DockerPsTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *DockerPsTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategoryDocker,
		Domain:               "container",
		Tags:                 []string{"docker", "container", "runtime"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationShort,
		SinceVersion:         toolDockerPsVersion,
	}
}

func (t *DockerPsTool) Availability(ctx context.Context) (bool, string) {
	return agent.BinaryAvailable(ctx, t.run, "docker")
}

func (t *DockerPsTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	containers, err := psContainers(ctx, t.run, "docker.ps")
	if err != nil {
		return nil, err
	}
	return json.Marshal(dockerPsResponse{Containers: containers})
}

// parseDockerPS parses the JSON lines emitted by `docker ps --format
// '{{json .}}'`. An empty output yields an empty (non-nil) container list.
func parseDockerPS(stdout string) ([]dockerContainer, error) {
	containers := make([]dockerContainer, 0, 4)
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw dockerPsRaw
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("invalid JSON line: %w", err)
		}
		containers = append(containers, dockerContainer{
			ID:     raw.ID,
			Name:   strings.Join(raw.Names, ", "),
			Image:  raw.Image,
			State:  raw.State,
			Status: raw.Status,
			Ports:  raw.Ports,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stdout: %w", err)
	}
	return containers, nil
}
