package workflow

import (
	"encoding/json"
	"time"
)

// StepStatus is the lifecycle state of a single workflow step.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// Result is the outcome of running a workflow. On failure the workflow stops
// immediately: the failing step is marked failed and every remaining step is
// marked skipped.
type Result struct {
	Workflow   string       `json:"workflow"`
	Project    string       `json:"project"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Steps      []StepResult `json:"steps"`
	Success    bool         `json:"success"`
}

// StepResult captures the execution of a single step. Result holds the raw
// tool output on success and Error the failure message on failure.
type StepResult struct {
	Name       string          `json:"name"`
	Tool       string          `json:"tool"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Status     StepStatus      `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}
