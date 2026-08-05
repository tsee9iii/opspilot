package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleMemInfo = `MemTotal:       1024 kB
MemFree:          768 kB
MemAvailable:     256 kB
Buffers:          128 kB
Cached:           384 kB
`

func TestMemoryToolName(t *testing.T) {
	tool := NewMemoryTool()
	if tool.Name() != ToolSystemMemory {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestParseMemInfo(t *testing.T) {
	res, err := parseMemInfo(strings.NewReader(sampleMemInfo))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TotalBytes != 1024*1024 {
		t.Fatalf("unexpected total_bytes: %d", res.TotalBytes)
	}
	if res.AvailableBytes != 256*1024 {
		t.Fatalf("unexpected available_bytes: %d", res.AvailableBytes)
	}
	if res.UsedBytes != 768*1024 {
		t.Fatalf("unexpected used_bytes: %d", res.UsedBytes)
	}
	if res.UsedPercent != 75 {
		t.Fatalf("unexpected used_percent: %f", res.UsedPercent)
	}
}

func TestParseMemInfoMissingKeys(t *testing.T) {
	for _, content := range []string{
		"MemFree: 1 kB\n",
		"MemTotal: 1 kB\n",
		"",
	} {
		if _, err := parseMemInfo(strings.NewReader(content)); err == nil {
			t.Fatalf("expected error for content: %q", content)
		}
	}
}

func TestParseMemInfoZeroTotal(t *testing.T) {
	res, err := parseMemInfo(strings.NewReader("MemTotal: 0 kB\nMemAvailable: 0 kB\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedPercent != 0 {
		t.Fatalf("expected zero used_percent, got: %f", res.UsedPercent)
	}
}

func TestMemoryToolExecute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meminfo")
	if err := os.WriteFile(path, []byte(sampleMemInfo), 0o644); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}

	tool := NewMemoryTool()
	tool.memInfoPath = path

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res memoryResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.TotalBytes != 1024*1024 || res.AvailableBytes != 256*1024 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.UsedBytes != 768*1024 {
		t.Fatalf("unexpected used_bytes: %d", res.UsedBytes)
	}
}
