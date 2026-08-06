package docker

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
)

func TestDockerInspectToolMetadata(t *testing.T) {
	tool := NewDockerInspectTool()
	if tool.Name() != ToolDockerInspect {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Version() == "" || tool.Description() == "" {
		t.Fatal("missing tool metadata")
	}
	if tool.ConfirmationLevel() != agent.ConfirmationNone {
		t.Fatalf("unexpected confirmation level: %s", tool.ConfirmationLevel())
	}
}

func TestDockerInspectParameterSchema(t *testing.T) {
	tool := NewDockerInspectTool()
	var schema struct {
		Type       string `json:"type"`
		Required   []string
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tool.ParameterSchema()), &schema); err != nil {
		t.Fatalf("invalid parameter schema: %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("unexpected schema type: %s", schema.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "container" {
		t.Fatalf("unexpected required: %v", schema.Required)
	}
	if _, ok := schema.Properties["container"]; !ok {
		t.Fatal("missing container property")
	}
}

func TestParseInspectRequest(t *testing.T) {
	container, err := parseInspectRequest([]byte(`{"container":"merchant-api"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if container != "merchant-api" {
		t.Fatalf("unexpected container: %s", container)
	}
}

func TestParseInspectRequestInvalid(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"missing container", []byte(`{}`)},
		{"empty container", []byte(`{"container":""}`)},
		{"malformed json", []byte(`{`)},
	}
	for _, tc := range cases {
		_, err := parseInspectRequest(tc.payload)
		if err == nil {
			t.Fatalf("expected error for %s", tc.name)
		}
		if tc.name == "empty payload" || tc.name == "malformed json" {
			continue
		}
		var te *agent.ToolError
		if !errors.As(err, &te) || te.Code != "invalid_container" {
			t.Fatalf("expected invalid_container for %s, got: %v", tc.name, err)
		}
	}
}

func fakeDockerInspectRun(inspectJSON, inspectStderr string, inspectExit int) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "--version":
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Docker version 26.1.1\n", ExitCode: 0})
			return b, nil
		default:
			b, _ := json.Marshal(agent.CommandResult{Stdout: inspectJSON, Stderr: inspectStderr, ExitCode: inspectExit})
			return b, nil
		}
	}
}

func inspectDocument() string {
	now := time.Now().UTC()
	started := now.Add(-2 * time.Minute).Format(time.RFC3339)
	return `[
  {
    "Id": "abc123def456",
    "Name": "/merchant-api",
    "RestartCount": 2,
    "Config": {"Image": "merchant-api:latest"},
    "State": {
      "Status": "running",
      "Running": true,
      "Paused": false,
      "Restarting": false,
      "ExitCode": 0,
      "Error": "",
      "StartedAt": "` + started + `",
      "FinishedAt": "0001-01-01T00:00:00Z",
      "Health": {"Status": "healthy"}
    },
    "NetworkSettings": {
      "Ports": {
        "8080/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}],
        "9000/tcp": null
      },
      "Networks": {"bridge": {}, "app-net": {}}
    },
    "Mounts": [
      {"Type": "bind", "Source": "/data", "Destination": "/app/data"},
      {"Type": "volume", "Name": "pgdata", "Destination": "/var/lib/postgresql/data"},
      {"Type": "tmpfs", "Destination": "/dev/shm"}
    ]
  }
]`
}

func TestDockerInspectToolExecuteRunning(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(-2 * time.Minute).Format(time.RFC3339)

	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun(inspectDocument(), "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"merchant-api"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerInspectResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.ID != "abc123def456" {
		t.Fatalf("unexpected id: %s", res.ID)
	}
	if res.Name != "merchant-api" {
		t.Fatalf("unexpected name: %s", res.Name)
	}
	if res.Image != "merchant-api:latest" {
		t.Fatalf("unexpected image: %s", res.Image)
	}
	if res.State != "running" {
		t.Fatalf("unexpected state: %s", res.State)
	}
	if res.Status != "Up 2 minutes" {
		t.Fatalf("unexpected status: %q", res.Status)
	}
	if res.RestartCount != 2 {
		t.Fatalf("unexpected restart count: %d", res.RestartCount)
	}
	if res.Health != "healthy" {
		t.Fatalf("unexpected health: %s", res.Health)
	}
	if res.StartedAt != started {
		t.Fatalf("unexpected started_at: %s (want %s)", res.StartedAt, started)
	}
	if len(res.Ports) != 2 {
		t.Fatalf("expected 2 ports, got: %d", len(res.Ports))
	}
	if res.Ports[0].Container != "8080/tcp" || res.Ports[0].Host != "0.0.0.0:8080" {
		t.Fatalf("unexpected first port: %+v", res.Ports[0])
	}
	if res.Ports[1].Container != "9000/tcp" || res.Ports[1].Host != "" {
		t.Fatalf("unexpected second port: %+v", res.Ports[1])
	}
	if len(res.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got: %d", len(res.Mounts))
	}
	if res.Mounts[0].Source != "/data" || res.Mounts[0].Destination != "/app/data" {
		t.Fatalf("unexpected first mount: %+v", res.Mounts[0])
	}
	if res.Mounts[1].Source != "pgdata" || res.Mounts[1].Destination != "/var/lib/postgresql/data" {
		t.Fatalf("unexpected second mount: %+v", res.Mounts[1])
	}
	if len(res.Networks) != 2 || res.Networks[0] != "app-net" || res.Networks[1] != "bridge" {
		t.Fatalf("unexpected networks: %v", res.Networks)
	}
}

func TestDockerInspectToolExecuteByID(t *testing.T) {
	var inspected string
	tool := NewDockerInspectTool()
	tool.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			b, _ := json.Marshal(agent.CommandResult{Stdout: "Docker version 26.1.1\n", ExitCode: 0})
			return b, nil
		}
		inspected = strings.Join(args[1:], " ")
		b, _ := json.Marshal(agent.CommandResult{Stdout: inspectDocument(), ExitCode: 0})
		return b, nil
	}

	if _, err := tool.Execute(context.Background(), []byte(`{"container":"abc123def456"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inspected != "abc123def456" {
		t.Fatalf("expected inspect of the ID, got args: %q", inspected)
	}
}

func TestDockerInspectToolExecuteUnhealthy(t *testing.T) {
	doc := `[{"Id":"u1","Name":"/api","RestartCount":0,"Config":{"Image":"api:latest"},"State":{"Status":"running","Running":true,"StartedAt":"0001-01-01T00:00:00Z"},"NetworkSettings":{},"Mounts":[]}]`
	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun(doc, "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"api"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerInspectResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Health != "none" {
		t.Fatalf("expected no health state, got: %s", res.Health)
	}
}

func TestDockerInspectToolExecuteStopped(t *testing.T) {
	finished := time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339)
	doc := `[{"Id":"s1","Name":"/worker","RestartCount":0,"Config":{"Image":"worker:latest"},"State":{"Status":"exited","Running":false,"Paused":false,"Restarting":false,"ExitCode":137,"Error":"","StartedAt":"0001-01-01T00:00:00Z","FinishedAt":"` + finished + `"},"NetworkSettings":{},"Mounts":[]}]`

	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun(doc, "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"worker"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerInspectResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.State != "exited" {
		t.Fatalf("unexpected state: %s", res.State)
	}
	if res.Status != "Exited (137) 3 minutes ago" {
		t.Fatalf("unexpected status: %q", res.Status)
	}
	if res.StartedAt != "" {
		t.Fatalf("expected empty started_at, got: %q", res.StartedAt)
	}
}

func TestDockerInspectToolExecuteStoppedWithError(t *testing.T) {
	finished := time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339)
	doc := `[{"Id":"s2","Name":"/crashed","RestartCount":0,"Config":{"Image":"app:latest"},"State":{"Status":"exited","Running":false,"Paused":false,"Restarting":false,"ExitCode":1,"Error":"boom","StartedAt":"0001-01-01T00:00:00Z","FinishedAt":"` + finished + `"},"NetworkSettings":{},"Mounts":[]}]`

	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun(doc, "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"crashed"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerInspectResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Status != "Exited (1) 3 minutes ago (boom)" {
		t.Fatalf("unexpected status: %q", res.Status)
	}
}

func TestDockerInspectToolExecuteCreated(t *testing.T) {
	doc := `[{"Id":"c1","Name":"/fresh","RestartCount":0,"Config":{"Image":"nginx:latest"},"State":{"Status":"created","Running":false,"Paused":false,"Restarting":false,"ExitCode":0,"Error":"","StartedAt":"0001-01-01T00:00:00Z","FinishedAt":"0001-01-01T00:00:00Z"},"NetworkSettings":{},"Mounts":[]}]`

	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun(doc, "", 0)

	result, err := tool.Execute(context.Background(), []byte(`{"container":"fresh"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res dockerInspectResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.Status != "Created" {
		t.Fatalf("unexpected status: %q", res.Status)
	}
}

func TestDockerInspectToolExecuteContainerNotFound(t *testing.T) {
	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun("", "Error response from daemon: No such object: ghost\n", 1)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"ghost"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected a structured ToolError, got: %v", err)
	}
	if te.Code != "container_not_found" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
	if !strings.Contains(te.Message, "ghost") || te.Suggestion == "" {
		t.Fatalf("unexpected error details: %+v", te)
	}
}

func TestDockerInspectToolExecuteEmptyDocument(t *testing.T) {
	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun("[]", "", 0)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"ghost"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) || te.Code != "container_not_found" {
		t.Fatalf("expected container_not_found for empty document, got: %v", err)
	}
}

func TestDockerInspectToolExecutePermissionDenied(t *testing.T) {
	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun("", "Got permission denied while trying to connect to the Docker daemon socket\n", 1)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected a structured ToolError, got: %v", err)
	}
	if te.Code != "docker_permission_denied" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
	if !strings.Contains(te.Suggestion, "usermod") {
		t.Fatalf("expected actionable suggestion: %s", te.Suggestion)
	}
}

func TestDockerInspectToolExecuteNotInstalled(t *testing.T) {
	tool := NewDockerInspectTool()
	tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, exec.ErrNotFound
	}

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected a structured ToolError, got: %v", err)
	}
	if te.Code != "docker_not_available" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
	if !strings.Contains(te.Suggestion, "PATH") {
		t.Fatalf("expected actionable suggestion: %s", te.Suggestion)
	}
}

func TestDockerInspectToolExecuteInvalidJSON(t *testing.T) {
	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun("garbage", "", 0)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected a structured ToolError, got: %v", err)
	}
	if te.Code != "inspection_failed" {
		t.Fatalf("unexpected error code: %s", te.Code)
	}
	if !strings.Contains(te.Message, "decode") {
		t.Fatalf("unexpected error message: %s", te.Message)
	}
}

func TestDockerInspectToolExecuteMultipleContainers(t *testing.T) {
	doc := `[{"Id":"a"},{"Id":"b"}]`
	tool := NewDockerInspectTool()
	tool.run = fakeDockerInspectRun(doc, "", 0)

	_, err := tool.Execute(context.Background(), []byte(`{"container":"web"}`))
	var te *agent.ToolError
	if !errors.As(err, &te) || te.Code != "inspection_failed" {
		t.Fatalf("expected inspection_failed for multiple containers, got: %v", err)
	}
}

func TestDockerInspectToolAvailability(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		tool := NewDockerInspectTool()
		var binary string
		tool.run = func(_ context.Context, bin string, _ ...string) ([]byte, error) {
			binary = bin
			out, _ := json.Marshal(agent.CommandResult{ExitCode: 0})
			return out, nil
		}
		ok, reason := tool.Availability(context.Background())
		if !ok || reason != "" {
			t.Fatalf("expected available, got ok=%v reason=%q", ok, reason)
		}
		if binary != "docker" {
			t.Fatalf("expected check of docker binary, got %q", binary)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		tool := NewDockerInspectTool()
		tool.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		ok, reason := tool.Availability(context.Background())
		if ok || reason != "docker is not installed" {
			t.Fatalf("expected unavailable, got ok=%v reason=%q", ok, reason)
		}
	})
}

func TestBuildPorts(t *testing.T) {
	ports := buildPorts(map[string][]dockerPortBindingRaw{
		"9000/tcp": nil,
		"80/tcp":   {{HostIp: "0.0.0.0", HostPort: "8080"}},
		"443/tcp":  {{HostIp: "", HostPort: "443"}},
	})
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got: %d", len(ports))
	}
	if ports[0].Container != "443/tcp" || ports[0].Host != "443" {
		t.Fatalf("unexpected first port: %+v", ports[0])
	}
	if ports[1].Container != "80/tcp" || ports[1].Host != "0.0.0.0:8080" {
		t.Fatalf("unexpected second port: %+v", ports[1])
	}
	if ports[2].Container != "9000/tcp" || ports[2].Host != "" {
		t.Fatalf("unexpected third port: %+v", ports[2])
	}
}

func TestBuildPortsEmpty(t *testing.T) {
	ports := buildPorts(nil)
	if ports == nil || len(ports) != 0 {
		t.Fatalf("expected empty non-nil ports, got: %v", ports)
	}
}

func TestBuildMounts(t *testing.T) {
	mounts := buildMounts([]dockerMountRaw{
		{Type: "bind", Source: "/data", Destination: "/app/data"},
		{Type: "volume", Name: "pgdata", Source: "/var/lib/docker/volumes/pgdata/_data", Destination: "/var/lib/postgresql/data"},
		{Type: "tmpfs", Destination: "/dev/shm"},
	})
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got: %d", len(mounts))
	}
	if mounts[0].Source != "/data" || mounts[0].Destination != "/app/data" {
		t.Fatalf("unexpected first mount: %+v", mounts[0])
	}
	if mounts[1].Source != "pgdata" || mounts[1].Destination != "/var/lib/postgresql/data" {
		t.Fatalf("unexpected second mount: %+v", mounts[1])
	}
}

func TestBuildNetworks(t *testing.T) {
	networks := buildNetworks(map[string]struct{}{"bridge": {}, "app-net": {}})
	if len(networks) != 2 || networks[0] != "app-net" || networks[1] != "bridge" {
		t.Fatalf("unexpected networks: %v", networks)
	}
}

func TestDockerStatusBranches(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		raw  dockerInspectRaw
		want string
	}{
		{
			name: "running",
			raw: dockerInspectRaw{State: struct {
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
			}{Status: "running", Running: true, StartedAt: now.Add(-2 * time.Minute).Format(time.RFC3339)}},
			want: "Up 2 minutes",
		},
		{
			name: "restarting",
			raw: dockerInspectRaw{RestartCount: 3, State: struct {
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
			}{Status: "restarting", Running: true, Restarting: true, StartedAt: now.Add(-1 * time.Minute).Format(time.RFC3339)}},
			want: "Restarting (3) About a minute ago",
		},
		{
			name: "paused",
			raw: dockerInspectRaw{State: struct {
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
			}{Status: "paused", Running: false, Paused: true, StartedAt: now.Add(-2 * time.Minute).Format(time.RFC3339)}},
			want: "Up 2 minutes (Paused)",
		},
		{
			name: "created",
			raw: dockerInspectRaw{State: struct {
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
			}{Status: "created"}},
			want: "Created",
		},
		{
			name: "exited with error",
			raw: dockerInspectRaw{State: struct {
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
			}{Status: "exited", ExitCode: 1, Error: "boom", FinishedAt: now.Add(-3 * time.Minute).Format(time.RFC3339)}},
			want: "Exited (1) 3 minutes ago (boom)",
		},
		{
			name: "unknown",
			raw: dockerInspectRaw{State: struct {
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
			}{Status: ""}},
			want: "Status unknown",
		},
	}
	for _, tc := range cases {
		if got := dockerStatus(tc.raw); got != tc.want {
			t.Fatalf("%s: dockerStatus = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		seconds int
		want    string
	}{
		{0, "Less than a second"},
		{1, "1 second"},
		{30, "30 seconds"},
		{60, "About a minute"},
		{5 * 60, "5 minutes"},
		{60 * 60, "About an hour"},
		{3 * 60 * 60, "3 hours"},
		{48 * 60 * 60, "2 days"},
		{10 * 24 * 60 * 60, "10 days"},
		{30 * 24 * 60 * 60, "4 weeks"},
		{90 * 24 * 60 * 60, "3 months"},
		{730 * 24 * 60 * 60, "2 years"},
	}
	for _, tc := range cases {
		if got := humanDuration(time.Duration(tc.seconds) * time.Second); got != tc.want {
			t.Fatalf("humanDuration(%d) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}
