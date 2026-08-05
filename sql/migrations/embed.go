// Package migrations embeds the SQL migration files so the built-in migration
// runner can execute them from the binary without reading the filesystem at
// runtime. sql/migrations/*.sql remains the single source of truth; sqlc still
// reads them from disk for type generation.
package migrations

import "embed"

// FS contains every migration file (sql/migrations/*.sql).
//
//go:embed *.sql
var FS embed.FS
