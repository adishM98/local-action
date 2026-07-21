package main

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS secrets (
  repo_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  key TEXT NOT NULL,
  value_encrypted BLOB NOT NULL,
  PRIMARY KEY (repo_path, kind, key)
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
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
