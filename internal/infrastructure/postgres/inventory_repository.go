package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/gen/postgresql"
	"github.com/tsee9iii/opspilot/internal/application/inventory"
)

var (
	_ inventory.ServerRepository  = (*InventoryRepository)(nil)
	_ inventory.AgentRepository   = (*InventoryRepository)(nil)
	_ inventory.CommandRepository = (*InventoryRepository)(nil)
)

// InventoryRepository provides the read-only inventory projections. It only
// reads: no secret, payload or result columns are ever projected.
type InventoryRepository struct {
	q *postgresql.Queries
}

func NewInventoryRepository(pool *pgxpool.Pool) *InventoryRepository {
	return &InventoryRepository{q: postgresql.New(pool)}
}

func (r *InventoryRepository) ListServers(ctx context.Context) ([]inventory.ServerSummary, error) {
	rows, err := r.q.ListServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list servers: %w", err)
	}
	servers := make([]inventory.ServerSummary, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, inventory.ServerSummary{
			ID:               row.ID,
			Name:             row.Name,
			Hostname:         row.Hostname,
			Environment:      row.Environment,
			Status:           row.Status,
			AgentCount:       row.AgentCount,
			OnlineAgentCount: row.OnlineAgentCount,
		})
	}
	return servers, nil
}

func (r *InventoryRepository) ListAgents(ctx context.Context, req inventory.ListAgentsRequest) ([]inventory.AgentSummary, error) {
	params := postgresql.ListAgentsParams{}
	if req.ServerID != nil {
		params.ServerID = pgtype.UUID{Bytes: *req.ServerID, Valid: true}
	}
	if req.Status != "" {
		params.Status = pgtype.Text{String: req.Status, Valid: true}
	}

	rows, err := r.q.ListAgents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("postgres: list agents: %w", err)
	}
	agents := make([]inventory.AgentSummary, 0, len(rows))
	for _, row := range rows {
		agents = append(agents, inventory.AgentSummary{
			ID:            row.ID,
			ServerID:      row.ServerID,
			ServerName:    row.ServerName,
			Hostname:      row.Hostname,
			Environment:   row.Environment,
			Version:       row.Version,
			Status:        row.Status,
			LastHeartbeat: pgtypeTimePtr(row.LastHeartbeat),
		})
	}
	return agents, nil
}

func (r *InventoryRepository) ListCommands(ctx context.Context, req inventory.ListCommandsRequest) ([]inventory.CommandSummary, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = inventory.DefaultLimit
	}
	params := postgresql.ListCommandsParams{Limit: limit}
	if req.Status != "" {
		params.Status = pgtype.Text{String: req.Status, Valid: true}
	}
	if req.AgentID != nil {
		params.AgentID = pgtype.UUID{Bytes: *req.AgentID, Valid: true}
	}

	rows, err := r.q.ListCommands(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("postgres: list commands: %w", err)
	}
	commands := make([]inventory.CommandSummary, 0, len(rows))
	for _, row := range rows {
		commands = append(commands, inventory.CommandSummary{
			ID:        row.ID,
			AgentID:   row.AgentID,
			Tool:      row.ToolName,
			Status:    row.Status,
			CreatedAt: row.CreatedAt,
		})
	}
	return commands, nil
}
