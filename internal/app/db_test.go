package app

import (
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
