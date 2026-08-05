package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type procTreeSpec struct {
	stat   string
	comm   string
	status string
}

func procStatLine(pid int, comm string, utime, stime int) string {
	return fmt.Sprintf("%d (%s) S 0 %d 1 0 -1 4194560 100 0 0 0 %d %d 0 0 20 0 1 0", pid, comm, pid, utime, stime)
}

func TestProcessesToolName(t *testing.T) {
	tool := NewProcessesTool()
	if tool.Name() != ToolSystemProcesses {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestParseProcStat(t *testing.T) {
	ticks, err := parseProcStat([]byte("1 (init) S 0 1 1 0 -1 4194560 100 0 0 0 100 50 0 0 20 0 1 0"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticks != 150 {
		t.Fatalf("unexpected ticks: %d", ticks)
	}
}

func TestParseProcStatCommWithSpaces(t *testing.T) {
	ticks, err := parseProcStat([]byte("42 (web (worker) pool) S 0 42 1 0 -1 4194560 100 0 0 0 50 25 0 0 20 0 1 0"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticks != 75 {
		t.Fatalf("unexpected ticks: %d", ticks)
	}
}

func TestParseProcStatMissingTerminator(t *testing.T) {
	if _, err := parseProcStat([]byte("no parens here")); err == nil {
		t.Fatal("expected error for missing comm terminator")
	}
}

func TestParseProcStatus(t *testing.T) {
	data := "Name: init\nState: S (sleeping)\nVmSize: 8000 kB\nVmRSS: 4096 kB\n"
	bytes, err := parseProcStatus([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes != 4096*1024 {
		t.Fatalf("unexpected memory bytes: %d", bytes)
	}
}

func TestParseProcStatusMissingKey(t *testing.T) {
	if _, err := parseProcStatus([]byte("Name: init\nVmSize: 8000 kB\n")); err == nil {
		t.Fatal("expected error for missing VmRSS")
	}
}

func TestParseSystemStat(t *testing.T) {
	data := "cpu  100 10 50 8000 20 5 5 0 0 0\n" +
		"cpu0 100 10 50 8000 20 5 5 0 0 0\n" +
		"cpu1 100 10 50 8000 20 5 5 0 0 0\n" +
		"intr 1234567\n"
	total, cpus, err := parseSystemStat([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 8190 {
		t.Fatalf("unexpected total ticks: %d", total)
	}
	if cpus != 2 {
		t.Fatalf("unexpected cpus: %d", cpus)
	}
}

func TestParseSystemStatMissingCPU(t *testing.T) {
	if _, _, err := parseSystemStat([]byte("intr 123\n")); err == nil {
		t.Fatal("expected error for missing cpu line")
	}
}

func writeProcTree(t *testing.T, root, sysStat string, procs map[string]procTreeSpec) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	if err := os.WriteFile(filepath.Join(root, "stat"), []byte(sysStat), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	for pid, spec := range procs {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		files := map[string]string{"stat": spec.stat, "comm": spec.comm, "status": spec.status}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatalf("write %s/%s: %v", pid, name, err)
			}
		}
	}
}

const testProcStatus = "Name: test\nState: S (sleeping)\nVmSize: 8000 kB\nVmRSS: 4096 kB\n"

func TestSampleProcFS(t *testing.T) {
	root := t.TempDir()
	sysStat := "cpu  100 10 50 8000 20 5 5 0 0 0\ncpu0 100 10 50 8000 20 5 5 0 0 0\nintr 1\n"
	writeProcTree(t, root, sysStat, map[string]procTreeSpec{
		"1": {stat: procStatLine(1, "init", 100, 50), comm: "init\n", status: testProcStatus},
		"2": {stat: procStatLine(2, "worker", 10, 20), comm: "worker\n", status: testProcStatus},
		"x": {stat: "ignored", comm: "ignored\n", status: "ignored\n"},
	})

	fs, err := sampleProcFS(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fs.totalTicks != 8190 {
		t.Fatalf("unexpected total ticks: %d", fs.totalTicks)
	}
	if fs.cpus != 1 {
		t.Fatalf("unexpected cpus: %d", fs.cpus)
	}
	if len(fs.procs) != 2 {
		t.Fatalf("expected 2 processes, got: %d", len(fs.procs))
	}
	if got := fs.procs[1]; got.ticks != 150 || got.name != "init" || got.memoryBytes != 4096*1024 {
		t.Fatalf("unexpected proc 1: %+v", got)
	}
}

func TestComputeTopProcesses(t *testing.T) {
	first := procFS{
		totalTicks: 1000,
		cpus:       2,
		procs: map[int]processInfo{
			1: {pid: 1, name: "low", ticks: 0, memoryBytes: 100},
			2: {pid: 2, name: "high", ticks: 0, memoryBytes: 200},
		},
	}
	second := procFS{
		totalTicks: 2000,
		cpus:       2,
		procs: map[int]processInfo{
			1: {pid: 1, name: "low", ticks: 100, memoryBytes: 100},
			2: {pid: 2, name: "high", ticks: 250, memoryBytes: 200},
			3: {pid: 3, name: "new", ticks: 500, memoryBytes: 300},
		},
	}

	results := computeTopProcesses(first, second)

	// deltaTicks=1000, cpus=2: pid1 delta=100 -> 20%, pid2 delta=250 -> 50%.
	// pid3 only exists in the second sample and is skipped.
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got: %d", len(results))
	}
	if results[0].PID != 2 || results[0].CPUPercent != 50 {
		t.Fatalf("expected pid 2 first at 50%%, got: %+v", results[0])
	}
	if results[1].PID != 1 || results[1].CPUPercent != 20 {
		t.Fatalf("expected pid 1 second at 20%%, got: %+v", results[1])
	}
	if results[0].MemoryBytes != 200 {
		t.Fatalf("unexpected memory_bytes: %d", results[0].MemoryBytes)
	}
}

func TestComputeTopProcessesLimitAndTieBreak(t *testing.T) {
	first := procFS{totalTicks: 1000, cpus: 1, procs: map[int]processInfo{}}
	second := procFS{totalTicks: 2000, cpus: 1, procs: map[int]processInfo{}}
	for pid := 1; pid <= 12; pid++ {
		first.procs[pid] = processInfo{pid: pid, name: "p", ticks: 0, memoryBytes: int64(pid)}
		// pid 12 uses the most CPU: delta = pid*10 -> 12 is the busiest.
		second.procs[pid] = processInfo{pid: pid, name: "p", ticks: uint64(pid * 10), memoryBytes: int64(pid)}
	}

	results := computeTopProcesses(first, second)
	if len(results) != 10 {
		t.Fatalf("expected top 10, got: %d", len(results))
	}
	if results[0].PID != 12 {
		t.Fatalf("expected pid 12 on top, got: %d", results[0].PID)
	}
	if results[9].PID != 3 {
		t.Fatalf("expected pid 3 at the tail, got: %d", results[9].PID)
	}
}

func TestProcessesToolExecute(t *testing.T) {
	sysFirst := "cpu  100 10 50 8000 20 5 5 0 0 0\ncpu0 100 10 50 8000 20 5 5 0 0 0\nintr 1\n"
	sysSecond := "cpu  290 10 50 8000 20 5 5 0 0 0\ncpu0 290 10 50 8000 20 5 5 0 0 0\nintr 1\n"

	firstRoot := t.TempDir()
	secondRoot := t.TempDir()

	writeProcTree(t, firstRoot, sysFirst, map[string]procTreeSpec{
		"1": {stat: procStatLine(1, "init", 0, 0), comm: "init\n", status: testProcStatus},
	})
	writeProcTree(t, secondRoot, sysSecond, map[string]procTreeSpec{
		"1": {stat: procStatLine(1, "init", 190, 0), comm: "init\n", status: testProcStatus},
	})

	calls := 0
	tool := NewProcessesTool()
	tool.delay = time.Millisecond
	tool.sample = func(string) (procFS, error) {
		calls++
		if calls == 1 {
			return sampleProcFS(firstRoot)
		}
		return sampleProcFS(secondRoot)
	}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []processResult
	if err := json.Unmarshal(result, &results); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got: %d", len(results))
	}
	res := results[0]
	if res.PID != 1 || res.Name != "init" {
		t.Fatalf("unexpected process: %+v", res)
	}
	if res.CPUPercent != 100 {
		t.Fatalf("unexpected cpu_percent: %f", res.CPUPercent)
	}
	if res.MemoryBytes != 4096*1024 {
		t.Fatalf("unexpected memory_bytes: %d", res.MemoryBytes)
	}
}
