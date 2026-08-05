package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolSystemProcesses      = "system.processes"
	toolProcessesVersion     = "1.0.0"
	toolProcessesDescription = "Report top processes by CPU usage"

	procPath           = "/proc"
	processSampleDelay = 200 * time.Millisecond
	topProcesses       = 10
)

type processResult struct {
	PID         int     `json:"pid"`
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// processInfo holds the fields sampled for one process.
type processInfo struct {
	pid         int
	name        string
	ticks       uint64
	memoryBytes int64
}

// procFS is a snapshot of system CPU ticks and per-process info.
type procFS struct {
	totalTicks uint64
	cpus       int
	procs      map[int]processInfo
}

// ProcessesTool reports the top processes by CPU usage, sampled twice so CPU
// percent reflects utilization in the sampling interval.
type ProcessesTool struct {
	procPath string
	delay    time.Duration
	sample   func(string) (procFS, error)
}

func NewProcessesTool() *ProcessesTool {
	return &ProcessesTool{
		procPath: procPath,
		delay:    processSampleDelay,
		sample:   sampleProcFS,
	}
}

func (t *ProcessesTool) Name() string {
	return ToolSystemProcesses
}

func (t *ProcessesTool) Version() string {
	return toolProcessesVersion
}

func (t *ProcessesTool) Description() string {
	return toolProcessesDescription
}

func (t *ProcessesTool) ParameterSchema() string {
	return agent.EmptyParameterSchema
}

func (t *ProcessesTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *ProcessesTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	first, err := t.sample(t.procPath)
	if err != nil {
		return nil, err
	}

	if err := sleepCtx(ctx, t.delay); err != nil {
		return nil, err
	}

	second, err := t.sample(t.procPath)
	if err != nil {
		return nil, err
	}

	return json.Marshal(computeTopProcesses(first, second))
}

// sampleProcFS snapshots the system CPU ticks and all readable processes
// under root (default /proc). Processes that cannot be read are skipped.
func sampleProcFS(root string) (procFS, error) {
	fs := procFS{procs: make(map[int]processInfo)}

	stat, err := os.ReadFile(filepath.Join(root, "stat"))
	if err != nil {
		return procFS{}, fmt.Errorf("read %s: %w", filepath.Join(root, "stat"), err)
	}
	totalTicks, cpus, err := parseSystemStat(stat)
	if err != nil {
		return procFS{}, err
	}
	fs.totalTicks = totalTicks
	fs.cpus = cpus

	entries, err := os.ReadDir(root)
	if err != nil {
		return procFS{}, fmt.Errorf("read %s: %w", root, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		info, err := readProcess(root, pid)
		if err != nil {
			continue
		}
		fs.procs[pid] = info
	}

	return fs, nil
}

// readProcess reads a process's name, CPU ticks and resident memory from
// root/<pid>/{comm,stat,status}.
func readProcess(root string, pid int) (processInfo, error) {
	base := filepath.Join(root, strconv.Itoa(pid))

	stat, err := os.ReadFile(filepath.Join(base, "stat"))
	if err != nil {
		return processInfo{}, err
	}
	ticks, err := parseProcStat(stat)
	if err != nil {
		return processInfo{}, err
	}

	name, err := readProcName(filepath.Join(base, "comm"))
	if err != nil {
		return processInfo{}, err
	}

	status, err := os.ReadFile(filepath.Join(base, "status"))
	if err != nil {
		return processInfo{}, err
	}
	memoryBytes, err := parseProcStatus(status)
	if err != nil {
		return processInfo{}, err
	}

	return processInfo{pid: pid, name: name, ticks: ticks, memoryBytes: memoryBytes}, nil
}

// parseProcStat extracts utime+stime (fields 14 and 15) from a /proc/<pid>/stat
// line. The comm field is delimited by the last ')' so it may contain spaces
// and parens.
func parseProcStat(data []byte) (uint64, error) {
	closeParen := strings.LastIndex(string(data), ")")
	if closeParen < 0 {
		return 0, fmt.Errorf("parse proc stat: no comm terminator")
	}
	fields := strings.Fields(string(data[closeParen+1:]))
	if len(fields) < 13 {
		return 0, fmt.Errorf("parse proc stat: too few fields")
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse proc stat utime: %w", err)
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse proc stat stime: %w", err)
	}
	return utime + stime, nil
}

func readProcName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// parseProcStatus extracts VmRSS (in kB) from a /proc/<pid>/status file.
func parseProcStatus(data []byte) (int64, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		key, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok || key != "VmRSS" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, fmt.Errorf("parse proc status: malformed VmRSS line")
		}
		kb, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse proc status VmRSS: %w", err)
		}
		return kb * kilobyte, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("parse proc status: %w", err)
	}
	return 0, fmt.Errorf("parse proc status: VmRSS not found")
}

// parseSystemStat reads the aggregate "cpu" total ticks and counts the "cpuN"
// logical processors from a /proc/stat file.
func parseSystemStat(data []byte) (totalTicks uint64, cpus int, err error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "cpu "):
			fields := strings.Fields(line)
			if len(fields) < 9 {
				return 0, 0, fmt.Errorf("parse system stat: too few cpu fields")
			}
			for _, f := range fields[1:9] {
				n, perr := strconv.ParseUint(f, 10, 64)
				if perr != nil {
					return 0, 0, fmt.Errorf("parse system stat value %q: %w", f, perr)
				}
				totalTicks += n
			}
		case strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9':
			cpus++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("read system stat: %w", err)
	}
	if totalTicks == 0 {
		return 0, 0, fmt.Errorf("parse system stat: cpu line not found")
	}
	return totalTicks, cpus, nil
}

// computeTopProcesses turns two snapshots into per-process CPU percentages
// (fraction of a single core, like top) and returns the top 10 by usage.
func computeTopProcesses(first, second procFS) []processResult {
	deltaTicks := second.totalTicks - first.totalTicks
	cpus := second.cpus
	if cpus == 0 {
		cpus = 1
	}

	var results []processResult
	for pid, cur := range second.procs {
		prev, ok := first.procs[pid]
		if !ok {
			continue
		}
		delta := cur.ticks - prev.ticks
		cpuPercent := 0.0
		if deltaTicks > 0 {
			cpuPercent = roundPercent(float64(delta) * float64(cpus) / float64(deltaTicks))
		}
		results = append(results, processResult{
			PID:         pid,
			Name:        cur.name,
			CPUPercent:  cpuPercent,
			MemoryBytes: cur.memoryBytes,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].CPUPercent != results[j].CPUPercent {
			return results[i].CPUPercent > results[j].CPUPercent
		}
		return results[i].PID < results[j].PID
	})
	if len(results) > topProcesses {
		results = results[:topProcesses]
	}
	return results
}
