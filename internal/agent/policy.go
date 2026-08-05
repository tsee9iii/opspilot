package agent

import (
	"errors"
	"time"
)

var (
	ErrPolicyDisabled    = errors.New("execution policy is disabled")
	ErrCommandDenied     = errors.New("command denied by policy")
	ErrCommandNotAllowed = errors.New("command not allowed by policy")
)

// ExecutionPolicy authorizes tool execution before it starts.
type ExecutionPolicy struct {
	Enabled          bool
	Timeout          time.Duration
	AllowedCommands  []string
	DeniedCommands   []string
	WorkingDirectory string
}

// Allow returns nil if name may run, or a sentinel error describing the
// policy decision. Denied entries win over the allow list.
func (p ExecutionPolicy) Allow(name string) error {
	if !p.Enabled {
		return ErrPolicyDisabled
	}
	if containsString(p.DeniedCommands, name) {
		return ErrCommandDenied
	}
	if len(p.AllowedCommands) > 0 && !containsString(p.AllowedCommands, name) {
		return ErrCommandNotAllowed
	}
	return nil
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
