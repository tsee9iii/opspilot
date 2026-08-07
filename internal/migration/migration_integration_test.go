package migration

import (
	"context"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrations uses a dedicated database (opspilot_migration_test) so it
// never interferes with the postgres package integration tests that share the
// opspilot database. It requires a reachable PostgreSQL and skips otherwise.
func TestMigrations(t *testing.T) {
	ctx := context.Background()
	pool := migrationTestPool(t)
	defer pool.Close()

	resetDatabase(t, ctx, pool)

	t.Run("embedded migration loading", func(t *testing.T) {
		runner := NewRunner(migrationsFS(t), NewStorage(pool))
		list, err := runner.Migrations()
		if err != nil {
			t.Fatalf("load migrations: %v", err)
		}
		if len(list) != 14 {
			t.Fatalf("expected 14 embedded migrations, got %d", len(list))
		}
		for i, m := range list {
			if i > 0 && list[i-1].Version >= m.Version {
				t.Fatalf("migrations not sorted: %s before %s", list[i-1].Version, m.Version)
			}
			if m.SQL == "" {
				t.Fatalf("migration %s has empty SQL", m.Version)
			}
		}
	})

	t.Run("empty database runs all migrations", func(t *testing.T) {
		runner := NewRunner(migrationsFS(t), NewStorage(pool))
		applied, err := runner.Run(ctx)
		if err != nil {
			t.Fatalf("run migrations: %v", err)
		}
		if len(applied) != 14 {
			t.Fatalf("expected 14 applied, got %d: %v", len(applied), applied)
		}
		assertColumnCount(t, ctx, pool, "schema_migrations", 14)
		assertTableExists(t, ctx, pool, "agents")
	})

	t.Run("schema_migrations has the required columns", func(t *testing.T) {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM information_schema.columns
			WHERE table_name = 'schema_migrations'
			  AND column_name IN ('version', 'applied_at')`).Scan(&count); err != nil {
			t.Fatalf("inspect columns: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected version + applied_at columns, got %d", count)
		}
		var hasDefault string
		if err := pool.QueryRow(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_name = 'schema_migrations' AND column_name = 'applied_at'`).Scan(&hasDefault); err != nil {
			t.Fatalf("inspect applied_at default: %v", err)
		}
		if hasDefault == "" {
			t.Fatal("applied_at has no default")
		}
	})

	t.Run("already up-to-date is a no-op", func(t *testing.T) {
		runner := NewRunner(migrationsFS(t), NewStorage(pool))
		applied, err := runner.Run(ctx)
		if err != nil {
			t.Fatalf("second run: %v", err)
		}
		if len(applied) != 0 {
			t.Fatalf("expected no migrations to run again, got %d: %v", len(applied), applied)
		}
	})

	t.Run("status reports all applied after full run", func(t *testing.T) {
		runner := NewRunner(migrationsFS(t), NewStorage(pool))
		st, err := runner.Status(ctx)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if len(st.Applied) != 14 || len(st.Pending) != 0 {
			t.Fatalf("expected 14 applied / 0 pending, got %d / %d", len(st.Applied), len(st.Pending))
		}
	})

	t.Run("partially migrated database runs only the remainder", func(t *testing.T) {
		resetDatabase(t, ctx, pool)

		// Apply only the first two migrations (their real SQL) so the schema
		// reflects a database that was migrated up to 0002.
		firstTwo := map[string][]byte{}
		for _, v := range []string{"0001_init.sql", "0002_agent_auth.sql"} {
			data, err := fs.ReadFile(migrationsFS(t), v)
			if err != nil {
				t.Fatalf("read %s: %v", v, err)
			}
			firstTwo[v] = data
		}
		storage := NewStorage(pool)
		partial := NewRunner(fstest.MapFS(map[string]*fstest.MapFile{
			"0001_init.sql":       {Data: firstTwo["0001_init.sql"]},
			"0002_agent_auth.sql": {Data: firstTwo["0002_agent_auth.sql"]},
		}), storage)
		if _, err := partial.Run(ctx); err != nil {
			t.Fatalf("apply first two: %v", err)
		}

		// Full source now sees 2 applied / 12 pending.
		runner := NewRunner(migrationsFS(t), storage)
		st, err := runner.Status(ctx)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if len(st.Applied) != 2 || len(st.Pending) != 12 {
			t.Fatalf("expected 2 applied / 12 pending, got %d / %d", len(st.Applied), len(st.Pending))
		}

		applied, err := runner.Run(ctx)
		if err != nil {
			t.Fatalf("run after partial: %v", err)
		}
		if len(applied) != 12 {
			t.Fatalf("expected 12 newly applied, got %d: %v", len(applied), applied)
		}
		// The two previously applied migrations must not be re-run.
		for _, v := range applied {
			if v == "0001_init.sql" || v == "0002_agent_auth.sql" {
				t.Fatalf("previously applied migration re-ran: %s", v)
			}
		}
		assertColumnCount(t, ctx, pool, "schema_migrations", 14)
	})

	t.Run("failed migration rolls back and stops", func(t *testing.T) {
		resetDatabase(t, ctx, pool)
		storage := NewStorage(pool)
		// Source with a valid first migration and a failing second one.
		runner := NewRunner(badSourceFS(t), storage)

		_, err := runner.Run(ctx)
		if err == nil {
			t.Fatal("expected run to fail")
		}

		// First migration succeeded and is recorded.
		assertTableExists(t, ctx, pool, "rollback_ok")
		// Second migration's table must be gone: the transaction rolled back.
		assertTableMissing(t, ctx, pool, "rollback_bad")
		assertColumnCount(t, ctx, pool, "schema_migrations", 1)

		// Rerunning still applies the failed migration attempt and fails again
		// (the failing version is never recorded).
		if _, err := runner.Run(ctx); err == nil {
			t.Fatal("expected rerun to fail again")
		}
		assertColumnCount(t, ctx, pool, "schema_migrations", 1)
	})

	t.Run("migration ordering is lexicographical", func(t *testing.T) {
		resetDatabase(t, ctx, pool)
		storage := NewStorage(pool)
		runner := NewRunner(orderSourceFS(t), storage)

		if _, err := runner.Run(ctx); err != nil {
			t.Fatalf("run ordered migrations: %v", err)
		}
		// order 0001 creates table, 0002 references it; running 0002 before
		// 0001 would fail, so success proves lexicographical order.
		var v string
		if err := pool.QueryRow(ctx, `SELECT value FROM ordered_data`).Scan(&v); err != nil {
			t.Fatalf("read ordered_data: %v", err)
		}
		if v != "ok" {
			t.Fatalf("expected value ok, got %q", v)
		}
	})
}

func migrationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("OPSPILOT_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://opspilot:opspilot@localhost:5432/opspilot?sslmode=disable"
	}

	admin, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(context.Background()); err != nil {
		t.Skipf("test database unavailable: %v", err)
	}

	// Create a dedicated database for this package.
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/opspilot_migration_test"
	if _, err := admin.Exec(context.Background(), `CREATE DATABASE opspilot_migration_test`); err != nil {
		// 42P04 = duplicate_database, expected on subsequent runs.
		if sqlErr, ok := err.(interface{ SQLState() string }); !ok || sqlErr.SQLState() != "42P04" {
			t.Skipf("test database unavailable: %v", err)
		}
	}

	pool, err := pgxpool.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}

func resetDatabase(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE`); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
}

func assertTableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, name).Scan(&n); err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	if n != 1 {
		t.Fatalf("expected table %s to exist", name)
	}
}

func assertTableMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, name).Scan(&n); err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	if n != 0 {
		t.Fatalf("expected table %s to be missing", name)
	}
}

func assertColumnCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, want int) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if n != want {
		t.Fatalf("expected %d rows in %s, got %d", want, table, n)
	}
}
