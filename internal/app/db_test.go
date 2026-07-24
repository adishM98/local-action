package app

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDB_CreatesSchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	db.Close()

	db2, err := OpenDB(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()

	rows, err := db2.Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables[name] = true
	}
	for _, want := range []string{"secrets", "runs", "run_logs"} {
		if !tables[want] {
			t.Errorf("expected table %q to exist", want)
		}
	}
}

func TestOpenDB_MigratesOldSecretsSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// Build a DB with the pre-workflow_file schema, bypassing OpenDB.
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := old.Exec(`
		CREATE TABLE secrets (
		  repo_path TEXT NOT NULL,
		  kind TEXT NOT NULL,
		  key TEXT NOT NULL,
		  value_encrypted BLOB NOT NULL,
		  PRIMARY KEY (repo_path, kind, key)
		);
		INSERT INTO secrets VALUES ('/repo/a', 'secret', 'TOKEN', x'DEADBEEF');
	`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	old.Close()

	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM secrets WHERE repo_path='/repo/a' AND kind='secret' AND key='TOKEN' AND workflow_file=''`,
	).Scan(&count); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migrated row with workflow_file='', got count=%d", count)
	}

	// Re-open: migration must be idempotent.
	db.Close()
	db2, err := OpenDB(path)
	if err != nil {
		t.Fatalf("re-open after migration: %v", err)
	}
	db2.Close()
}
