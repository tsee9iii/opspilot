package migration

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// Migration is a single embedded migration file.
type Migration struct {
	Version string // file name, e.g. "0001_init.sql"
	SQL     string
}

// Runner applies pending migrations in lexicographical order.
type Runner struct {
	source fs.FS
	store  *Storage
}

func NewRunner(source fs.FS, store *Storage) *Runner {
	return &Runner{source: source, store: store}
}

// Migrations returns every migration file, sorted lexicographically by file
// name (e.g. 0001_… < 0002_… < … < 0010_…).
func (r *Runner) Migrations() ([]Migration, error) {
	entries, err := fs.ReadDir(r.source, ".")
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	migrations := make([]Migration, 0, len(files))
	for _, name := range files {
		data, err := fs.ReadFile(r.source, name)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{Version: name, SQL: string(data)})
	}
	return migrations, nil
}

// Run applies all pending migrations in order and returns the versions applied
// during this call (empty when already up to date). Each migration runs in its
// own transaction; on failure the current transaction is rolled back, execution
// stops, and the error is returned.
func (r *Runner) Run(ctx context.Context) ([]string, error) {
	if err := r.store.EnsureTable(ctx); err != nil {
		return nil, fmt.Errorf("migration: ensure schema_migrations: %w", err)
	}

	applied, err := r.store.AppliedVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration: read applied: %w", err)
	}

	migrations, err := r.Migrations()
	if err != nil {
		return nil, fmt.Errorf("migration: load embedded migrations: %w", err)
	}

	var appliedNow []string
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if err := r.store.apply(ctx, m.Version, m.SQL); err != nil {
			return appliedNow, fmt.Errorf("migration %s: %w", m.Version, err)
		}
		appliedNow = append(appliedNow, m.Version)
	}
	return appliedNow, nil
}

// Status reports applied and pending migrations. Applied versions are sorted by
// file name to match execution order.
type Status struct {
	Applied []string
	Pending []string
}

func (r *Runner) Status(ctx context.Context) (Status, error) {
	if err := r.store.EnsureTable(ctx); err != nil {
		return Status{}, fmt.Errorf("migration: ensure schema_migrations: %w", err)
	}

	appliedSet, err := r.store.AppliedVersions(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("migration: read applied: %w", err)
	}

	migrations, err := r.Migrations()
	if err != nil {
		return Status{}, fmt.Errorf("migration: load embedded migrations: %w", err)
	}

	var st Status
	for _, m := range migrations {
		if _, ok := appliedSet[m.Version]; ok {
			st.Applied = append(st.Applied, m.Version)
		} else {
			st.Pending = append(st.Pending, m.Version)
		}
	}
	return st, nil
}
