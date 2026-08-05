package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS secrets (
  repo_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  key TEXT NOT NULL,
  workflow_file TEXT NOT NULL DEFAULT '',
  value_encrypted BLOB NOT NULL,
  revealable INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (repo_path, kind, key, workflow_file)
);

CREATE TABLE IF NOT EXISTS runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_path TEXT NOT NULL,
  workflow_file TEXT NOT NULL,
  event TEXT NOT NULL,
  inputs TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  started_at INTEGER,
  finished_at INTEGER,
  created_at INTEGER NOT NULL,
  branch TEXT NOT NULL DEFAULT '',
  commit_sha TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS run_logs (
  run_id INTEGER NOT NULL,
  line_no INTEGER NOT NULL,
  text TEXT NOT NULL,
  PRIMARY KEY (run_id, line_no)
);

CREATE TABLE IF NOT EXISTS event_payloads (
  repo_path TEXT NOT NULL,
  workflow_file TEXT NOT NULL,
  payload TEXT NOT NULL,
  PRIMARY KEY (repo_path, workflow_file)
);

CREATE TABLE IF NOT EXISTS workflow_categories (
  repo_path TEXT NOT NULL,
  workflow_file TEXT NOT NULL,
  category TEXT NOT NULL,
  PRIMARY KEY (repo_path, workflow_file)
);

-- Generic key/value store for small pieces of app state that aren't
-- worth a dedicated table — currently just the last version this install
-- has seen, so a version bump can be detected once and offer a run-history
-- reset instead of silently carrying old runs forward.
CREATE TABLE IF NOT EXISTS app_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite only supports one writer at a time; database/sql's default
	// pool opens multiple connections and interleaves reads/writes across
	// them, which surfaces as spurious SQLITE_BUSY errors under concurrent
	// access (e.g. the act engine's run/log writers racing HTTP/test
	// readers). Force a single connection so all access is serialized.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateSecretsWorkflowFile(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateRunsBranchCommit(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateSecretsRevealable(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// migrateSecretsRevealable adds revealable to a pre-existing secrets table.
// Existing rows default to 1 (revealable) — they were saved back when
// there was no write-only choice at all, under the assumption a value
// could always be viewed again, so upgrading silently makes something
// previously-viewable stop being viewable would be a surprising regression,
// not a security improvement.
func migrateSecretsRevealable(db *sql.DB) error {
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('secrets') WHERE name = 'revealable'`,
	).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.Exec(`ALTER TABLE secrets ADD COLUMN revealable INTEGER NOT NULL DEFAULT 1`)
	return err
}

// migrateRunsBranchCommit adds branch/commit_sha to a pre-existing runs
// table. Unlike migrateSecretsWorkflowFile, these columns aren't part of a
// primary key, so a plain ADD COLUMN suffices — no table rebuild needed.
// Each column is checked and added independently (rather than gating both
// on one sentinel check) so a crash between the two ALTERs can't leave one
// column permanently missing on restart.
func migrateRunsBranchCommit(db *sql.DB) error {
	for _, col := range []string{"branch", "commit_sha"} {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('runs') WHERE name = ?`, col,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

// migrateSecretsWorkflowFile upgrades a pre-workflow_file secrets table in
// place. Old rows become repo-wide (workflow_file = ”). SQLite can't add a
// column into an existing PRIMARY KEY, so we rebuild the table. The rebuild
// is wrapped in a transaction to ensure all-or-nothing semantics on crash.
func migrateSecretsWorkflowFile(db *sql.DB) error {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('secrets') WHERE name = 'workflow_file'`,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		ALTER TABLE secrets RENAME TO secrets_old;
		CREATE TABLE secrets (
		  repo_path TEXT NOT NULL,
		  kind TEXT NOT NULL,
		  key TEXT NOT NULL,
		  workflow_file TEXT NOT NULL DEFAULT '',
		  value_encrypted BLOB NOT NULL,
		  PRIMARY KEY (repo_path, kind, key, workflow_file)
		);
		INSERT INTO secrets (repo_path, kind, key, workflow_file, value_encrypted)
		  SELECT repo_path, kind, key, '', value_encrypted FROM secrets_old;
		DROP TABLE secrets_old;
	`); err != nil {
		return err
	}
	return tx.Commit()
}

// GetMeta returns "" (not an error) for a key that's never been set — every
// caller so far treats "unset" and "empty" the same way (e.g. "no version
// recorded yet, so this must be the very first launch").
func GetMeta(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM app_meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func SetMeta(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO app_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
