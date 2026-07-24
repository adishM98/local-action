# GitHub-Style UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild local-action's UI as a GitHub Actions clone (workflow sidebar, runs list, run detail with collapsible job/step logs), add repo-wide vs per-workflow secret scoping, and fix docker-health UX.

**Architecture:** Backend (Go, `internal/app`) gains a `workflow_file` column on `secrets`, a merge function with workflow-over-repo precedence, `--json` on the act argv, and a `dockerError` field on `/api/health`. Frontend (Vite/React, `cmd/local-action/web/src`) is restructured: new `TopBar`/`Sidebar`/`RunsView`/`RunWorkflowMenu`/`RunDetail`/`SecretsPage` components replace `PromptBar`/`WorkflowsPanel`/`HistoryPanel`/`LogViewer`/`SecretsPanel`; a pure-JS log parser groups act's JSON log lines into jobs/steps client-side. Spec: `docs/superpowers/specs/2026-07-24-github-style-ui-design.md`.

**Tech Stack:** Go 1.25, modernc.org/sqlite, React 18, Vite 5, `node:test` for the JS parser. NO new dependencies anywhere.

## Global Constraints

- No new npm dependencies, no new Go modules. `node:test` (built into Node) is the only JS test runner.
- All log storage/transport stays raw lines; parsing is client-side only.
- Single dark theme. Palette: bg `#0d1117`, panel `#161b22`, border `#30363d`, text `#e6edf2`, muted `#8b949e`, green `#3fb950`, red `#f85149`, yellow `#d29922`, blue `#4493f8`.
- Existing behavior preserved: one run at a time, localhost bind, AES-256-GCM secrets, WS raw-line streaming.
- Commands run from repo root `/Users/adishm/Downloads/local-action` unless a `cd` is shown.
- Every task ends with `go test ./...` (backend) or `cd cmd/local-action/web && npm run build` (frontend) green before commit.

---

### Task 1: act argv gets `--json`

**Files:**
- Modify: `internal/app/actrunner_argv.go`
- Test: `internal/app/actrunner_argv_test.go`

**Interfaces:**
- Produces: `BuildArgv(req RunRequest, secretFile, varFile string) []string` now emits `--json` right after `-W <file>`. All later log-parsing work relies on act emitting JSON lines.

- [ ] **Step 1: Update both existing tests to expect `--json`**

In `internal/app/actrunner_argv_test.go`, change the two `want` slices:

```go
	want := []string{"push", "-W", ".github/workflows/ci.yml", "--json", "--secret-file", "/tmp/secrets.env", "--var-file", "/tmp/vars.env"}
```

and

```go
	want := []string{
		"workflow_dispatch", "-W", "deploy.yml", "--json", "--secret-file", "/tmp/s.env", "--var-file", "/tmp/v.env",
		"--input", "alpha=hello world",
		"--input", "zeta=1",
	}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/app/ -run TestBuildArgv -v`
Expected: both FAIL (missing `--json`).

- [ ] **Step 3: Implement**

In `internal/app/actrunner_argv.go` change the argv line:

```go
	argv := []string{req.Event, "-W", req.WorkflowFile, "--json", "--secret-file", secretFile, "--var-file", varFile}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -run TestBuildArgv -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/actrunner_argv.go internal/app/actrunner_argv_test.go
git commit -m "feat: run act with --json for structured job/step log lines"
```

---

### Task 2: ScanWorkflows returns `[]`, never `nil`

**Files:**
- Modify: `internal/app/scanner.go`
- Test: `internal/app/scanner_test.go`

**Interfaces:**
- Produces: `ScanWorkflows(repoPath string) ([]WorkflowInfo, error)` — zero-workflow results marshal as JSON `[]`, not `null`. Frontend stops needing the `result || []` guard (keep the guard anyway, harmless).

- [ ] **Step 1: Write the failing test**

Append to `internal/app/scanner_test.go` (add `"encoding/json"` to its imports):

```go
func TestScanWorkflows_EmptyResultsMarshalAsEmptyArray(t *testing.T) {
	// No workflows dir at all.
	repo := t.TempDir()
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	b, _ := json.Marshal(workflows)
	if string(b) != "[]" {
		t.Fatalf("no-dir case: expected [], got %s", b)
	}

	// Empty workflows dir.
	repo2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo2, ".github", "workflows"), 0755); err != nil {
		t.Fatal(err)
	}
	workflows, err = ScanWorkflows(repo2)
	if err != nil {
		t.Fatalf("scan empty dir: %v", err)
	}
	b, _ = json.Marshal(workflows)
	if string(b) != "[]" {
		t.Fatalf("empty-dir case: expected [], got %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run TestScanWorkflows_EmptyResultsMarshalAsEmptyArray -v`
Expected: FAIL with `expected [], got null`.

- [ ] **Step 3: Implement**

In `internal/app/scanner.go`, inside `ScanWorkflows`:

```go
	dir := filepath.Join(repoPath, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []WorkflowInfo{}, nil
	}
	if err != nil {
		return nil, err
	}

	results := []WorkflowInfo{}
```

