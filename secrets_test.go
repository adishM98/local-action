package main

import (
	"path/filepath"
	"testing"
)

func TestSecrets_UpsertListGetDelete(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	key := make([]byte, keySize)

	if err := UpsertSecret(db, key, "/repo/a", KindSecret, "TOKEN", "abc123"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	entries, err := ListSecrets(db, "/repo/a", KindSecret)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "TOKEN" {
		t.Fatalf("expected one entry named TOKEN, got %+v", entries)
	}

	value, err := GetSecretValue(db, key, "/repo/a", KindSecret, "TOKEN")
	if err != nil {
		t.Fatalf("get value: %v", err)
	}
	if value != "abc123" {
		t.Fatalf("expected abc123, got %q", value)
	}

	// overwrite
	if err := UpsertSecret(db, key, "/repo/a", KindSecret, "TOKEN", "xyz789"); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	value, _ = GetSecretValue(db, key, "/repo/a", KindSecret, "TOKEN")
	if value != "xyz789" {
		t.Fatalf("expected updated value xyz789, got %q", value)
	}

	// scoping: same key name under a different repo path is independent
	if err := UpsertSecret(db, key, "/repo/b", KindSecret, "TOKEN", "other-repo-value"); err != nil {
		t.Fatalf("upsert repo b: %v", err)
	}
	valueB, _ := GetSecretValue(db, key, "/repo/b", KindSecret, "TOKEN")
	if valueB != "other-repo-value" {
		t.Fatalf("expected repo b to have its own value, got %q", valueB)
	}

	if err := DeleteSecret(db, "/repo/a", KindSecret, "TOKEN"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, _ = ListSecrets(db, "/repo/a", KindSecret)
	if len(entries) != 0 {
		t.Fatalf("expected no entries after delete, got %+v", entries)
	}
}
