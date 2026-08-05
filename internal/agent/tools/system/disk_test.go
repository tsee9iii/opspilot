package system

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDiskToolName(t *testing.T) {
	tool := NewDiskTool()
	if tool.Name() != ToolSystemDisk {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
}

func TestDiskFromStat(t *testing.T) {
	res := diskFromStat(diskStat{total: 1024 * 1024, used: 768 * 1024, available: 256 * 1024})
	if res.TotalBytes != 1024*1024 {
		t.Fatalf("unexpected total_bytes: %d", res.TotalBytes)
	}
	if res.UsedBytes != 768*1024 {
		t.Fatalf("unexpected used_bytes: %d", res.UsedBytes)
	}
	if res.AvailableBytes != 256*1024 {
		t.Fatalf("unexpected available_bytes: %d", res.AvailableBytes)
	}
	if res.UsedPercent != 75 {
		t.Fatalf("unexpected used_percent: %f", res.UsedPercent)
	}
}

func TestDiskFromStatZero(t *testing.T) {
	res := diskFromStat(diskStat{})
	if res.UsedPercent != 0 {
		t.Fatalf("expected zero used_percent, got: %f", res.UsedPercent)
	}
}

func TestDiskFromStatRounding(t *testing.T) {
	res := diskFromStat(diskStat{total: 300, used: 100})
	if res.UsedPercent != 33.33 {
		t.Fatalf("unexpected used_percent: %f", res.UsedPercent)
	}
}

func TestDiskToolExecute(t *testing.T) {
	tool := NewDiskTool()
	tool.statfs = func(string) (diskStat, error) {
		return diskStat{total: 1024 * 1024, used: 768 * 1024, available: 256 * 1024}, nil
	}

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res diskResult
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if res.TotalBytes != 1024*1024 || res.UsedBytes != 768*1024 || res.AvailableBytes != 256*1024 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.UsedPercent != 75 {
		t.Fatalf("unexpected used_percent: %f", res.UsedPercent)
	}
}

func TestDiskToolExecuteError(t *testing.T) {
	tool := NewDiskTool()
	tool.statfs = func(string) (diskStat, error) {
		return diskStat{}, errors.New("statfs boom")
	}

	if _, err := tool.Execute(context.Background(), nil); err == nil {
		t.Fatal("expected error from statfs")
	}
}
