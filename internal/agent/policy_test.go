package agent

import (
	"errors"
	"testing"
)

func TestExecutionPolicyDisabled(t *testing.T) {
	p := ExecutionPolicy{Enabled: false}
	if err := p.Allow("uptime"); !errors.Is(err, ErrPolicyDisabled) {
		t.Fatalf("expected ErrPolicyDisabled, got: %v", err)
	}
}

func TestExecutionPolicyDenied(t *testing.T) {
	p := ExecutionPolicy{Enabled: true, DeniedCommands: []string{"rm"}}
	if err := p.Allow("rm"); !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("expected ErrCommandDenied, got: %v", err)
	}
	if err := p.Allow("uptime"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutionPolicyAllowedList(t *testing.T) {
	p := ExecutionPolicy{Enabled: true, AllowedCommands: []string{"uptime"}}
	if err := p.Allow("uptime"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := p.Allow("ls"); !errors.Is(err, ErrCommandNotAllowed) {
		t.Fatalf("expected ErrCommandNotAllowed, got: %v", err)
	}
}

func TestExecutionPolicyEmptyAllowedMeansAllowAll(t *testing.T) {
	p := ExecutionPolicy{Enabled: true}
	if err := p.Allow("anything"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutionPolicyDeniedOverridesAllowed(t *testing.T) {
	p := ExecutionPolicy{Enabled: true, AllowedCommands: []string{"rm"}, DeniedCommands: []string{"rm"}}
	if err := p.Allow("rm"); !errors.Is(err, ErrCommandDenied) {
		t.Fatalf("expected ErrCommandDenied, got: %v", err)
	}
}
