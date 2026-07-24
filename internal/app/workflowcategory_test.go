package app

import (
	"path/filepath"
	"testing"
)

func TestWorkflowCategories_SaveGetAndClear(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	got, err := GetWorkflowCategories(db, "/repo/a")
	if err != nil {
		t.Fatalf("get before save: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map before any override, got %v", got)
	}

	if err := SaveWorkflowCategory(db, "/repo/a", "ci.yml", "Testing"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := SaveWorkflowCategory(db, "/repo/a", "deploy.yml", "Deployment"); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Different repo is independent.
	if err := SaveWorkflowCategory(db, "/repo/b", "ci.yml", "Security"); err != nil {
		t.Fatalf("save other repo: %v", err)
	}

	got, err = GetWorkflowCategories(db, "/repo/a")
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	want := map[string]string{"ci.yml": "Testing", "deploy.yml": "Deployment"}
	if len(got) != len(want) || got["ci.yml"] != want["ci.yml"] || got["deploy.yml"] != want["deploy.yml"] {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Overwrite.
	if err := SaveWorkflowCategory(db, "/repo/a", "ci.yml", "CI/Build"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _ = GetWorkflowCategories(db, "/repo/a")
	if got["ci.yml"] != "CI/Build" {
		t.Fatalf("expected overwritten category, got %q", got["ci.yml"])
	}

	// Clearing with an empty category deletes the row.
	if err := SaveWorkflowCategory(db, "/repo/a", "ci.yml", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ = GetWorkflowCategories(db, "/repo/a")
	if _, ok := got["ci.yml"]; ok {
		t.Fatalf("expected ci.yml override cleared, got %v", got)
	}
	if got["deploy.yml"] != "Deployment" {
		t.Fatalf("expected deploy.yml override to remain, got %v", got)
	}
}
