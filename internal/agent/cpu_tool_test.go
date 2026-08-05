package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleCPUStat = `cpu  100 10 50 800 20 5 5 0 0 0
cpu0 50 5 25 400 10 2 2 0 0 0
cpu1 50 5 25 400 10 3 3 0 0 0
intr 1234567
ctxt 42
btime 1700000000
processes 7
procs_running 2
procs_blocked 1
`

func TestCPUToolName(t *testing.T) {
	tool := NewCPUTool()
	if tool.Name() != ToolSystemCPU {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestParseCPUStat(t *testing.T) {
	s, err := parseCPUStat(strings.NewReader(sampleCPUStat))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// user+nice = 110, system+irq+softirq = 60, idle+iowait = 820, total = 990
	if s.user != 110 {
		t.Fatalf("unexpected user: %d", s.user)
	}
	if s.system != 60 {
		t.Fatalf("unexpected system: %d", s.system)
	}
	if s.idle != 820 {
		t.Fatalf("unexpected idle: %d", s.idle)
	}
	if s.total != 990 {
		t.Fatalf("unexpected total: %d", s.total)
	}
}

func TestParseCPUStatMissingLine(t *testing.T) {
	if _, err := parseCPUStat(strings.NewReader("intr 123\nctxt 42\n")); err == nil {
		t.Fatal("expected error for missing cpu line")
	}
}

func TestParseCPUStatMalformed(t *testing.T) {
	if _, err := parseCPUStat(strings.NewReader("cpu notanumber 1 2 3 4 5 6 7\n")); err == nil {
		t.Fatal("expected error for malformed cpu line")
	}
}

func TestCPUFromDeltas(t *testing.T) {
	// user=550, system=300, idle=150, total=1000 -> 55 / 30 / 15
	res := cpuFromDeltas(cpuSample{user: 550, system: 300, idle: 150, total: 1000})
	if res.UserPercent != 55 {
		t.Fatalf("unexpected user_percent: %f", res.UserPercent)
	}
	if res.SystemPercent != 30 {
		t.Fatalf("unexpected system_percent: %f", res.SystemPercent)
	}
	if res.IdlePercent != 15 {
		t.Fatalf("unexpected idle_percent: %f", res.IdlePercent)
	}
}

func TestCPUFromDeltasZero(t *testing.T) {
	res := cpuFromDeltas(cpuSample{})
	if res.UserPercent != 0 || res.SystemPercent != 0 || res.IdlePercent != 0 {
		t.Fatalf("expected zero percentages, got: %+v", res)
	}
}

func TestCPUFromDeltasRounding(t *testing.T) {
	// user=100, system=100, idle=100, total=300 -> 33.33 / 33.33 / 33.33
	res := cpuFromDeltas(cpuSample{user: 100, system: 100, idle: 100, total: 300})
	if res.UserPercent != 33.33 || res.SystemPercent != 33.33 || res.IdlePercent != 33.33 {
		t.Fatalf("unexpected percentages: %+v", res)
	}
}

func TestCPUToolExecute(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "stat-a")
	second := filepath.Join(dir, "stat-b")

	mustWriteFile(t, first, sampleCPUStat)
	// Second sample advances only user ticks: user=110->160, total=990->1040
	mustWriteFile(t, second, strings.Replace(sampleCPUStat, "cpu  100 10 50 800 20 5 5 0 0 0", "cpu  150 10 50 800 20 5 5 0 0 0", 1))

	reads := 0
	tool := NewCPUTool()
	tool.delay = time.Millisecond
	tool.readStat = func(path string) (cpuSample, error) {
		reads++
		switch reads {
		case 1:
			return readCPUStat(first)
		case 2:
			return readCPUStat(second)
		default:
			t.Fatalf("unexpected number of reads: %d", reads)
			return cpuSample{}, nil
		}
	}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res cpuResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	// delta user=50, total=50 -> 100 / 0 / 0
	if res.UserPercent != 100 {
		t.Fatalf("unexpected user_percent: %f", res.UserPercent)
	}
	if res.SystemPercent != 0 || res.IdlePercent != 0 {
		t.Fatalf("unexpected percentages: %+v", res)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
