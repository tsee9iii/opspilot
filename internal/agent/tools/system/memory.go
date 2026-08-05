package system

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/opspilot/opspilot/internal/agent"
)

const (
	ToolSystemMemory      = "system.memory"
	toolMemoryVersion     = "1.0.0"
	toolMemoryDescription = "Report system memory usage"

	memInfoPath = "/proc/meminfo"
	kilobyte    = 1024
)

type memoryResult struct {
	TotalBytes     int64   `json:"total_bytes"`
	AvailableBytes int64   `json:"available_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

// MemoryTool reports Linux memory usage from /proc/meminfo.
type MemoryTool struct {
	memInfoPath string
}

func NewMemoryTool() *MemoryTool {
	return &MemoryTool{memInfoPath: memInfoPath}
}

func (t *MemoryTool) Name() string {
	return ToolSystemMemory
}

func (t *MemoryTool) Version() string {
	return toolMemoryVersion
}

func (t *MemoryTool) Description() string {
	return toolMemoryDescription
}

func (t *MemoryTool) ParameterSchema() string {
	return agent.EmptyParameterSchema
}

func (t *MemoryTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *MemoryTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	f, err := os.Open(t.memInfoPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", t.memInfoPath, err)
	}
	defer f.Close()

	res, err := parseMemInfo(f)
	if err != nil {
		return nil, err
	}
	return json.Marshal(res)
}

// parseMemInfo reads /proc/meminfo key-value lines ("Key: N kB") and returns
// the memory breakdown in bytes. used is derived as total minus available.
func parseMemInfo(r io.Reader) (memoryResult, error) {
	values := make(map[string]int64)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, rest, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return memoryResult{}, fmt.Errorf("read meminfo: %w", err)
	}

	totalKB, ok := values["MemTotal"]
	if !ok {
		return memoryResult{}, fmt.Errorf("parse meminfo: MemTotal not found")
	}
	availableKB, ok := values["MemAvailable"]
	if !ok {
		return memoryResult{}, fmt.Errorf("parse meminfo: MemAvailable not found")
	}

	total := totalKB * kilobyte
	available := availableKB * kilobyte
	used := total - available

	usedPercent := 0.0
	if total > 0 {
		usedPercent = math.Round(float64(used)/float64(total)*10000) / 100
	}

	return memoryResult{
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
		UsedPercent:    usedPercent,
	}, nil
}
