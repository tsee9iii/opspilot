package agent

import (
	"context"
	"encoding/json"
)

const (
	ToolSystemDisk      = "system.disk"
	toolDiskVersion     = "1.0.0"
	toolDiskDescription = "Report root filesystem disk usage"
)

type diskResult struct {
	TotalBytes     int64   `json:"total_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	AvailableBytes int64   `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

// diskStat mirrors the statfs(2) fields the tool reads, decoupled from the
// platform syscall so the math is testable everywhere.
type diskStat struct {
	total     int64
	used      int64
	available int64
}

// DiskTool reports disk usage for the root filesystem via statfs(2).
type DiskTool struct {
	rootPath string
	statfs   func(string) (diskStat, error)
}

func NewDiskTool() *DiskTool {
	return &DiskTool{
		rootPath: "/",
		statfs:   statfsRoot,
	}
}

func (t *DiskTool) Name() string {
	return ToolSystemDisk
}

func (t *DiskTool) Version() string {
	return toolDiskVersion
}

func (t *DiskTool) Description() string {
	return toolDiskDescription
}

func (t *DiskTool) ParameterSchema() string {
	return toolEmptyParameterSchema
}

func (t *DiskTool) Execute(_ context.Context, _ []byte) ([]byte, error) {
	stat, err := t.statfs(t.rootPath)
	if err != nil {
		return nil, err
	}
	return json.Marshal(diskFromStat(stat))
}

// diskFromStat converts statfs fields into the reported result. used_percent
// is used over total, rounded to two decimals.
func diskFromStat(s diskStat) diskResult {
	usedPercent := 0.0
	if s.total > 0 {
		usedPercent = roundPercent(float64(s.used) / float64(s.total))
	}
	return diskResult{
		TotalBytes:     s.total,
		UsedBytes:      s.used,
		AvailableBytes: s.available,
		UsedPercent:    usedPercent,
	}
}
