package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/opspilot/opspilot/internal/agent"
)

// dockerInstalled verifies the docker CLI is available on PATH.
func dockerInstalled(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool string) error {
	out, err := run(ctx, "docker", "--version")
	if err != nil {
		return wrapDockerError(tool, err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return fmt.Errorf("%s: decode command result: %w", tool, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s: docker --version failed: %s", tool, res.Stderr)
	}
	return nil
}

// psContainers lists running containers via `docker ps --format '{{json .}}'`,
// reusing parseDockerPS for all docker tools.
func psContainers(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool string) ([]dockerContainer, error) {
	out, err := run(ctx, "docker", "ps", "--format", `{{json .}}`)
	if err != nil {
		return nil, wrapDockerError(tool, err)
	}
	var res agent.CommandResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("%s: decode command result: %w", tool, err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("%s: docker ps failed: %s", tool, res.Stderr)
	}
	containers, err := parseDockerPS(res.Stdout)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tool, err)
	}
	return containers, nil
}

// containerExists verifies a running container matching idOrName exists.
func containerExists(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), tool, idOrName string) error {
	containers, err := psContainers(ctx, run, tool)
	if err != nil {
		return err
	}
	for _, c := range containers {
		if containerMatches(c, idOrName) {
			return nil
		}
	}
	return fmt.Errorf("%s: container not found: %s", tool, idOrName)
}

func wrapDockerError(tool string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s: docker is not installed: %w", tool, err)
	}
	return fmt.Errorf("%s: %w", tool, err)
}
