package agent

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrPolicyDisabled    = errors.New("execution policy is disabled")
	ErrCommandDenied     = errors.New("command denied by policy")
	ErrCommandNotAllowed = errors.New("command not allowed by policy")
)

// ExecutionPolicy authorizes shell command execution before it starts.
// Program matching is done on the first token of the command string.
type ExecutionPolicy struct {
	Enabled          bool
	Timeout          time.Duration
	AllowedCommands  []string
	DeniedCommands   []string
	WorkingDirectory string
}

// Allow returns nil if program may run, or a sentinel error describing the
// policy decision. Denied commands win over the allow list.
func (p ExecutionPolicy) Allow(program string) error {
	if !p.Enabled {
		return ErrPolicyDisabled
	}
	if containsString(p.DeniedCommands, program) {
		return ErrCommandDenied
	}
	if len(p.AllowedCommands) > 0 && !containsString(p.AllowedCommands, program) {
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

func commandName(command string) string {
	command = strings.TrimSpace(command)
	if i := strings.IndexAny(command, " \t"); i >= 0 {
		return command[:i]
	}
	return command
}
