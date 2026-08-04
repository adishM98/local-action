package db

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

func TestOpenDB_MigratesOldRunsSchemaAddsBranchAndCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := old.Exec(`
		CREATE TABLE runs (
		  id INTEGER PRIMARY KEY AUTOINCREMENT,
		  repo_path TEXT NOT NULL,
		  workflow_file TEXT NOT NULL,
		  event TEXT NOT NULL,
		  inputs TEXT NOT NULL DEFAULT '{}',
		  status TEXT NOT NULL,
		  started_at INTEGER,
		  finished_at INTEGER,
		  created_at INTEGER NOT NULL
		);
		INSERT INTO runs (repo_path, workflow_file, event, status, created_at) VALUES ('/repo/a', 'ci.yml', 'push', 'success', 100);
	`); err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	old.Close()

	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	defer db.Close()

	var branch, sha string
	if err := db.QueryRow(`SELECT branch, commit_sha FROM runs WHERE repo_path = '/repo/a'`).Scan(&branch, &sha); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if branch != "" || sha != "" {
		t.Fatalf("expected empty defaults for a pre-existing row, got branch=%q sha=%q", branch, sha)
	}

	db.Close()
	db2, err := OpenDB(path)
	if err != nil {
		t.Fatalf("re-open after migration: %v", err)
	}
	db2.Close()
}

func TestGetSetMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	v, err := GetMeta(db, "last_version")
	if err != nil {
		t.Fatalf("get unset meta: %v", err)
	}
	if v != "" {
		t.Fatalf("expected empty string for unset key, got %q", v)
	}

	if err := SetMeta(db, "last_version", "0.9.0"); err != nil {
		t.Fatalf("set meta: %v", err)
	}
	v, err = GetMeta(db, "last_version")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if v != "0.9.0" {
		t.Fatalf("expected 0.9.0, got %q", v)
	}

	if err := SetMeta(db, "last_version", "0.9.1"); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	v, _ = GetMeta(db, "last_version")
	if v != "0.9.1" {
		t.Fatalf("expected updated value 0.9.1, got %q", v)
	}
}
