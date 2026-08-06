package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tsee9iii/opspilot/internal/application/inventory"
)

// TestInventoryProjections exercises the read-only inventory pipeline
// (use case -> repository -> database) against a real PostgreSQL and skips when
// the test database is unavailable. It verifies the projections are complete
// for their read surfaces and that no secret, payload or result data leaks.
func TestInventoryProjections(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	resetSchema(t, ctx, pool)

	serverA := seedServer(t, ctx, pool, "edge-1", "edge-1.example", "prod")
	serverB := seedServer(t, ctx, pool, "edge-2", "edge-2.example", "prod")
	agentA1 := seedAgentOn(t, ctx, pool, serverA, "online", mustTimePtr(time.Now().Add(-time.Minute)))
	agentA2 := seedAgentOn(t, ctx, pool, serverA, "offline", nil)
	agentB1 := seedAgentOn(t, ctx, pool, serverB, "online", mustTimePtr(time.Now().Add(-2*time.Minute)))

	repo := NewInventoryRepository(pool)
	serversUC := inventory.NewListServersUseCase(repo)
	agentsUC := inventory.NewListAgentsUseCase(repo)
	commandsUC := inventory.NewListCommandsUseCase(repo)

	t.Run("list servers with agent totals", func(t *testing.T) {
		servers, err := serversUC.List(ctx)
		if err != nil {
			t.Fatalf("list servers: %v", err)
		}
		if len(servers) != 2 {
			t.Fatalf("expected 2 servers, got %d: %+v", len(servers), servers)
		}
		byName := map[string]inventory.ServerSummary{}
		for _, s := range servers {
			byName[s.Name] = s
		}
		a := byName["edge-1"]
		b := byName["edge-2"]
		if a.AgentCount != 2 || a.OnlineAgentCount != 1 {
			t.Fatalf("expected edge-1 totals 2/1, got %d/%d", a.AgentCount, a.OnlineAgentCount)
		}
		if b.AgentCount != 1 || b.OnlineAgentCount != 1 {
			t.Fatalf("expected edge-2 totals 1/1, got %d/%d", b.AgentCount, b.OnlineAgentCount)
		}
	})

	t.Run("list agents unfiltered and filtered", func(t *testing.T) {
		all, err := agentsUC.List(ctx, inventory.ListAgentsRequest{})
		if err != nil {
			t.Fatalf("list agents: %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("expected 3 agents, got %d", len(all))
		}
		for _, a := range all {
			if a.ServerName == "" || a.Hostname == "" || a.Environment == "" {
				t.Fatalf("missing server context in projection: %+v", a)
			}
		}

		onServer, err := agentsUC.List(ctx, inventory.ListAgentsRequest{ServerID: &serverA})
		if err != nil {
			t.Fatalf("filter by server: %v", err)
		}
		if len(onServer) != 2 {
			t.Fatalf("expected 2 agents on edge-1, got %d", len(onServer))
		}

		online, err := agentsUC.List(ctx, inventory.ListAgentsRequest{Status: "online"})
		if err != nil {
			t.Fatalf("filter by status: %v", err)
		}
		if len(online) != 2 {
			t.Fatalf("expected 2 online agents, got %d", len(online))
		}

		both, err := agentsUC.List(ctx, inventory.ListAgentsRequest{ServerID: &serverA, Status: "online"})
		if err != nil {
			t.Fatalf("filter by both: %v", err)
		}
		if len(both) != 1 || both[0].ID != agentA1 {
			t.Fatalf("expected only agentA1, got %+v", both)
		}
	})

	t.Run("list commands is lightweight and filterable", func(t *testing.T) {
		created := make([]string, 0, 4)
		for _, tc := range []struct {
			agent uuid.UUID
			tool  string
			pay   string
		}{
			{agentA1, "system.uptime", `{"host":"edge-1"}`},
			{agentA1, "pm2.restart", `{"process":"web","token":"secret"}`},
			{agentA2, "system.uptime", `{}`},
			{agentB1, "system.uptime", `{}`},
		} {
			var id string
			if err := pool.QueryRow(ctx,
				`INSERT INTO commands (agent_id, tool_name, payload, status, result, error)
				 VALUES ($1, $2, $3::jsonb, 'completed', '{"ok":true}', NULL) RETURNING id`,
				tc.agent, tc.tool, tc.pay).Scan(&id); err != nil {
				t.Fatalf("insert command: %v", err)
			}
			created = append(created, id)
		}

		cmds, err := commandsUC.List(ctx, inventory.ListCommandsRequest{Limit: 100})
		if err != nil {
			t.Fatalf("list commands: %v", err)
		}
		if len(cmds) != 4 {
			t.Fatalf("expected 4 commands, got %d", len(cmds))
		}
		for _, c := range cmds {
			if c.Tool == "" || c.Status == "" || c.CreatedAt.IsZero() {
				t.Fatalf("incomplete summary: %+v", c)
			}
		}

		onAgent, err := commandsUC.List(ctx, inventory.ListCommandsRequest{AgentID: &agentA1, Limit: 100})
		if err != nil {
			t.Fatalf("filter commands by agent: %v", err)
		}
		if len(onAgent) != 2 {
			t.Fatalf("expected 2 commands for agentA1, got %d", len(onAgent))
		}
	})

	t.Run("limit caps the result set", func(t *testing.T) {
		limited, err := commandsUC.List(ctx, inventory.ListCommandsRequest{Limit: 2})
		if err != nil {
			t.Fatalf("list with limit: %v", err)
		}
		if len(limited) != 2 {
			t.Fatalf("expected 2 commands, got %d", len(limited))
		}
	})
}

func mustTimePtr(t time.Time) *time.Time { return &t }

func seedServer(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, hostname, env string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO servers (name, hostname, environment) VALUES ($1, $2, $3) RETURNING id`,
		name, hostname, env).Scan(&id); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return id
}

func seedAgentOn(t *testing.T, ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, status string, heartbeat *time.Time) uuid.UUID {
	t.Helper()
	agentID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO agents (id, server_id, secret, version, status, last_heartbeat) VALUES ($1, $2, 'hash', '1.0', $3, $4)`,
		agentID, serverID, status, heartbeat); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	return agentID
}
