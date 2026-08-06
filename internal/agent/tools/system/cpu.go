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
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
)

const (
	ToolSystemCPU      = "system.cpu"
	toolCPUVersion     = "1.0.0"
	toolCPUDescription = "Report system CPU usage"

	cpuStatPath    = "/proc/stat"
	cpuSampleDelay = 200 * time.Millisecond
)

type cpuResult struct {
	UserPercent   float64 `json:"user_percent"`
	SystemPercent float64 `json:"system_percent"`
	IdlePercent   float64 `json:"idle_percent"`
}

// cpuSample holds cumulative CPU ticks since boot for the aggregate "cpu" line.
type cpuSample struct {
	user   uint64
	system uint64
	idle   uint64
	total  uint64
}

func (s cpuSample) sub(o cpuSample) cpuSample {
	return cpuSample{
		user:   s.user - o.user,
		system: s.system - o.system,
		idle:   s.idle - o.idle,
		total:  s.total - o.total,
	}
}

// CPUTool reports Linux CPU usage from /proc/stat using two samples separated
// by a short delay, so percentages reflect utilization in that interval.
type CPUTool struct {
	statPath string
	delay    time.Duration
	readStat func(string) (cpuSample, error)
}

func NewCPUTool() *CPUTool {
	return &CPUTool{
		statPath: cpuStatPath,
		delay:    cpuSampleDelay,
		readStat: readCPUStat,
	}
}

func (t *CPUTool) Name() string {
	return ToolSystemCPU
}

func (t *CPUTool) Version() string {
	return toolCPUVersion
}

func (t *CPUTool) Description() string {
	return toolCPUDescription
}

func (t *CPUTool) ParameterSchema() string {
	return agent.EmptyParameterSchema
}

func (t *CPUTool) ConfirmationLevel() agent.ConfirmationLevel {
	return agent.ConfirmationNone
}

func (t *CPUTool) Metadata() agent.ToolMetadata {
	return agent.ToolMetadata{
		Name:                 t.Name(),
		Description:          t.Description(),
		Category:             agent.CategorySystem,
		Domain:               "linux",
		Tags:                 []string{"cpu", "usage", "host"},
		Risk:                 agent.RiskReadOnly,
		RequiresConfirmation: t.ConfirmationLevel() == agent.ConfirmationRequired,
		EstimatedDuration:    agent.DurationInstant,
		SinceVersion:         toolCPUVersion,
	}
}

func (t *CPUTool) Availability(_ context.Context) (bool, string) {
	return platformSupported()
}

func (t *CPUTool) Execute(ctx context.Context, _ []byte) ([]byte, error) {
	first, err := t.readStat(t.statPath)
	if err != nil {
		return nil, err
	}

	if err := sleepCtx(ctx, t.delay); err != nil {
		return nil, err
	}

	second, err := t.readStat(t.statPath)
	if err != nil {
		return nil, err
	}

	return json.Marshal(cpuFromDeltas(second.sub(first)))
}

// sleepCtx sleeps for d, or returns ctx.Err() if the context finishes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readCPUStat(path string) (cpuSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return cpuSample{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	return parseCPUStat(f)
}

// parseCPUStat reads a /proc/stat aggregate "cpu" line
// ("cpu user nice system idle iowait irq softirq steal ...") and sums the
// fields into a sample. guest/guest_nice are ignored to avoid double counting
// them with user/nice.
func parseCPUStat(r io.Reader) (cpuSample, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			return cpuSample{}, fmt.Errorf("parse cpu stat: too few fields in %q", line)
		}
		nums := make([]uint64, len(fields)-1)
		for i, f := range fields[1:] {
			n, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return cpuSample{}, fmt.Errorf("parse cpu value %q: %w", f, err)
			}
			nums[i] = n
		}
		return sampleFromFields(nums), nil
	}
	if err := scanner.Err(); err != nil {
		return cpuSample{}, fmt.Errorf("read cpu stat: %w", err)
	}
	return cpuSample{}, fmt.Errorf("parse cpu stat: cpu line not found")
}

// sampleFromFields buckets raw jiffies into user (user+nice), system
// (system+irq+softirq), idle (idle+iowait) and total (through steal).
func sampleFromFields(nums []uint64) cpuSample {
	user := nums[0] + nums[1]
	system := nums[2] + nums[5] + nums[6]
	idle := nums[3] + nums[4]
	return cpuSample{
		user:   user,
		system: system,
		idle:   idle,
		total:  user + system + idle + nums[7],
	}
}

// cpuFromDeltas converts a per-interval sample into usage percentages rounded
// to two decimals.
func cpuFromDeltas(d cpuSample) cpuResult {
	if d.total == 0 {
		return cpuResult{}
	}
	return cpuResult{
		UserPercent:   roundPercent(float64(d.user) / float64(d.total)),
		SystemPercent: roundPercent(float64(d.system) / float64(d.total)),
		IdlePercent:   roundPercent(float64(d.idle) / float64(d.total)),
	}
}

func roundPercent(fraction float64) float64 {
	return math.Round(fraction*10000) / 100
}
