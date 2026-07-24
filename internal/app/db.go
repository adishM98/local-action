package app

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
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS run_logs (
  run_id INTEGER NOT NULL,
  line_no INTEGER NOT NULL,
  text TEXT NOT NULL,
  PRIMARY KEY (run_id, line_no)
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
	return db, nil
}

// migrateSecretsWorkflowFile upgrades a pre-workflow_file secrets table in
// place. Old rows become repo-wide (workflow_file = ''). SQLite can't add a
// column into an existing PRIMARY KEY, so we rebuild the table.
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
	_, err = db.Exec(`
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
	`)
	return err
}
