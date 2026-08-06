package workflow

import (
	"context"
	"errors"
	"time"

	"github.com/tsee9iii/opspilot/internal/agent"
	"github.com/tsee9iii/opspilot/internal/agent/project"
)

// Executor runs workflows by executing each step's tool through the agent's
// executor (the RegistryExecutor in production), so every step goes through
// the normal execution pipeline: registry lookup, policy gate, JSON Schema
// payload validation, and tool execution. It does not bypass the registry.
type Executor struct {
	executor      agent.Executor
	stopOnFailure bool
	onStep        func(step string, status StepStatus)
	abortWhen     func(sr StepResult) bool
	failWhen      func(sr StepResult) (bool, string)
}

func NewExecutor(executor agent.Executor) *Executor {
	return &Executor{executor: executor, stopOnFailure: true}
}

// StopOnFailure toggles whether a failing step stops the workflow and marks
// the remaining steps skipped. Deploy workflows stop on failure; diagnose
// workflows execute every step regardless of failure. Returns the executor for
// chaining.
func (e *Executor) StopOnFailure(stop bool) *Executor {
	e.stopOnFailure = stop
	return e
}

// AbortWhen registers a predicate evaluated after every completed step. When
// it returns true, the completed step stays recorded but execution stops: the
// current step remains completed, every remaining step is marked skipped, and
// the workflow is treated as failed. It is used by deploy workflows to refuse
// a deployment when the repository is dirty. Returns the executor for
// chaining.
func (e *Executor) AbortWhen(fn func(sr StepResult) bool) *Executor {
	e.abortWhen = fn
	return e
}

// FailWhen registers a predicate evaluated after every completed step. When it
// returns true, the step is re-marked as failed with the returned reason and
// execution stops with the remaining steps skipped. It is used by deploy
// workflows to fail when the health check reports an unhealthy service.
// Returns the executor for chaining.
func (e *Executor) FailWhen(fn func(sr StepResult) (bool, string)) *Executor {
	e.failWhen = fn
	return e
}

// OnStep registers a callback invoked on every step state transition during
// Execute. It is intended for observability and testing.
func (e *Executor) OnStep(fn func(step string, status StepStatus)) {
	e.onStep = fn
}

// Execute runs wf against the project profile p, executing each step in order
// via the injected agent executor. When stopOnFailure is set, a failing step
// stops execution immediately: it is marked failed, every remaining step is
// marked skipped, and Success is false. Otherwise every step executes and the
// workflow succeeds when at least one step completed successfully.
func (e *Executor) Execute(ctx context.Context, p project.Project, wf Workflow) Result {
	res := Result{
		Workflow:  wf.Name,
		Project:   p.Name,
		StartedAt: time.Now(),
		Steps:     make([]StepResult, 0, len(wf.Steps)),
	}

	stop := e.stopOnFailure
	failed := false
	completed := 0

	for i, step := range wf.Steps {
		sr := StepResult{
			Name:       step.Name,
			Tool:       step.Tool.Tool,
			Parameters: step.Tool.Parameters,
			Status:     StepPending,
			StartedAt:  time.Now(),
		}
		e.emit(step.Name, sr.Status)

		sr.Status = StepRunning
		e.emit(step.Name, sr.Status)

		out, err := e.executor.Execute(ctx, step.Tool.Tool, step.Tool.Parameters)
		if err != nil {
			sr.Status = StepFailed
			sr.Error = err.Error()
			var te *agent.ToolError
			if errors.As(err, &te) {
				sr.ErrorCode = te.Code
				sr.Message = te.Message
				sr.Suggestion = te.Suggestion
			}
			sr.FinishedAt = time.Now()
			e.emit(step.Name, sr.Status)
			res.Steps = append(res.Steps, sr)
			failed = true
			if stop {
				res = e.markSkipped(res, wf.Steps[i+1:])
				break
			}
			continue
		}

		sr.Status = StepCompleted
		sr.Result = out
		sr.FinishedAt = time.Now()
		e.emit(step.Name, sr.Status)
		res.Steps = append(res.Steps, sr)
		completed++

		if e.abortWhen != nil && e.abortWhen(sr) {
			failed = true
			res = e.markSkipped(res, wf.Steps[i+1:])
			break
		}
		if e.failWhen != nil {
			if should, reason := e.failWhen(sr); should {
				sr.Status = StepFailed
				sr.Error = reason
				sr.FinishedAt = time.Now()
				e.emit(sr.Name, sr.Status)
				res.Steps[len(res.Steps)-1] = sr
				completed--
				failed = true
				res = e.markSkipped(res, wf.Steps[i+1:])
				break
			}
		}
	}

	res.FinishedAt = time.Now()
	if stop {
		res.Success = !failed
	} else {
		res.Success = completed > 0
	}
	return res
}

// markSkipped appends every remaining step as skipped and returns the updated
// result.
func (e *Executor) markSkipped(res Result, remaining []Step) Result {
	for _, rem := range remaining {
		skipped := StepResult{
			Name:   rem.Name,
			Tool:   rem.Tool.Tool,
			Status: StepSkipped,
		}
		e.emit(rem.Name, skipped.Status)
		res.Steps = append(res.Steps, skipped)
	}
	return res
}

func (e *Executor) emit(step string, status StepStatus) {
	if e.onStep != nil {
		e.onStep(step, status)
	}
}
