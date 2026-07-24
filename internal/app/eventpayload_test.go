package app

import (
	"path/filepath"
	"testing"
)

func TestEventPayload_SaveGetRoundTrip(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	got, err := GetEventPayload(db, "/repo/a", "ci.yml")
	if err != nil {
		t.Fatalf("get before save: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty payload before save, got %q", got)
	}

	payload := `{"action":"labeled","label":{"name":"run-ci"}}`
	if err := SaveEventPayload(db, "/repo/a", "ci.yml", payload); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err = GetEventPayload(db, "/repo/a", "ci.yml")
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	if got != payload {
		t.Fatalf("got %q, want %q", got, payload)
	}

	// A different workflow in the same repo is independent.
	got, err = GetEventPayload(db, "/repo/a", "deploy.yml")
	if err != nil {
		t.Fatalf("get other workflow: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty payload for unrelated workflow, got %q", got)
	}

	// Overwrite.
	updated := `{"action":"opened"}`
	if err := SaveEventPayload(db, "/repo/a", "ci.yml", updated); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _ = GetEventPayload(db, "/repo/a", "ci.yml")
	if got != updated {
		t.Fatalf("expected overwritten value %q, got %q", updated, got)
	}
}

func TestEventPayload_SavingEmptyClearsRow(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := SaveEventPayload(db, "/repo/a", "ci.yml", `{"action":"labeled"}`); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := SaveEventPayload(db, "/repo/a", "ci.yml", ""); err != nil {
		t.Fatalf("save empty: %v", err)
	}
	got, err := GetEventPayload(db, "/repo/a", "ci.yml")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "" {
		t.Fatalf("expected cleared payload, got %q", got)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM event_payloads WHERE repo_path = ? AND workflow_file = ?`, "/repo/a", "ci.yml").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected row deleted, found %d rows", count)
	}
}
