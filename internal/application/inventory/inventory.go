// Package inventory exposes read-only inventory projections of the platform:
// servers, agents and command summaries. The projection shapes live here, in
// the application layer; the MCP adapter and any future transport map them onto
// their own wire formats.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidServerID is returned when a server_id filter is not a UUID.
	ErrInvalidServerID = errors.New("invalid server id")
	// ErrInvalidAgentID is returned when an agent_id filter is not a UUID.
	ErrInvalidAgentID = errors.New("invalid agent id")
	// ErrInvalidLimit is returned when a limit is outside the allowed range.
	ErrInvalidLimit = errors.New("invalid limit")
)

const (
	// MaxLimit is the largest command result set a single call may return.
	MaxLimit = 500
	// DefaultLimit is used when a caller omits the limit.
	DefaultLimit = 50
)

// ServerSummary is a server with its agent totals. It contains no secrets.
type ServerSummary struct {
	ID               uuid.UUID
	Name             string
	Hostname         string
	Environment      string
	Status           string
	AgentCount       int64
	OnlineAgentCount int64
}

// AgentSummary is an agent with its server context. The secret hash is never
// part of the projection.
type AgentSummary struct {
	ID            uuid.UUID
	ServerID      uuid.UUID
	ServerName    string
	Hostname      string
	Environment   string
	Version       string
	Status        string
	LastHeartbeat *time.Time
}

// CommandSummary is a lightweight command record. Payload and result are never
// part of the projection.
type CommandSummary struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	Tool      string
	Status    string
	CreatedAt time.Time
}

// ListAgentsRequest filters the agent projection. Empty filters select all
// agents.
type ListAgentsRequest struct {
	ServerID *uuid.UUID
	Status   string
}

// ListCommandsRequest filters the command projection and caps the result set.
type ListCommandsRequest struct {
	Status  string
	AgentID *uuid.UUID
	Limit   int32
}

// ServerRepository persists the server projection.
type ServerRepository interface {
	ListServers(ctx context.Context) ([]ServerSummary, error)
}

// AgentRepository persists the agent projection.
type AgentRepository interface {
	ListAgents(ctx context.Context, req ListAgentsRequest) ([]AgentSummary, error)
}

// CommandRepository persists the command projection.
type CommandRepository interface {
	ListCommands(ctx context.Context, req ListCommandsRequest) ([]CommandSummary, error)
}

// ListServersUseCase returns every server with its online/offline agent
// summary, ordered by name.
type ListServersUseCase struct {
	repo ServerRepository
}

func NewListServersUseCase(repo ServerRepository) *ListServersUseCase {
	return &ListServersUseCase{repo: repo}
}

func (uc *ListServersUseCase) List(ctx context.Context) ([]ServerSummary, error) {
	servers, err := uc.repo.ListServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("inventory: list servers: %w", err)
	}
	return servers, nil
}

// ListAgentsUseCase returns agents matching the optional server_id and status
// filters.
type ListAgentsUseCase struct {
	repo AgentRepository
}

func NewListAgentsUseCase(repo AgentRepository) *ListAgentsUseCase {
	return &ListAgentsUseCase{repo: repo}
}

func (uc *ListAgentsUseCase) List(ctx context.Context, req ListAgentsRequest) ([]AgentSummary, error) {
	if req.ServerID != nil {
		if err := validateUUID(*req.ServerID); err != nil {
			return nil, ErrInvalidServerID
		}
	}
	agents, err := uc.repo.ListAgents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("inventory: list agents: %w", err)
	}
	return agents, nil
}

// ListCommandsUseCase returns command summaries matching the optional status
// and agent_id filters, capped by limit.
type ListCommandsUseCase struct {
	repo CommandRepository
}

func NewListCommandsUseCase(repo CommandRepository) *ListCommandsUseCase {
	return &ListCommandsUseCase{repo: repo}
}

func (uc *ListCommandsUseCase) List(ctx context.Context, req ListCommandsRequest) ([]CommandSummary, error) {
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}
	if req.Limit > MaxLimit {
		return nil, ErrInvalidLimit
	}
	if req.AgentID != nil {
		if err := validateUUID(*req.AgentID); err != nil {
			return nil, ErrInvalidAgentID
		}
	}
	commands, err := uc.repo.ListCommands(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("inventory: list commands: %w", err)
	}
	return commands, nil
}

func validateUUID(id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.New("zero value id")
	}
	return nil
}
