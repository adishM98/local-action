package secrets

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"local-action/internal/db"
)

func TestSecrets_UpsertListGetDelete(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	key := make([]byte, KeySize)

	if err := UpsertSecret(db, key, "/repo/a", KindSecret, "TOKEN", "abc123", "", true); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	entries, err := ListSecrets(db, "/repo/a", KindSecret)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "TOKEN" {
		t.Fatalf("expected one entry named TOKEN, got %+v", entries)
	}

	value, err := GetSecretValue(db, key, "/repo/a", KindSecret, "TOKEN", "")
	if err != nil {
		t.Fatalf("get value: %v", err)
	}
	if value != "abc123" {
		t.Fatalf("expected abc123, got %q", value)
	}

	// overwrite
	if err := UpsertSecret(db, key, "/repo/a", KindSecret, "TOKEN", "xyz789", "", true); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	value, _ = GetSecretValue(db, key, "/repo/a", KindSecret, "TOKEN", "")
	if value != "xyz789" {
		t.Fatalf("expected updated value xyz789, got %q", value)
	}

	// scoping: same key name under a different repo path is independent
	if err := UpsertSecret(db, key, "/repo/b", KindSecret, "TOKEN", "other-repo-value", "", true); err != nil {
		t.Fatalf("upsert repo b: %v", err)
	}
	valueB, _ := GetSecretValue(db, key, "/repo/b", KindSecret, "TOKEN", "")
	if valueB != "other-repo-value" {
		t.Fatalf("expected repo b to have its own value, got %q", valueB)
	}

	if err := DeleteSecret(db, "/repo/a", KindSecret, "TOKEN", ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, _ = ListSecrets(db, "/repo/a", KindSecret)
	if len(entries) != 0 {
		t.Fatalf("expected no entries after delete, got %+v", entries)
	}
	// ListSecrets must return an empty slice, not nil, for a zero-row result:
	// encoding/json marshals nil as `null`, which crashes SecretsPanel.jsx's
	// entries.map() for a brand new repo with no secrets yet.
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entries: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("expected empty ListSecrets to marshal as [], got %s", b)
	}
}

func TestListSecrets_EmptyMarshalsAsEmptyArray(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	entries, err := ListSecrets(db, "/repo/brand-new", KindSecret)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %+v", entries)
	}
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entries: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("expected empty ListSecrets to marshal as [], got %s", b)
	}
}

func TestSecretsForRun_WorkflowOverridesRepoWide(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	key := make([]byte, KeySize)
	const wf = ".github/workflows/ci.yml"

	// repo-wide FOO and BAR, workflow-specific FOO for ci.yml,
	// workflow-specific BAZ for a DIFFERENT workflow.
	mustUpsert := func(name, value, workflowFile string) {
		t.Helper()
		if err := UpsertSecret(db, key, "/repo/a", KindSecret, name, value, workflowFile, true); err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
	}
	mustUpsert("FOO", "repo-value", "")
	mustUpsert("BAR", "bar-value", "")
	mustUpsert("FOO", "ci-value", wf)
	mustUpsert("BAZ", "other-value", ".github/workflows/other.yml")

	got, err := SecretsForRun(db, key, "/repo/a", wf, KindSecret)
	if err != nil {
		t.Fatalf("SecretsForRun: %v", err)
	}
	want := map[string]string{"FOO": "ci-value", "BAR": "bar-value"}
	if len(got) != len(want) || got["FOO"] != want["FOO"] || got["BAR"] != want["BAR"] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSecretsForRun_CorruptedValueFails(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	key := make([]byte, KeySize)

	if _, err := db.Exec(
		`INSERT INTO secrets (repo_path, kind, key, workflow_file, value_encrypted) VALUES (?, ?, ?, '', ?)`,
		"/repo/a", string(KindSecret), "BAD", []byte("short"),
	); err != nil {
		t.Fatalf("insert corrupted: %v", err)
	}
	if _, err := SecretsForRun(db, key, "/repo/a", "ci.yml", KindSecret); err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
}