(Two changes: the `return nil, nil` becomes `return []WorkflowInfo{}, nil`, and `var results []WorkflowInfo` becomes `results := []WorkflowInfo{}`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -run TestScanWorkflows -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/scanner.go internal/app/scanner_test.go
git commit -m "fix: ScanWorkflows returns empty slice instead of nil"
```

---

### Task 3: secrets schema migration — `workflow_file` column

**Files:**
- Modify: `internal/app/db.go`
- Test: `internal/app/db_test.go`

**Interfaces:**
- Produces: `secrets` table shape `(repo_path, kind, key, workflow_file, value_encrypted)` with `PRIMARY KEY (repo_path, kind, key, workflow_file)` and `workflow_file TEXT NOT NULL DEFAULT ''`. `''` means repo-wide. Old DBs migrate on `OpenDB`; old rows get `workflow_file = ''`.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Append to `internal/app/db_test.go` (add `"database/sql"` to its imports):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run TestOpenDB_MigratesOldSecretsSchema -v`
Expected: FAIL with `no such column: workflow_file`.

- [ ] **Step 3: Implement**

In `internal/app/db.go`, replace the `secrets` definition inside the `schema` const:

```go
CREATE TABLE IF NOT EXISTS secrets (
  repo_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  key TEXT NOT NULL,
  workflow_file TEXT NOT NULL DEFAULT '',
  value_encrypted BLOB NOT NULL,
  PRIMARY KEY (repo_path, kind, key, workflow_file)
);
```

Add after the `schema` exec in `OpenDB` (before `return db, nil`):

```go
	if err := migrateSecretsWorkflowFile(db); err != nil {
		db.Close()
		return nil, err
	}
```

Add at the bottom of the file:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -run 'TestOpenDB' -v`
Expected: both `TestOpenDB_CreatesSchemaIdempotently` and `TestOpenDB_MigratesOldSecretsSchema` PASS.

- [ ] **Step 5: Full package test, then commit**

Run: `go test ./internal/app/`
Expected: PASS (nothing else touches the column yet).

```bash
git add internal/app/db.go internal/app/db_test.go
git commit -m "feat: add workflow_file column to secrets with startup migration"
```

---

### Task 4: workflow-scoped secrets CRUD + precedence merge

**Files:**
- Modify: `internal/app/secrets.go`
- Modify: `internal/app/actrunner.go` (writeTempFiles/writeDotenvTemp)
- Test: `internal/app/secrets_test.go`, `internal/app/actrunner_test.go`

**Interfaces:**
- Produces (all in package `app`; every signature below is exact — later tasks call these):
  - `type SecretEntry struct { RepoPath string; Kind SecretKind; Key string; WorkflowFile string }` — `WorkflowFile` has json tag `workflowFile`, `''` = repo-wide.
  - `UpsertSecret(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, value, workflowFile string) error`
  - `ListSecrets(db *sql.DB, repoPath string, kind SecretKind) ([]SecretEntry, error)` — returns BOTH scopes, ordered by `workflow_file, key`.
  - `GetSecretValue(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, workflowFile string) (string, error)`
  - `DeleteSecret(db *sql.DB, repoPath string, kind SecretKind, name, workflowFile string) error`
  - `SecretsForRun(db *sql.DB, encKey []byte, repoPath, workflowFile string, kind SecretKind) (map[string]string, error)` — merged repo-wide + workflow-specific, workflow wins on name clash.
- Consumes: Task 3's schema.

- [ ] **Step 1: Write the failing precedence test**

Append to `internal/app/secrets_test.go`:

```go
func TestSecretsForRun_WorkflowOverridesRepoWide(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	key := make([]byte, keySize)
	const wf = ".github/workflows/ci.yml"

	// repo-wide FOO and BAR, workflow-specific FOO for ci.yml,
	// workflow-specific BAZ for a DIFFERENT workflow.
	mustUpsert := func(name, value, workflowFile string) {
		t.Helper()
		if err := UpsertSecret(db, key, "/repo/a", KindSecret, name, value, workflowFile); err != nil {
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
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	key := make([]byte, keySize)

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
```

- [ ] **Step 2: Run tests to verify compilation fails**

Run: `go test ./internal/app/ -run TestSecretsForRun -v`
Expected: compile error (`UpsertSecret` arity, `SecretsForRun` undefined).

- [ ] **Step 3: Rewrite `internal/app/secrets.go`**

Replace the whole file:

```go
package app

import (
	"database/sql"
)

type SecretKind string

const (
	KindSecret SecretKind = "secret"
	KindVar    SecretKind = "var"
)

type SecretEntry struct {
	RepoPath     string     `json:"repoPath"`
	Kind         SecretKind `json:"kind"`
	Key          string     `json:"key"`
	WorkflowFile string     `json:"workflowFile"` // "" = repo-wide (all workflows in the repo)
}

func UpsertSecret(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, value, workflowFile string) error {
	ciphertext, err := Encrypt(encKey, []byte(value))
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO secrets (repo_path, kind, key, workflow_file, value_encrypted) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(repo_path, kind, key, workflow_file) DO UPDATE SET value_encrypted = excluded.value_encrypted`,
		repoPath, string(kind), name, workflowFile, ciphertext,
	)
	return err
}

// ListSecrets returns every entry for the repo, both repo-wide and
// workflow-scoped. Callers filter by WorkflowFile as needed.
func ListSecrets(db *sql.DB, repoPath string, kind SecretKind) ([]SecretEntry, error) {
	rows, err := db.Query(
		`SELECT repo_path, kind, key, workflow_file FROM secrets WHERE repo_path = ? AND kind = ? ORDER BY workflow_file, key`,
		repoPath, string(kind),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []SecretEntry{}
	for rows.Next() {
		var e SecretEntry
		var kindStr string
		if err := rows.Scan(&e.RepoPath, &kindStr, &e.Key, &e.WorkflowFile); err != nil {
			return nil, err
		}
		e.Kind = SecretKind(kindStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func GetSecretValue(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, workflowFile string) (string, error) {
	var ciphertext []byte
	err := db.QueryRow(
		`SELECT value_encrypted FROM secrets WHERE repo_path = ? AND kind = ? AND key = ? AND workflow_file = ?`,
		repoPath, string(kind), name, workflowFile,
	).Scan(&ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := Decrypt(encKey, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func DeleteSecret(db *sql.DB, repoPath string, kind SecretKind, name, workflowFile string) error {
	_, err := db.Exec(
		`DELETE FROM secrets WHERE repo_path = ? AND kind = ? AND key = ? AND workflow_file = ?`,
		repoPath, string(kind), name, workflowFile,
	)
	return err
}

// SecretsForRun returns the decrypted values injected into a run of
// workflowFile: repo-wide entries overlaid by workflow-specific entries
// (workflow wins on name clash). The ORDER BY puts '' (repo-wide) first so
// the overlay is a simple map overwrite.
func SecretsForRun(db *sql.DB, encKey []byte, repoPath, workflowFile string, kind SecretKind) (map[string]string, error) {
	rows, err := db.Query(
		`SELECT key, value_encrypted FROM secrets
		 WHERE repo_path = ? AND kind = ? AND workflow_file IN ('', ?)
		 ORDER BY workflow_file, key`,
		repoPath, string(kind), workflowFile,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var name string
		var ciphertext []byte
		if err := rows.Scan(&name, &ciphertext); err != nil {
			return nil, err
		}
		plaintext, err := Decrypt(encKey, ciphertext)
		if err != nil {
			return nil, err
		}
		values[name] = string(plaintext)
	}
	return values, rows.Err()
}
```

- [ ] **Step 4: Update `internal/app/actrunner.go` to use the merge**

Replace `writeTempFiles` and `writeDotenvTemp`:

```go
func (e *Engine) writeTempFiles(req RunRequest) (secretFile, varFile string, cleanup func(), err error) {
	secrets, err := SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, KindSecret)
	if err != nil {
		return "", "", nil, err
	}
	vars, err := SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, KindVar)
	if err != nil {
		return "", "", nil, err
	}

	sf, err := writeDotenvTemp("act-secrets-*.env", secrets, req.ExtraSecrets)
	if err != nil {
		return "", "", nil, err
	}
	vf, err := writeDotenvTemp("act-vars-*.env", vars, req.ExtraVars)
	if err != nil {
		os.Remove(sf)
		return "", "", nil, err
	}
	cleanup = func() {
		os.Remove(sf)
		os.Remove(vf)
	}
	return sf, vf, cleanup, nil
}

func writeDotenvTemp(pattern string, values, extra map[string]string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(name)
		}
	}()
	if err := f.Chmod(0600); err != nil {
		return "", err
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(f, "%s=%s\n", k, values[k])
	}
	for k, v := range extra {
		fmt.Fprintf(f, "%s=%s\n", k, v)
	}
	success = true
	return name, nil
}
```

Add `"sort"` to `internal/app/actrunner.go`'s imports. (`writeDotenvTemp` is no longer a method — decryption now happens in `SecretsForRun`, so it needs no Engine state.)

- [ ] **Step 5: Fix broken call sites in tests**

1. `internal/app/secrets_test.go` — existing `TestSecrets_UpsertListGetDelete`: add `""` as the final argument to every `UpsertSecret(...)`, `GetSecretValue(...)`, and `DeleteSecret(...)` call (7 calls).
2. `internal/app/actrunner_test.go` — DELETE the entire `TestWriteDotenvTemp_CleansUpTempFileOnError` function. Its scenario (decrypt failure mid-file-write) no longer exists: decryption now completes in `SecretsForRun` before any temp file is created, and the new `TestSecretsForRun_CorruptedValueFails` covers the failure. If other tests in the file call `UpsertSecret`, add the `""` argument there too. Verify with:

Run: `grep -n 'UpsertSecret\|GetSecretValue\|DeleteSecret(' internal/app/*_test.go`
Expected: every call ends with a workflowFile argument.

- [ ] **Step 6: Run the full package**

Run: `go test ./internal/app/ -v`
Expected: all PASS, including both new `TestSecretsForRun_*` tests.

- [ ] **Step 7: Commit**

```bash
git add internal/app/secrets.go internal/app/actrunner.go internal/app/secrets_test.go internal/app/actrunner_test.go
git commit -m "feat: workflow-scoped secrets with workflow-over-repo precedence"
```

---

### Task 5: API — workflowFile on secrets endpoints, dockerError on health

**Files:**
- Modify: `internal/app/api.go`
- Test: `internal/app/api_test.go`

**Interfaces:**
- Produces (HTTP contract the frontend consumes):
  - `GET /api/secrets?repoPath=&kind=` → `[{repoPath, kind, key, workflowFile}]`
  - `POST /api/secrets` body `{repoPath, kind, key, value, workflowFile}` (`workflowFile` optional, defaults `""`)
  - `DELETE /api/secrets` body `{repoPath, kind, key, workflowFile}`
  - `GET /api/health` → `{actOK, actVersion, dockerOK, dockerError?}` — `dockerError` present only when `dockerOK` is false.

- [ ] **Step 1: Write the failing test**

Append to `internal/app/api_test.go`:

```go
func TestAPI_WorkflowScopedSecrets(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	post := func(payload map[string]string) {
		t.Helper()
		body, _ := json.Marshal(payload)
		resp, err := http.Post(server.URL+"/api/secrets", "application/json", bytes.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("upsert %v: err=%v status=%v", payload, err, resp.StatusCode)
		}
	}
	post(map[string]string{"repoPath": "/r", "kind": "secret", "key": "FOO", "value": "repo"})
	post(map[string]string{"repoPath": "/r", "kind": "secret", "key": "FOO", "value": "ci", "workflowFile": "ci.yml"})

	resp, err := http.Get(server.URL + "/api/secrets?repoPath=/r&kind=secret")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var entries []SecretEntry
	json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (repo-wide + workflow), got %+v", entries)
	}
	if entries[0].WorkflowFile != "" || entries[1].WorkflowFile != "ci.yml" {
		t.Fatalf("expected ordering repo-wide then ci.yml, got %+v", entries)
	}

	// Delete only the workflow-scoped one.
	delBody, _ := json.Marshal(map[string]string{"repoPath": "/r", "kind": "secret", "key": "FOO", "workflowFile": "ci.yml"})
	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/secrets", bytes.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: err=%v status=%v", err, resp.StatusCode)
	}
	resp, _ = http.Get(server.URL + "/api/secrets?repoPath=/r&kind=secret")
	entries = nil
	json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) != 1 || entries[0].WorkflowFile != "" {
		t.Fatalf("expected only the repo-wide entry to remain, got %+v", entries)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/ -run TestAPI_WorkflowScopedSecrets -v`
Expected: compile error — the handlers call `UpsertSecret`/`DeleteSecret` with old arity. (api.go was NOT yet updated for Task 4's signatures, so the whole package fails to build; that's the failure signal here.)

Note: if Task 4 left api.go compiling by some other means, the test fails at the second `post(...)` instead. Either failure is the correct red state.

- [ ] **Step 3: Update `internal/app/api.go`**

Health handler — replace the docker line and response:

```go
		actOut, actErr := exec.CommandContext(ctx, actBin, "--version").CombinedOutput()
		dockerOut, dockerErr := exec.CommandContext(ctx, "docker", "info").CombinedOutput()

		resp := map[string]any{
			"actOK":      actErr == nil,
			"actVersion": string(actOut),
			"dockerOK":   dockerErr == nil,
		}
		if dockerErr != nil {
			msg := strings.TrimSpace(string(dockerOut))
			if msg == "" {
				msg = dockerErr.Error()
			}
			if len(msg) > 300 {
				msg = msg[:300]
			}
			resp["dockerError"] = msg
		}
		writeJSON(w, http.StatusOK, resp)
```

Add `"strings"` to api.go's imports.

`POST /api/secrets` — add the field and pass it through:

```go
		var body struct {
			RepoPath     string     `json:"repoPath"`
			Kind         SecretKind `json:"kind"`
			Key          string     `json:"key"`
			Value        string     `json:"value"`
			WorkflowFile string     `json:"workflowFile"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := UpsertSecret(db, key, body.RepoPath, body.Kind, body.Key, body.Value, body.WorkflowFile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
```

`DELETE /api/secrets` — same shape:

```go
		var body struct {
			RepoPath     string     `json:"repoPath"`
			Kind         SecretKind `json:"kind"`
			Key          string     `json:"key"`
			WorkflowFile string     `json:"workflowFile"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := DeleteSecret(db, body.RepoPath, body.Kind, body.Key, body.WorkflowFile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
```

`GET /api/secrets` needs no change (ListSecrets signature unchanged).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/ -v`
Expected: all PASS (including the pre-existing `TestAPI_ScanSecretsAndRunLifecycle`, whose 4-field upsert body now just means `workflowFile: ""`).

- [ ] **Step 5: Commit**

```bash
git add internal/app/api.go internal/app/api_test.go
git commit -m "feat: workflowFile on secrets API, dockerError detail on health"
```

---

### Task 6: frontend log parser + format helpers (pure JS, node:test)

**Files:**
- Create: `cmd/local-action/web/src/logparse.js`
- Create: `cmd/local-action/web/src/logparse.test.js`
- Create: `cmd/local-action/web/src/format.js`
- Modify: `cmd/local-action/web/package.json` (add test script)

**Interfaces:**
- Produces:
  - `parseLogLines(lines: string[]) → { jobs: Job[], other: Line[] }` where `Line = {no, text}`, `Job = {id, name, result, steps: Step[], tail: Line[]}`, `Step = {name, result, lines: Line[]}`. `result` values come straight from act: `"success"`, `"failure"`, `"skipped"`, or `null` while unresolved.
  - `relativeTime(unixSeconds) → string` ("just now", "5m ago", "2h ago", "3d ago")
  - `duration(run) → string` ("42s", "3m 5s") — accepts the backend Run JSON where `startedAt`/`finishedAt` are Go `sql.NullInt64` objects `{Int64, Valid}`.

- [ ] **Step 1: Add the test script**

In `cmd/local-action/web/package.json` scripts:

```json
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "test": "node --test src/"
  },
```

- [ ] **Step 2: Write the failing tests**

Create `cmd/local-action/web/src/logparse.test.js`:

```js
import test from 'node:test'
import assert from 'node:assert/strict'
import { parseLogLines } from './logparse.js'

const line = (obj) => JSON.stringify(obj)

test('groups lines into jobs and steps with results', () => {
  const lines = [
    line({ msg: 'Using docker host ...', level: 'info' }),
    line({ msg: '⭐ Run Set up job', job: 'Hello/greet', jobID: 'greet', step: 'Set up job' }),
    line({ msg: '🚀 Start image=x', job: 'Hello/greet', jobID: 'greet', step: 'Set up job' }),
    line({ msg: '✅ Success - Set up job', jobID: 'greet', job: 'Hello/greet', step: 'Set up job', stepResult: 'success' }),
    line({ msg: '⭐ Run Main echo hi', jobID: 'greet', job: 'Hello/greet', step: 'echo hi' }),
    line({ msg: 'hi\n', jobID: 'greet', job: 'Hello/greet', step: 'echo hi', raw_output: true }),
    line({ msg: '✅ Success - Main echo hi', jobID: 'greet', job: 'Hello/greet', step: 'echo hi', stepResult: 'success' }),
    line({ msg: '🏁 Job succeeded', jobID: 'greet', job: 'Hello/greet', jobResult: 'success' }),
  ]
  const { jobs, other } = parseLogLines(lines)

  assert.equal(other.length, 1) // the docker-host line has no jobID
  assert.equal(other[0].no, 1)

  assert.equal(jobs.length, 1)
  const job = jobs[0]
  assert.equal(job.id, 'greet')
  assert.equal(job.name, 'Hello/greet')
  assert.equal(job.result, 'success')
  assert.equal(job.tail.length, 1) // 🏁 line: has jobID, no step

  assert.deepEqual(job.steps.map((s) => s.name), ['Set up job', 'echo hi'])
  assert.equal(job.steps[0].result, 'success')
  assert.equal(job.steps[1].lines.some((l) => l.text === 'hi'), true) // trailing \n stripped
})

test('non-JSON lines land in other, order preserved', () => {
  const { jobs, other } = parseLogLines(['plain text', 'Error: something broke'])
  assert.equal(jobs.length, 0)
  assert.deepEqual(other, [
    { no: 1, text: 'plain text' },
    { no: 2, text: 'Error: something broke' },
  ])
})

test('unresolved step has null result', () => {
  const { jobs } = parseLogLines([
    line({ msg: '⭐ Run Main build', jobID: 'b', job: 'CI/b', step: 'build' }),
  ])
  assert.equal(jobs[0].steps[0].result, null)
  assert.equal(jobs[0].result, null)
})

test('JSON line without msg string is treated as raw text', () => {
  const { jobs, other } = parseLogLines(['{"foo": 1}', '42'])
  assert.equal(jobs.length, 0)
  assert.equal(other.length, 2)
})
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd cmd/local-action/web && npm test`
Expected: FAIL — cannot find module `./logparse.js`.

- [ ] **Step 4: Implement `cmd/local-action/web/src/logparse.js`**

```js
// Groups raw act --json log lines into GitHub-Actions-shaped jobs/steps.
// Lines that aren't JSON (old runs, act's bare stderr) fall into `other`.
export function parseLogLines(lines) {
  const jobs = []
  const jobsById = new Map()
  const other = []

  lines.forEach((text, i) => {
    const no = i + 1
    let entry
    try {
      entry = JSON.parse(text)
    } catch {
      other.push({ no, text })
      return
    }
    if (!entry || typeof entry !== 'object' || typeof entry.msg !== 'string') {
      other.push({ no, text })
      return
    }
    const msg = entry.msg.replace(/\n$/, '')
    if (!entry.jobID) {
      other.push({ no, text: msg })
      return
    }

    let job = jobsById.get(entry.jobID)
    if (!job) {
      job = {
        id: entry.jobID,
        name: entry.job || entry.jobID,
        result: null,
        steps: [],
        stepsByName: new Map(),
        tail: [],
      }
      jobsById.set(entry.jobID, job)
      jobs.push(job)
    }
    if (entry.jobResult) job.result = entry.jobResult

    if (!entry.step) {
      job.tail.push({ no, text: msg })
      return
    }
    let step = job.stepsByName.get(entry.step)
    if (!step) {
      step = { name: entry.step, result: null, lines: [] }
      job.stepsByName.set(entry.step, step)
      job.steps.push(step)
    }
    step.lines.push({ no, text: msg })
    if (entry.stepResult) step.result = entry.stepResult
  })

  return { jobs: jobs.map(({ stepsByName, ...job }) => job), other }
}
```

- [ ] **Step 5: Implement `cmd/local-action/web/src/format.js`**

```js
function unwrap(nullable) {
  if (nullable == null) return null
  if (typeof nullable === 'number') return nullable
  return nullable.Valid ? nullable.Int64 : null
}

export function relativeTime(unixSeconds) {
  if (!unixSeconds) return ''
  const delta = Math.floor(Date.now() / 1000) - unixSeconds
  if (delta < 60) return 'just now'
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`
  return `${Math.floor(delta / 86400)}d ago`
}

export function duration(run) {
  const start = unwrap(run.startedAt)
  if (!start) return ''
  const end = unwrap(run.finishedAt) ?? Math.floor(Date.now() / 1000)
  const secs = Math.max(0, end - start)
  const m = Math.floor(secs / 60)
  const s = secs % 60
  return m ? `${m}m ${s}s` : `${s}s`
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd cmd/local-action/web && npm test`
Expected: all 4 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/local-action/web/src/logparse.js cmd/local-action/web/src/logparse.test.js cmd/local-action/web/src/format.js cmd/local-action/web/package.json
git commit -m "feat: client-side act --json log parser and time format helpers"
```

---

### Task 7: api.js, StatusIcon, and the GitHub dark stylesheet

**Files:**
- Modify: `cmd/local-action/web/src/api.js`
- Create: `cmd/local-action/web/src/components/StatusIcon.jsx`
- Modify (full replace): `cmd/local-action/web/src/style.css`

**Interfaces:**
- Produces:
  - `api.upsertSecret(repoPath, kind, key, value, workflowFile = '')`, `api.deleteSecret(repoPath, kind, key, workflowFile = '')` — other api methods unchanged.
  - `<StatusIcon status={'success'|'failed'|'cancelled'|'running'|'queued'|'skipped'} />`
  - CSS classes used by Tasks 8–10 (exact names): `top-bar`, `top-bar__logo`, `top-bar__path`, `health`, `health__item`, `dot dot--{ok,bad,pending}`, `shell`, `sidebar`, `sidebar__heading`, `sidebar__item`, `sidebar__note`, `sidebar__note--error`, `sidebar__spacer`, `content`, `runs-view`, `runs-view__head`, `banner banner--warn`, `run-rows`, `run-row`, `run-row__main`, `run-row__name`, `run-row__meta`, `run-row__duration`, `run-menu`, `run-menu__panel`, `field`, `btn`, `btn--primary`, `btn--block`, `linklike`, `run-detail`, `run-detail__head`, `run-detail__meta`, `run-detail__actions`, `job-card`, `job-card__head`, `job-card__tail`, `step`, `step__header`, `step__chevron`, `step__name`, `step__lines`, `secrets-page`, `tabs`, `tab`, `field-row`, `secret-table`, `scope-badge`, `status-icon status-icon--{success,failed,cancelled,running,queued,skipped}`, `error`, `empty-state`.

Note: after this task the OLD components render unstyled until Task 10 switches them out. `npm run build` must still pass; visuals are transiently broken and that's expected.

- [ ] **Step 1: Update `cmd/local-action/web/src/api.js`**

Replace the two secret mutators:

```js
  upsertSecret: (repoPath, kind, key, value, workflowFile = '') =>
    request('POST', '/api/secrets', { repoPath, kind, key, value, workflowFile }),
  deleteSecret: (repoPath, kind, key, workflowFile = '') =>
    request('DELETE', '/api/secrets', { repoPath, kind, key, workflowFile }),
```

- [ ] **Step 2: Create `cmd/local-action/web/src/components/StatusIcon.jsx`**

```jsx
const GLYPH = {
  success: '✓',
  failed: '✕',
  failure: '✕',
  cancelled: '⊘',
  skipped: '⊘',
  running: '●',
  queued: '○',
}

// Normalizes act's step/job results (success/failure/skipped) and run
// statuses (success/failed/cancelled/running/queued) onto one icon set.
export default function StatusIcon({ status }) {
  const s = status === 'failure' ? 'failed' : status === 'skipped' ? 'cancelled' : status || 'queued'
  return <span className={`status-icon status-icon--${s}`}>{GLYPH[status] || GLYPH[s] || '○'}</span>
}
```

- [ ] **Step 3: Replace `cmd/local-action/web/src/style.css` entirely**

```css
:root {
  --bg: #0d1117;
  --panel: #161b22;
  --panel-2: #1c2129;
  --border: #30363d;
  --text: #e6edf2;
  --muted: #8b949e;
  --green: #3fb950;
  --red: #f85149;
  --yellow: #d29922;
  --blue: #4493f8;
  --font: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans', Helvetica, Arial, sans-serif;
  --mono: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font-family: var(--font);
  font-size: 14px;
}

button { font: inherit; color: inherit; }

h2 { font-size: 20px; font-weight: 600; margin: 0; }
h3 { font-size: 14px; font-weight: 600; margin: 0; }

.app { display: flex; flex-direction: column; height: 100vh; }

/* ── Top bar ─────────────────────────────────────────── */
.top-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
}
.top-bar__logo { font-weight: 600; white-space: nowrap; }
.top-bar__path {
  flex: 1;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--text);
  font-family: var(--mono);
  font-size: 13px;
  padding: 6px 10px;
}
.top-bar__path:focus { outline: none; border-color: var(--blue); }

.health { display: flex; gap: 4px; }
.health__item {
  display: flex;
  align-items: center;
  gap: 6px;
  background: none;
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 4px 8px;
  color: var(--muted);
  cursor: pointer;
  font-size: 12px;
}
.health__item:hover { border-color: var(--border); }

.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--muted); }
.dot--ok { background: var(--green); }
.dot--bad { background: var(--red); }
.dot--pending { background: var(--yellow); }

/* ── Shell / sidebar ─────────────────────────────────── */
.shell { display: flex; flex: 1; min-height: 0; }

.sidebar {
  display: flex;
  flex-direction: column;
  width: 260px;
  padding: 16px 8px;
  border-right: 1px solid var(--border);
  overflow-y: auto;
}
.sidebar__heading {
  font-size: 12px;
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  padding: 4px 8px;
}
.sidebar__item {
  display: block;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  border-radius: 6px;
  padding: 6px 8px;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sidebar__item:hover { background: var(--panel-2); }
.sidebar__item.active { background: var(--panel-2); font-weight: 600; box-shadow: inset 2px 0 0 var(--blue); }
.sidebar__note { color: var(--muted); font-size: 12px; padding: 4px 8px; }
.sidebar__note--error { color: var(--red); }
.sidebar__spacer { flex: 1; }

.content { flex: 1; overflow-y: auto; padding: 24px 32px; }

/* ── Runs list ───────────────────────────────────────── */
.runs-view__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.banner {
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 8px 12px;
  margin-bottom: 12px;
  font-size: 13px;
}
.banner--warn { border-color: var(--yellow); color: var(--yellow); background: rgba(210, 153, 34, 0.08); }

.run-rows { border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
.run-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  border-bottom: 1px solid var(--border);
  padding: 10px 16px;
  cursor: pointer;
}
.run-row:last-child { border-bottom: none; }
.run-row:hover { background: var(--panel); }
.run-row__main { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.run-row__name { font-weight: 600; }
.run-row__meta { color: var(--muted); font-size: 12px; }
.run-row__duration { color: var(--muted); font-size: 12px; white-space: nowrap; }

/* ── Status icons ────────────────────────────────────── */
.status-icon { font-weight: 700; width: 18px; text-align: center; flex-shrink: 0; }
.status-icon--success { color: var(--green); }
.status-icon--failed { color: var(--red); }
.status-icon--cancelled { color: var(--muted); }
.status-icon--queued { color: var(--muted); }
.status-icon--running { color: var(--yellow); animation: pulse 1.2s ease-in-out infinite; }
@keyframes pulse { 50% { opacity: 0.35; } }

/* ── Buttons / fields ────────────────────────────────── */
.btn {
  background: var(--panel-2);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 5px 14px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
}
.btn:hover { border-color: var(--muted); }
.btn:disabled { opacity: 0.5; cursor: default; }
.btn--primary { background: #238636; border-color: #2ea043; color: #fff; }
.btn--primary:hover { background: #2ea043; }
.btn--block { width: 100%; }

.linklike {
  background: none;
  border: none;
  color: var(--blue);
  cursor: pointer;
  padding: 0;
  font-size: 13px;
  text-align: left;
}
.linklike:hover { text-decoration: underline; }

.field { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; font-size: 13px; }
.field > span { font-weight: 600; }
.field small { color: var(--muted); }
.field input, .field select, .field-row select, .secrets-page input, .secrets-page select {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--text);
  padding: 5px 10px;
  font-size: 13px;
}

/* ── Run workflow menu ───────────────────────────────── */
.run-menu { position: relative; }
.run-menu__panel {
  position: absolute;
  right: 0;
  top: calc(100% + 6px);
  z-index: 10;
  width: 320px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 14px;
  box-shadow: 0 8px 24px rgba(1, 4, 9, 0.85);
}

/* ── Run detail ──────────────────────────────────────── */
.run-detail__head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 10px 0 16px;
}
.run-detail__meta { color: var(--muted); font-size: 13px; flex: 1; }
.run-detail__actions { display: flex; gap: 8px; }

.job-card {
  border: 1px solid var(--border);
  border-radius: 6px;
  margin-bottom: 16px;
  overflow: hidden;
}
.job-card__head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
}
.job-card__tail {
  padding: 6px 14px;
  color: var(--muted);
  font-family: var(--mono);
  font-size: 12px;
}

.step { border-bottom: 1px solid var(--border); }
.step:last-child { border-bottom: none; }
.step__header {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  padding: 8px 14px;
  cursor: pointer;
}
.step__header:hover { background: var(--panel-2); }
.step__chevron { color: var(--muted); width: 12px; }
.step__name { font-size: 13px; }
.step__lines {
  margin: 0;
  padding: 8px 14px 8px 0;
  background: #0a0d12;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.7;
  color: #c9d1d9;
  overflow-x: auto;
  list-style-position: inside;
}
.step__lines li { white-space: pre-wrap; padding-left: 46px; }
.step__lines li::marker { color: var(--muted); }

/* ── Secrets page ────────────────────────────────────── */
.secrets-page { max-width: 760px; }
.tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border); margin: 16px 0; }
.tab {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 8px 14px;
  cursor: pointer;
  color: var(--muted);
}
.tab.active { color: var(--text); font-weight: 600; border-bottom-color: #f78166; }

.field-row { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; font-size: 13px; }

.secret-table { width: 100%; border-collapse: collapse; margin-bottom: 20px; }
.secret-table th {
  text-align: left;
  font-size: 12px;
  color: var(--muted);
  border-bottom: 1px solid var(--border);
  padding: 6px 10px;
}
.secret-table td { border-bottom: 1px solid var(--border); padding: 8px 10px; font-family: var(--mono); font-size: 13px; }

.scope-badge {
  display: inline-block;
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 1px 10px;
  font-size: 12px;
  font-family: var(--font);
  color: var(--muted);
}
.scope-badge--wf { color: var(--blue); border-color: var(--blue); }

/* ── Misc ────────────────────────────────────────────── */
.error { color: var(--red); font-size: 13px; }
.empty-state { color: var(--muted); padding: 24px 0; }
```

- [ ] **Step 4: Verify the build**

Run: `cd cmd/local-action/web && npm run build`
Expected: build succeeds (old components still compile; they just look unstyled until Task 10).

- [ ] **Step 5: Commit**

```bash
git add cmd/local-action/web/src/api.js cmd/local-action/web/src/components/StatusIcon.jsx cmd/local-action/web/src/style.css
git commit -m "feat: GitHub dark stylesheet, StatusIcon, workflowFile in api client"
```

---

### Task 8: RunWorkflowMenu + RunsView

**Files:**
- Create: `cmd/local-action/web/src/components/RunWorkflowMenu.jsx`
- Create: `cmd/local-action/web/src/components/RunsView.jsx`

**Interfaces:**
- Consumes: `api` (Task 7), `StatusIcon` (Task 7), `relativeTime`/`duration` (Task 6).
- Produces:
  - `<RunWorkflowMenu repoPath workflow onStarted(runId) onOpenSecrets(workflowFile) />` — `workflow` is one `WorkflowInfo` `{file, name, events, dispatchInputs, parseError}`.
  - `<RunsView repoPath workflows workflowFile health onOpenRun(runId) onOpenSecrets(workflowFile) />` — `workflowFile` null = all workflows.
- Not yet imported by App — build stays green.

- [ ] **Step 1: Create `cmd/local-action/web/src/components/RunWorkflowMenu.jsx`**

```jsx
import { useEffect, useRef, useState } from 'react'
import { api } from '../api.js'

export default function RunWorkflowMenu({ repoPath, workflow, onStarted, onOpenSecrets }) {
  const [open, setOpen] = useState(false)
  const [event, setEvent] = useState(workflow.events?.[0] || '')
  const [inputs, setInputs] = useState({})
  const [counts, setCounts] = useState(null)
  const [error, setError] = useState(null)
  const [starting, setStarting] = useState(false)
  const ref = useRef(null)

  useEffect(() => {
    if (!open) return
    function onDocClick(e) {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false)
    }
    document.addEventListener('mousedown', onDocClick)
    return () => document.removeEventListener('mousedown', onDocClick)
  }, [open])

  useEffect(() => {
    if (!open) return
    Promise.all([api.listSecrets(repoPath, 'secret'), api.listSecrets(repoPath, 'var')])
      .then(([secrets, vars]) => {
        const relevant = (list) =>
          (list || []).filter((e) => !e.workflowFile || e.workflowFile === workflow.file).length
        setCounts({ secrets: relevant(secrets), vars: relevant(vars) })
      })
      .catch(() => setCounts(null))
  }, [open, repoPath, workflow.file])

  async function run() {
    setStarting(true)
    setError(null)
    try {
      const { runId } = await api.createRun({
        repoPath,
        workflowFile: workflow.file,
        event,
        inputs,
      })
      setOpen(false)
      onStarted(runId)
    } catch (err) {
      setError(err.message)
    } finally {
      setStarting(false)
    }
  }

  const dispatchInputs = event === 'workflow_dispatch' ? workflow.dispatchInputs || [] : []

  return (
    <div className="run-menu" ref={ref}>
      <button className="btn btn--primary" onClick={() => setOpen(!open)}>
        Run workflow ▾
      </button>
      {open && (
        <div className="run-menu__panel">
          <label className="field">
            <span>Event</span>
            <select value={event} onChange={(e) => setEvent(e.target.value)}>
              {(workflow.events || []).map((ev) => (
                <option key={ev} value={ev}>
                  {ev}
                </option>
              ))}
            </select>
          </label>
          {dispatchInputs.map((input) => (
            <label className="field" key={input.name}>
              <span>
                {input.name}
                {input.required ? ' *' : ''}
              </span>
              {input.type === 'choice' && input.options?.length ? (
                <select
                  value={inputs[input.name] ?? input.default ?? ''}
                  onChange={(e) => setInputs({ ...inputs, [input.name]: e.target.value })}
                >
                  {input.options.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  placeholder={input.default}
                  value={inputs[input.name] || ''}
                  onChange={(e) => setInputs({ ...inputs, [input.name]: e.target.value })}
                />
              )}
              {input.description && <small>{input.description}</small>}
            </label>
          ))}
          {counts && (
            <p>
              <button className="linklike" onClick={() => onOpenSecrets(workflow.file)}>
                {counts.secrets} secret{counts.secrets === 1 ? '' : 's'}, {counts.vars} var
                {counts.vars === 1 ? '' : 's'} will be injected
              </button>
            </p>
          )}
          {error && <p className="error">{error}</p>}
          <button className="btn btn--primary btn--block" onClick={run} disabled={!event || starting}>
            {starting ? 'Starting…' : 'Run workflow'}
          </button>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Create `cmd/local-action/web/src/components/RunsView.jsx`**

```jsx
import { useEffect, useState } from 'react'
import { api } from '../api.js'
import StatusIcon from './StatusIcon.jsx'
import RunWorkflowMenu from './RunWorkflowMenu.jsx'
import { relativeTime, duration } from '../format.js'

export default function RunsView({ repoPath, workflows, workflowFile, health, onOpenRun, onOpenSecrets }) {
  const [runs, setRuns] = useState([])
  const [error, setError] = useState(null)

  useEffect(() => {
    if (!repoPath) return
    let cancelled = false
    async function load() {
      try {
        const result = await api.listRuns(repoPath)
        if (!cancelled) {
          setRuns(result || [])
          setError(null)
        }
      } catch (err) {
        if (!cancelled) setError(err.message)
      }
    }
    load()
    const interval = setInterval(load, 2000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [repoPath])

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to get started.</p>
  }

  const workflow = workflowFile ? workflows.find((w) => w.file === workflowFile) : null
  const visible = workflowFile ? runs.filter((r) => r.workflowFile === workflowFile) : runs
  const wfName = (file) => workflows.find((w) => w.file === file)?.name || file

  return (
    <div className="runs-view">
      <div className="runs-view__head">
        <h2>{workflow ? workflow.name : 'All workflows'}</h2>
        {workflow && !workflow.parseError && (
          <RunWorkflowMenu
            repoPath={repoPath}
            workflow={workflow}
            onStarted={onOpenRun}
            onOpenSecrets={onOpenSecrets}
          />
        )}
      </div>
      {workflow?.parseError && <p className="error">{workflow.parseError}</p>}
      {health && health.dockerOK === false && (
        <div className="banner banner--warn">Docker is not running — workflow runs will fail.</div>
      )}
      {error && <p className="error">{error}</p>}
      <div className="run-rows">
        {visible.length === 0 && !error && <p className="empty-state">No runs yet.</p>}
        {visible.map((run) => (
          <button key={run.id} className="run-row" onClick={() => onOpenRun(run.id)}>
            <StatusIcon status={run.status} />
            <span className="run-row__main">
              <span className="run-row__name">
                {wfName(run.workflowFile)} #{run.id}
              </span>
              <span className="run-row__meta">
                {run.event} · {relativeTime(run.createdAt)}
              </span>
            </span>
            <span className="run-row__duration">{duration(run)}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Verify the build**

Run: `cd cmd/local-action/web && npm run build`
Expected: success (files exist but nothing imports them yet — Vite only builds from the entry graph, so this mainly catches syntax errors via the old graph still compiling; full check comes in Task 10).

- [ ] **Step 4: Commit**

```bash
git add cmd/local-action/web/src/components/RunWorkflowMenu.jsx cmd/local-action/web/src/components/RunsView.jsx
git commit -m "feat: GitHub-style runs list and run-workflow dropdown"
```

---

### Task 9: RunDetail with job/step log rendering

**Files:**
- Create: `cmd/local-action/web/src/components/RunDetail.jsx`

**Interfaces:**
- Consumes: `api`, `StatusIcon`, `parseLogLines`, `relativeTime`, `duration`.
- Produces: `<RunDetail runId onBack() onOpenRun(runId) />` — `onOpenRun` is used by Re-run to jump to the new run.

- [ ] **Step 1: Create `cmd/local-action/web/src/components/RunDetail.jsx`**

```jsx
import { useEffect, useMemo, useState } from 'react'
import { api } from '../api.js'
import StatusIcon from './StatusIcon.jsx'
import { relativeTime, duration } from '../format.js'
import { parseLogLines } from '../logparse.js'

const TERMINAL = ['success', 'failed', 'cancelled']

// While the run is live: WS streams lines, a 2s poll tracks status. On any
// terminal poll the persisted log replaces the streamed lines wholesale, so
// WS hiccups can't lose output — SQLite is the source of truth at the end.
export default function RunDetail({ runId, onBack, onOpenRun }) {
  const [run, setRun] = useState(null)
  const [lines, setLines] = useState([])
  const [error, setError] = useState(null)
  const [wsDown, setWsDown] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    setRun(null)
    setLines([])
    setError(null)
    setWsDown(false)
    let cancelled = false
    let socket = null
    let interval = null
    let wsFailed = false

    function stopLive() {
      if (interval) clearInterval(interval)
      if (socket) socket.close()
      interval = null
      socket = null
    }

    async function poll() {
      try {
        const result = await api.getRun(runId)
        if (cancelled) return
        setRun(result.run)
        if (TERMINAL.includes(result.run.status)) {
          setLines(result.logs || [])
          stopLive()
        } else if (wsFailed) {
          setLines(result.logs || [])
        }
      } catch (err) {
        if (!cancelled) setError(`Couldn't load this run: ${err.message}`)
      }
    }

    api
      .getRun(runId)
      .then((result) => {
        if (cancelled) return
        setRun(result.run)
        if (TERMINAL.includes(result.run.status)) {
          setLines(result.logs || [])
          return
        }
        const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
        socket = new WebSocket(`${protocol}://${window.location.host}/ws/runs/${runId}`)
        socket.onmessage = (event) => setLines((prev) => [...prev, event.data])
        socket.onerror = () => {
          wsFailed = true
          if (!cancelled) setWsDown(true)
        }
        interval = setInterval(poll, 2000)
      })
      .catch((err) => {
        if (!cancelled) setError(`Couldn't load this run: ${err.message}`)
      })

    return () => {
      cancelled = true
      stopLive()
    }
  }, [runId])

  const parsed = useMemo(() => parseLogLines(lines), [lines])
  const isTerminal = run && TERMINAL.includes(run.status)

  async function cancel() {
    setBusy(true)
    setError(null)
    try {
      await api.cancelRun(runId)
    } catch (err) {
      setError(`Couldn't cancel: ${err.message}`)
    } finally {
      setBusy(false)
    }
  }

  async function rerun() {
    setBusy(true)
    setError(null)
    try {
      let inputs = {}
      try {
        inputs = JSON.parse(run.inputs || '{}')
      } catch {
        inputs = {}
      }
      const { runId: newId } = await api.createRun({
        repoPath: run.repoPath,
        workflowFile: run.workflowFile,
        event: run.event,
        inputs,
      })
      onOpenRun(newId)
    } catch (err) {
      setError(`Couldn't re-run: ${err.message}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="run-detail">
      <button className="linklike" onClick={onBack}>
        ← All runs
      </button>
      <div className="run-detail__head">
        <StatusIcon status={run?.status} />
        <h2>{run ? `${run.workflowFile} #${run.id}` : `Run #${runId}`}</h2>
        {run && (
          <span className="run-detail__meta">
            {run.event} · {relativeTime(run.createdAt)}
            {duration(run) && ` · ${duration(run)}`}
          </span>
        )}
        <div className="run-detail__actions">
          {run && !isTerminal && (
            <button className="btn" onClick={cancel} disabled={busy}>
              Cancel
            </button>
          )}
          {isTerminal && (
            <button className="btn" onClick={rerun} disabled={busy}>
              Re-run
            </button>
          )}
        </div>
      </div>
      {wsDown && run && !isTerminal && (
        <div className="banner banner--warn">
          Live stream lost — falling back to polling. Output may lag a couple of seconds.
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {parsed.jobs.map((job) => (
        <JobCard key={job.id} job={job} runStatus={run?.status} />
      ))}
      {parsed.other.length > 0 && (
        <JobCard
          job={{
            id: '_other',
            name: 'Output',
            result: null,
            steps: [{ name: 'Raw output', result: null, lines: parsed.other }],
            tail: [],
          }}
          runStatus={run?.status}
        />
      )}
      {lines.length === 0 && !error && <p className="empty-state">Waiting for output…</p>}
    </div>
  )
}

// null result while the run is live means "in progress"; after the run ends
// an unresolved step just never ran (queued glyph, muted).
function liveStatus(result, runStatus) {
  if (result) return result
  return runStatus === 'running' ? 'running' : 'queued'
}

function JobCard({ job, runStatus }) {
  return (
    <section className="job-card">
      <header className="job-card__head">
        <StatusIcon status={liveStatus(job.result, runStatus)} />
        <h3>{job.name}</h3>
      </header>
      {job.steps.map((step) => (
        <StepRow key={step.name} step={step} runStatus={runStatus} />
      ))}
      {job.tail.length > 0 && (
        <div className="job-card__tail">
          {job.tail.map((l) => (
            <div key={l.no}>{l.text}</div>
          ))}
        </div>
      )}
    </section>
  )
}

function StepRow({ step, runStatus }) {
  // GitHub behavior: steps auto-expand while unresolved or failed,
  // auto-collapse on success. A manual toggle always wins afterwards.
  const [userOpen, setUserOpen] = useState(null)
  const autoOpen = step.result == null || step.result === 'failure'
  const open = userOpen ?? autoOpen

  return (
    <div className="step">
      <button className="step__header" onClick={() => setUserOpen(!open)}>
        <span className="step__chevron">{open ? '▾' : '▸'}</span>
        <StatusIcon status={liveStatus(step.result, runStatus)} />
        <span className="step__name">{step.name}</span>
      </button>
      {open && (
        <ol className="step__lines">
          {step.lines.map((l) => (
            <li key={l.no} value={l.no}>
              {l.text}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Verify the build**

Run: `cd cmd/local-action/web && npm run build`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add cmd/local-action/web/src/components/RunDetail.jsx
git commit -m "feat: run detail page with collapsible job/step log sections"
```

---

### Task 10: SecretsPage, TopBar, Sidebar, new App — the switchover

**Files:**
- Create: `cmd/local-action/web/src/components/SecretsPage.jsx`
- Create: `cmd/local-action/web/src/components/TopBar.jsx`
- Create: `cmd/local-action/web/src/components/Sidebar.jsx`
- Modify (full replace): `cmd/local-action/web/src/App.jsx`
- Delete: `cmd/local-action/web/src/components/PromptBar.jsx`, `WorkflowsPanel.jsx`, `SecretsPanel.jsx`, `HistoryPanel.jsx`, `LogViewer.jsx`

**Interfaces:**
- Consumes: everything from Tasks 6–9.
- Produces: the running app. View state shape (owned by App): `{name: 'runs', workflowFile: string|null}` | `{name: 'run', runId, workflowFile}` | `{name: 'secrets', workflowFile: string|null}`.

- [ ] **Step 1: Create `cmd/local-action/web/src/components/TopBar.jsx`**

```jsx
import { useState } from 'react'

const RECENT_KEY = 'recentRepoPaths'
const MAX_RECENT = 8

function rememberPath(path) {
  if (!path) return
  const existing = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')
  const next = [path, ...existing.filter((p) => p !== path)].slice(0, MAX_RECENT)
  localStorage.setItem(RECENT_KEY, JSON.stringify(next))
}

function HealthDot({ label, ok, error, onClick }) {
  const state = ok == null ? 'pending' : ok ? 'ok' : 'bad'
  const title =
    ok == null
      ? `checking ${label}…`
      : ok
        ? `${label} ready — click to recheck`
        : `${label} not available${error ? `: ${error}` : ''} — click to recheck`
  return (
    <button className="health__item" title={title} onClick={onClick}>
      <span className={`dot dot--${state}`} />
      {label}
    </button>
  )
}

export default function TopBar({ repoPath, onCommit, health, onRecheck }) {
  const [draft, setDraft] = useState(repoPath)
  const recentPaths = JSON.parse(localStorage.getItem(RECENT_KEY) || '[]')

  function commit() {
    const path = draft.trim()
    if (path && path !== repoPath) {
      rememberPath(path)
      onCommit(path)
    }
  }

  return (
    <header className="top-bar">
      <span className="top-bar__logo">local-action</span>
      <input
        className="top-bar__path"
        list="recent-repo-paths"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => e.key === 'Enter' && commit()}
        placeholder="/path/to/repo"
        spellCheck={false}
      />
      <datalist id="recent-repo-paths">
        {recentPaths.map((p) => (
          <option key={p} value={p} />
        ))}
      </datalist>
      <div className="health">
        <HealthDot label="act" ok={health?.actOK} onClick={onRecheck} />
        <HealthDot label="docker" ok={health?.dockerOK} error={health?.dockerError} onClick={onRecheck} />
      </div>
    </header>
  )
}
```

- [ ] **Step 2: Create `cmd/local-action/web/src/components/Sidebar.jsx`**

```jsx
export default function Sidebar({ workflows, scanState, view, onNavigate }) {
  const inRuns = view.name === 'runs' || view.name === 'run'
  return (
    <nav className="sidebar">
      <div className="sidebar__heading">Actions</div>
      <button
        className={`sidebar__item${inRuns && !view.workflowFile ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'runs', workflowFile: null })}
      >
        All workflows
      </button>
      {workflows.map((wf) => (
        <button
          key={wf.file}
          className={`sidebar__item${inRuns && view.workflowFile === wf.file ? ' active' : ''}`}
          onClick={() => onNavigate({ name: 'runs', workflowFile: wf.file })}
          title={wf.file}
        >
          {wf.name}
        </button>
      ))}
      {scanState.error && <p className="sidebar__note sidebar__note--error">{scanState.error}</p>}
      {scanState.scanned && !scanState.error && workflows.length === 0 && (
        <p className="sidebar__note">No workflows under .github/workflows.</p>
      )}
      <div className="sidebar__spacer" />
      <div className="sidebar__heading">Settings</div>
      <button
        className={`sidebar__item${view.name === 'secrets' ? ' active' : ''}`}
        onClick={() => onNavigate({ name: 'secrets', workflowFile: null })}
      >
        Secrets and variables
      </button>
    </nav>
  )
}
```

- [ ] **Step 3: Create `cmd/local-action/web/src/components/SecretsPage.jsx`**

```jsx
import { useEffect, useState } from 'react'
import { api } from '../api.js'

export default function SecretsPage({ repoPath, workflows, initialWorkflowFilter }) {
  const [kind, setKind] = useState('secret')
  const [entries, setEntries] = useState([])
  const [filter, setFilter] = useState(initialWorkflowFilter || '')
  const [name, setName] = useState('')
  const [value, setValue] = useState('')
  const [scope, setScope] = useState(initialWorkflowFilter || '')
  const [error, setError] = useState(null)

  async function load() {
    if (!repoPath) return
    try {
      setEntries((await api.listSecrets(repoPath, kind)) || [])
      setError(null)
    } catch (err) {
      setError(err.message)
    }
  }

  useEffect(() => {
    load()
  }, [repoPath, kind])

  async function save() {
    setError(null)
    try {
      await api.upsertSecret(repoPath, kind, name, value, scope)
      setName('')
      setValue('')
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  async function remove(entry) {
    setError(null)
    try {
      await api.deleteSecret(repoPath, kind, entry.key, entry.workflowFile || '')
      load()
    } catch (err) {
      setError(err.message)
    }
  }

  if (!repoPath) {
    return <p className="empty-state">Enter a repo path above to manage secrets.</p>
  }

  const noun = kind === 'secret' ? 'secret' : 'variable'
  const visible = filter ? entries.filter((e) => !e.workflowFile || e.workflowFile === filter) : entries
  const wfName = (file) => workflows.find((w) => w.file === file)?.name || file

  return (
    <div className="secrets-page">
      <h2>Secrets and variables</h2>
      <div className="tabs">
        <button className={`tab${kind === 'secret' ? ' active' : ''}`} onClick={() => setKind('secret')}>
          Secrets
        </button>
        <button className={`tab${kind === 'var' ? ' active' : ''}`} onClick={() => setKind('var')}>
          Variables
        </button>
      </div>
      {workflows.length > 0 && (
        <div className="field-row">
          <label>Filter by workflow</label>
          <select value={filter} onChange={(e) => setFilter(e.target.value)}>
            <option value="">All</option>
            {workflows.map((w) => (
              <option key={w.file} value={w.file}>
                {w.name}
              </option>
            ))}
          </select>
        </div>
      )}
      {error && <p className="error">{error}</p>}
      {visible.length === 0 ? (
        <p className="empty-state">No {noun}s stored yet.</p>
      ) : (
        <table className="secret-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Scope</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {visible.map((entry) => (
              <tr key={`${entry.key}|${entry.workflowFile}`}>
                <td>{entry.key}</td>
                <td>
                  {entry.workflowFile ? (
                    <span className="scope-badge scope-badge--wf" title={entry.workflowFile}>
                      {wfName(entry.workflowFile)}
                    </span>
                  ) : (
                    <span className="scope-badge">Repository</span>
                  )}
                </td>
                <td>
                  <button className="linklike" onClick={() => remove(entry)}>
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <h3>New {noun}</h3>
      <div className="field">
        <span>Name</span>
        <input placeholder="KEY" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <span>Value</span>
        <input placeholder="value (write-only after save)" value={value} onChange={(e) => setValue(e.target.value)} />
      </div>
      <div className="field">
        <span>Scope</span>
        <label>
          <input type="radio" checked={scope === ''} onChange={() => setScope('')} /> All workflows in this
          repo
        </label>
        <label>
          <input
            type="radio"
            checked={scope !== ''}
            disabled={workflows.length === 0}
            onChange={() => setScope(workflows[0]?.file || '')}
          />{' '}
          Specific workflow
        </label>
        {scope !== '' && (
          <select value={scope} onChange={(e) => setScope(e.target.value)}>
            {workflows.map((w) => (
              <option key={w.file} value={w.file}>
                {w.name}
              </option>
            ))}
          </select>
        )}
      </div>
      <button className="btn btn--primary" onClick={save} disabled={!name}>
        Add {noun}
      </button>
    </div>
  )
}
```

- [ ] **Step 4: Replace `cmd/local-action/web/src/App.jsx`**

```jsx
import { useCallback, useEffect, useState } from 'react'
import { api } from './api.js'
import TopBar from './components/TopBar.jsx'
import Sidebar from './components/Sidebar.jsx'
import RunsView from './components/RunsView.jsx'
import RunDetail from './components/RunDetail.jsx'
import SecretsPage from './components/SecretsPage.jsx'

export default function App() {
  const [repoPath, setRepoPath] = useState(localStorage.getItem('repoPath') || '')
  const [workflows, setWorkflows] = useState([])
  const [scanState, setScanState] = useState({ scanned: false, error: null })
  const [view, setView] = useState({ name: 'runs', workflowFile: null })
  const [health, setHealth] = useState(null)

  const checkHealth = useCallback(async () => {
    try {
      setHealth(await api.health())
    } catch {
      setHealth({ actOK: false, dockerOK: false, dockerError: 'server unreachable' })
    }
  }, [])

  useEffect(() => {
    checkHealth()
  }, [checkHealth])

  // Poll fast while unhealthy so a booting Docker Desktop turns the dot
  // green within seconds; back off once everything is fine.
  useEffect(() => {
    if (!health) return
    const healthy = health.actOK && health.dockerOK
    const id = setTimeout(checkHealth, healthy ? 30000 : 5000)
    return () => clearTimeout(id)
  }, [health, checkHealth])

  const scan = useCallback(async (path) => {
    if (!path) return
    try {
      const result = await api.scan(path)
      setWorkflows(result || [])
      setScanState({ scanned: true, error: null })
    } catch (err) {
      setWorkflows([])
      setScanState({ scanned: true, error: err.message })
    }
  }, [])

  useEffect(() => {
    scan(repoPath)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // initial scan for the remembered path only; later scans go through commitRepoPath

  function commitRepoPath(path) {
    setRepoPath(path)
    localStorage.setItem('repoPath', path)
    setView({ name: 'runs', workflowFile: null })
    scan(path)
  }

  return (
    <div className="app">
      <TopBar repoPath={repoPath} onCommit={commitRepoPath} health={health} onRecheck={checkHealth} />
      <div className="shell">
        <Sidebar workflows={workflows} scanState={scanState} view={view} onNavigate={setView} />
        <main className="content">
          {view.name === 'runs' && (
            <RunsView
              repoPath={repoPath}
              workflows={workflows}
              workflowFile={view.workflowFile}
              health={health}
              onOpenRun={(runId) => setView({ name: 'run', runId, workflowFile: view.workflowFile })}
              onOpenSecrets={(workflowFile) => setView({ name: 'secrets', workflowFile })}
            />
          )}
          {view.name === 'run' && (
            <RunDetail
              runId={view.runId}
              onBack={() => setView({ name: 'runs', workflowFile: view.workflowFile || null })}
              onOpenRun={(runId) => setView({ name: 'run', runId, workflowFile: view.workflowFile })}
            />
          )}
          {view.name === 'secrets' && (
            <SecretsPage
              repoPath={repoPath}
              workflows={workflows}
              initialWorkflowFilter={view.workflowFile || ''}
            />
          )}
        </main>
      </div>
    </div>
  )
}
```

- [ ] **Step 5: Delete the old components**

```bash
git rm cmd/local-action/web/src/components/PromptBar.jsx cmd/local-action/web/src/components/WorkflowsPanel.jsx cmd/local-action/web/src/components/SecretsPanel.jsx cmd/local-action/web/src/components/HistoryPanel.jsx cmd/local-action/web/src/components/LogViewer.jsx
```

- [ ] **Step 6: Build and run JS tests**

Run: `cd cmd/local-action/web && npm run build && npm test`
Expected: build succeeds, 4 parser tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/local-action/web/src
git commit -m "feat: GitHub Actions-style layout — sidebar, runs list, run detail, secrets page"
```

---

### Task 11: end-to-end verification

**Files:** none (verification only; fix-forward anything found).

- [ ] **Step 1: Full backend suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 2: Format and vet**

Run: `make fmt`
Expected: `gofmt -l .` prints nothing; `go vet` clean.

- [ ] **Step 3: Build the whole app**

Run: `make build`
Expected: frontend builds into `cmd/local-action/web/dist`, Go binary `local-action` produced.

- [ ] **Step 4: Smoke test against the sample repo**

```bash
./local-action -db /tmp/smoke-test.db -addr 127.0.0.1:8091 &
sleep 1
curl -s http://127.0.0.1:8091/api/health
curl -s -X POST http://127.0.0.1:8091/api/scan -d '{"path":"'$PWD'/testdata/sample-repo"}'
curl -s -X POST http://127.0.0.1:8091/api/secrets -d '{"repoPath":"/r","kind":"secret","key":"K","value":"v","workflowFile":"ci.yml"}' -o /dev/null -w '%{http_code}\n'
curl -s 'http://127.0.0.1:8091/api/secrets?repoPath=/r&kind=secret'
kill %1
rm -f /tmp/smoke-test.db
```

Expected: health JSON includes `dockerOK` (and `dockerError` only if docker is down); scan returns the Hello workflow with `workflow_dispatch` inputs; secrets POST returns 204; list shows `"workflowFile":"ci.yml"`.

- [ ] **Step 5: Manual UI check (user-visible)**

Run: `make run`, open `http://localhost:8090`, then verify against the spec:
1. GitHub-dark layout: sidebar with workflow names, runs list, no Scan button (typing a path + Enter scans).
2. "Run workflow ▾" on a workflow → trigger → lands on run detail with live collapsible steps.
3. Secrets page: add repo-wide and workflow-scoped entries, badges correct, delete works.
4. Stop Docker Desktop → dot goes red within ~5s with a reason in the tooltip and a banner on the runs view; start it → green again.

- [ ] **Step 6: Commit any fixes, then final commit if needed**

```bash
git status
```

Expected: clean tree (or commit fixes with descriptive messages).

---

## Self-Review Notes

- Spec coverage: layout/nav (T10), run detail + `--json` parsing (T1, T6, T9), secrets scoping (T3–T5, T10), health UX (T5, T10), scanner wart (T2), re-run (T9), WS fallback (T9), error states (T8–T10), tests (T2–T6). Removed features (Scan button, History tab, old panels) die in T10.
- Type consistency: `SecretEntry.WorkflowFile` json `workflowFile` used by api.js/SecretsPage/RunWorkflowMenu; `parseLogLines` return shape matches RunDetail's `JobCard`/`StepRow` props; view-state shape identical in App/Sidebar/RunsView.
- Old runs (pre-`--json` logs) render via the `other` catch-all — verified path in T6 test 2.
