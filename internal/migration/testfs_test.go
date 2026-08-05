package migration

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/tsee9iii/opspilot/sql/migrations"
)

// migrationsFS returns the real embedded migration files.
func migrationsFS(t *testing.T) fs.FS {
	t.Helper()
	return migrations.FS
}

// badSourceFS is a source whose second migration fails at runtime; the first
// migration creates a table so rollback behavior can be asserted.
func badSourceFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"0001_ok.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE rollback_ok (id INT);"),
		},
		"0002_bad.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE rollback_bad (id INT);\nSELECT * FROM nonexistent_table;"),
		},
	}
}

// orderSourceFS is a source whose migrations depend on execution order: 0002
// inserts into a table created by 0001, so running 0002 first would fail.
func orderSourceFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"0001_create.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE ordered_data (value TEXT);"),
		},
		"0002_insert.sql": &fstest.MapFile{
			Data: []byte("INSERT INTO ordered_data (value) VALUES ('ok');"),
		},
	}
}
