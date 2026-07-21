# local-action Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary, selfhosted web app that wraps `act` (nektos/act) so a user can run GitHub Actions workflows locally via a browser UI instead of the CLI — with secrets/vars management, live log streaming, and run history.

**Architecture:** Go backend (single package `main`) embeds a built React SPA via `go:embed`, exposes a REST + WebSocket API, shells out to the `act` CLI per run through a single-worker FIFO queue, and persists secrets (encrypted) and run history in an embedded SQLite database.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite` (pure Go, no cgo), `gopkg.in/yaml.v3`, `github.com/gorilla/websocket`, React + Vite (no router/state library).

## Global Constraints

- Go 1.22+ required (uses `net/http.ServeMux` method+path patterns and `r.PathValue`).
- SQLite driver must be pure-Go (`modernc.org/sqlite`) — no cgo — so the final binary stays a portable single static binary.
- Go module name: `local-action`.
- Secrets/vars are only ever written to disk as short-lived temp dotenv files during a run (0600 perms), deleted immediately after the run finishes.
- Encryption key lives at `os.UserConfigDir()/local-action/seed.key`, 0600 perms, generated once on first run.
- Only one `act` process runs at a time (FIFO queue) — no concurrent runs.
- No external services beyond Docker + the `act` binary, both assumed pre-installed on the host (app runs as a native binary, not containerized).
- Frontend: React + Vite, no react-router or state-management library — plain `useState`/tab switching is sufficient for this scope.
- Repo input is a local filesystem path only — no git clone/URL/auth handling anywhere in this plan.

## Implementation Note (deviation from spec, flagged during planning)

The design spec called for a SQLite-backed mtime cache of parsed workflow files. For a personal, single-user tool scanning a handful of workflow files per repo, re-parsing the YAML on every scan request is a few milliseconds and adds no caching-invalidation bugs to maintain. This plan implements scan-on-demand (Task 4) with no cache table. If the repo grows to hundreds of workflow files and scanning becomes noticeably slow, add the cache table back then.

---

### Task 1: Project scaffold + SQLite schema

**Files:**
- Create: `go.mod`
- Create: `db.go`
- Test: `db_test.go`

**Interfaces:**
- Produces: `OpenDB(path string) (*sql.DB, error)` — opens (creating if needed) the SQLite file and applies the schema. Later tasks call this to get their `*sql.DB`.

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
cd /Users/adishm/Downloads/local-action
go mod init local-action
```
Expected: creates `go.mod` with `module local-action` and a `go` directive.

- [ ] **Step 2: Set Go version and add dependencies**

Edit `go.mod` so the top reads:
```
module local-action

go 1.22
```

Run:
```bash
go get modernc.org/sqlite@latest gopkg.in/yaml.v3@latest github.com/gorilla/websocket@latest
```
Expected: `go.mod` gains three `require` lines, `go.sum` is created.

- [ ] **Step 3: Write the failing test**

Create `db_test.go`:
```go
package main

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestOpenDB -v`
Expected: FAIL — `OpenDB` undefined.

- [ ] **Step 3: Implement `db.go`**

Create `db.go`:
```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestOpenDB -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum db.go db_test.go
git commit -m "feat: add SQLite schema and OpenDB"
```

---

### Task 2: Encryption key + AES-GCM encrypt/decrypt

**Files:**
- Create: `crypto.go`
- Test: `crypto_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `DefaultKeyPath() (string, error)`, `LoadOrCreateKey(path string) ([]byte, error)`, `Encrypt(key, plaintext []byte) ([]byte, error)`, `Decrypt(key, data []byte) ([]byte, error)`. Task 3 (secrets store) calls `Encrypt`/`Decrypt`; Task 9 (main.go) calls `DefaultKeyPath`/`LoadOrCreateKey`.

- [ ] **Step 1: Write the failing tests**

Create `crypto_test.go`:
```go
package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateKey_PersistsAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.key")

	key1, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if len(key1) != keySize {
		t.Fatalf("expected key length %d, got %d", keySize, len(key1))
	}

	key2, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if !bytes.Equal(key1, key2) {
		t.Fatal("expected same key to be reloaded from disk")
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key := make([]byte, keySize)
	plaintext := []byte("super-secret-value")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestLoadOrCreateKey|TestEncryptDecrypt' -v`
Expected: FAIL — undefined: `LoadOrCreateKey`, `Encrypt`, `keySize`.

- [ ] **Step 3: Implement `crypto.go`**

Create `crypto.go`:
```go
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const keySize = 32 // AES-256

func DefaultKeyPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local-action", "seed.key"), nil
}

func LoadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		key, decErr := base64.StdEncoding.DecodeString(string(data))
		if decErr != nil || len(key) != keySize {
			return nil, errors.New("seed key file is corrupt")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestLoadOrCreateKey|TestEncryptDecrypt' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add crypto.go crypto_test.go
git commit -m "feat: add encryption key management and AES-GCM helpers"
```

---

### Task 3: Secrets/vars store

**Files:**
- Create: `secrets.go`
- Test: `secrets_test.go`

**Interfaces:**
- Consumes: `OpenDB` (Task 1), `Encrypt`/`Decrypt` (Task 2).
- Produces: `SecretKind` (`KindSecret`, `KindVar`), `SecretEntry{RepoPath, Kind, Key}`, `UpsertSecret(db, encKey, repoPath, kind, name, value string) error`, `ListSecrets(db, repoPath, kind) ([]SecretEntry, error)`, `GetSecretValue(db, encKey, repoPath, kind, name string) (string, error)`, `DeleteSecret(db, repoPath, kind, name string) error`. Task 7 (act engine) calls `ListSecrets`/`GetSecretValue`; Task 9 (API) calls all four.

- [ ] **Step 1: Write the failing tests**

Create `secrets_test.go`:
```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestSecrets_UpsertListGetDelete -v`
Expected: FAIL — undefined: `UpsertSecret`, `KindSecret`, etc.

- [ ] **Step 3: Implement `secrets.go`**

Create `secrets.go`:
```go
package main

import (
	"database/sql"
)

type SecretKind string

const (
	KindSecret SecretKind = "secret"
	KindVar    SecretKind = "var"
)

type SecretEntry struct {
	RepoPath string     `json:"repoPath"`
	Kind     SecretKind `json:"kind"`
	Key      string     `json:"key"`
}

func UpsertSecret(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, value string) error {
	ciphertext, err := Encrypt(encKey, []byte(value))
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO secrets (repo_path, kind, key, value_encrypted) VALUES (?, ?, ?, ?)
		 ON CONFLICT(repo_path, kind, key) DO UPDATE SET value_encrypted = excluded.value_encrypted`,
		repoPath, string(kind), name, ciphertext,
	)
	return err
}

func ListSecrets(db *sql.DB, repoPath string, kind SecretKind) ([]SecretEntry, error) {
	rows, err := db.Query(
		`SELECT repo_path, kind, key FROM secrets WHERE repo_path = ? AND kind = ? ORDER BY key`,
		repoPath, string(kind),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []SecretEntry
	for rows.Next() {
		var e SecretEntry
		var kindStr string
		if err := rows.Scan(&e.RepoPath, &kindStr, &e.Key); err != nil {
			return nil, err
		}
		e.Kind = SecretKind(kindStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func GetSecretValue(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name string) (string, error) {
	var ciphertext []byte
	err := db.QueryRow(
		`SELECT value_encrypted FROM secrets WHERE repo_path = ? AND kind = ? AND key = ?`,
		repoPath, string(kind), name,
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

func DeleteSecret(db *sql.DB, repoPath string, kind SecretKind, name string) error {
	_, err := db.Exec(
		`DELETE FROM secrets WHERE repo_path = ? AND kind = ? AND key = ?`,
		repoPath, string(kind), name,
	)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestSecrets_UpsertListGetDelete -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add secrets.go secrets_test.go
git commit -m "feat: add encrypted secrets/vars store"
```

---

### Task 4: Workflow YAML scanner

**Files:**
- Create: `scanner.go`
- Test: `scanner_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `WorkflowInfo{File, Name, Events, DispatchInputs, ParseError}`, `DispatchInput{Name, Description, Required, Default, Type, Options}`, `ScanWorkflows(repoPath string) ([]WorkflowInfo, error)`. Task 9 (API) calls `ScanWorkflows`.

**Important gotcha this task must handle:** YAML 1.1 scalar resolution can treat a bare `on` mapping key as boolean `true`. Parsing via `yaml.Node` (not into a `map[string]interface{}`) keeps the raw key text (`"on"`), avoiding that bug.

- [ ] **Step 1: Write the failing tests**

Create `scanner_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorkflow(t *testing.T, repoPath, filename, content string) {
	t.Helper()
	dir := filepath.Join(repoPath, ".github", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

func TestScanWorkflows_StringEvent(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}
	wf := workflows[0]
	if wf.Name != "CI" {
		t.Errorf("expected name CI, got %q", wf.Name)
	}
	if len(wf.Events) != 1 || wf.Events[0] != "push" {
		t.Errorf("expected events [push], got %v", wf.Events)
	}
}

func TestScanWorkflows_ListEvents(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: [push, pull_request]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if len(wf.Events) != 2 || wf.Events[0] != "push" || wf.Events[1] != "pull_request" {
		t.Fatalf("expected [push pull_request], got %v", wf.Events)
	}
}

func TestScanWorkflows_MapEventsWithDispatchInputs(t *testing.T) {
	repo := t.TempDir()
	content := `
name: Deploy
on:
  push:
  workflow_dispatch:
    inputs:
      environment:
        description: "Target environment"
        required: true
        default: staging
        type: choice
        options:
          - staging
          - production
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps: []
`
	writeWorkflow(t, repo, "deploy.yml", content)

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if len(wf.Events) != 2 {
		t.Fatalf("expected 2 events, got %v", wf.Events)
	}
	if len(wf.DispatchInputs) != 1 {
		t.Fatalf("expected 1 dispatch input, got %+v", wf.DispatchInputs)
	}
	input := wf.DispatchInputs[0]
	if input.Name != "environment" || !input.Required || input.Default != "staging" || input.Type != "choice" {
		t.Fatalf("unexpected input parsed: %+v", input)
	}
	if len(input.Options) != 2 || input.Options[0] != "staging" || input.Options[1] != "production" {
		t.Fatalf("unexpected options: %v", input.Options)
	}
}

func TestScanWorkflows_InvalidYAML(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "broken.yml", "name: Broken\non: [push\njobs: {}\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan should not fail outright: %v", err)
	}
	if len(workflows) != 1 || workflows[0].ParseError == "" {
		t.Fatalf("expected a parse error on the broken file, got %+v", workflows)
	}
}

func TestScanWorkflows_NoWorkflowsDir(t *testing.T) {
	repo := t.TempDir()

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("expected no error for missing workflows dir, got %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("expected no workflows, got %v", workflows)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestScanWorkflows -v`
Expected: FAIL — undefined: `ScanWorkflows`.

- [ ] **Step 3: Implement `scanner.go`**

Create `scanner.go`:
```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type WorkflowInfo struct {
	File           string          `json:"file"`
	Name           string          `json:"name"`
	Events         []string        `json:"events"`
	DispatchInputs []DispatchInput `json:"dispatchInputs,omitempty"`
	ParseError     string          `json:"parseError,omitempty"`
}

type DispatchInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
}

func ScanWorkflows(repoPath string) ([]WorkflowInfo, error) {
	dir := filepath.Join(repoPath, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var results []WorkflowInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		info, err := ParseWorkflowFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		info.File = filepath.Join(".github", "workflows", name)
		results = append(results, info)
	}
	return results, nil
}

func ParseWorkflowFile(path string) (WorkflowInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowInfo{}, err
	}
	info := WorkflowInfo{Name: filepath.Base(path)}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		info.ParseError = err.Error()
		return info, nil
	}
	if len(doc.Content) == 0 {
		info.ParseError = "empty workflow file"
		return info, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		info.ParseError = "workflow file root is not a mapping"
		return info, nil
	}

	var onNode *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]
		switch keyNode.Value {
		case "name":
			if valNode.Kind == yaml.ScalarNode {
				info.Name = valNode.Value
			}
		case "on":
			onNode = valNode
		}
	}

	if onNode == nil {
		info.ParseError = "no 'on' trigger block found"
		return info, nil
	}

	events, dispatchInputs, err := parseOnNode(onNode)
	if err != nil {
		info.ParseError = err.Error()
		return info, nil
	}
	info.Events = events
	info.DispatchInputs = dispatchInputs
	return info, nil
}

func parseOnNode(n *yaml.Node) ([]string, []DispatchInput, error) {
	var events []string
	var dispatchInputs []DispatchInput

	switch n.Kind {
	case yaml.ScalarNode:
		events = append(events, n.Value)
	case yaml.SequenceNode:
		for _, item := range n.Content {
			events = append(events, item.Value)
		}
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			valNode := n.Content[i+1]
			events = append(events, keyNode.Value)
			if keyNode.Value == "workflow_dispatch" && valNode.Kind == yaml.MappingNode {
				dispatchInputs = parseDispatchInputs(valNode)
			}
		}
	default:
		return nil, nil, fmt.Errorf("unsupported 'on' node kind: %v", n.Kind)
	}
	return events, dispatchInputs, nil
}

func parseDispatchInputs(n *yaml.Node) []DispatchInput {
	var inputs []DispatchInput
	for i := 0; i < len(n.Content); i += 2 {
		keyNode := n.Content[i]
		valNode := n.Content[i+1]
		if keyNode.Value != "inputs" || valNode.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(valNode.Content); j += 2 {
			nameNode := valNode.Content[j]
			specNode := valNode.Content[j+1]
			input := DispatchInput{Name: nameNode.Value, Type: "string"}
			if specNode.Kind == yaml.MappingNode {
				for k := 0; k < len(specNode.Content); k += 2 {
					fk := specNode.Content[k].Value
					fv := specNode.Content[k+1]
					switch fk {
					case "description":
						input.Description = fv.Value
					case "required":
						input.Required = fv.Value == "true"
					case "default":
						input.Default = fv.Value
					case "type":
						input.Type = fv.Value
					case "options":
						for _, opt := range fv.Content {
							input.Options = append(input.Options, opt.Value)
						}
					}
				}
			}
			inputs = append(inputs, input)
		}
	}
	return inputs
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestScanWorkflows -v`
Expected: PASS (all 5 subtests)

- [ ] **Step 5: Commit**

```bash
git add scanner.go scanner_test.go
git commit -m "feat: add workflow YAML scanner"
```

---

### Task 5: Run history store

**Files:**
- Create: `runs.go`
- Test: `runs_test.go`

**Interfaces:**
- Consumes: `OpenDB` (Task 1).
- Produces: `RunStatus` (`StatusQueued`, `StatusRunning`, `StatusSuccess`, `StatusFailed`, `StatusCancelled`), `Run{ID, RepoPath, WorkflowFile, Event, Inputs, Status, StartedAt, FinishedAt, CreatedAt}`, `CreateRun(db, r Run) (int64, error)`, `UpdateRunStatus(db, id int64, status RunStatus, startedAt, finishedAt *int64) error`, `AppendRunLog(db, runID int64, lineNo int, text string) error`, `GetRunLogs(db, runID int64) ([]string, error)`, `GetRun(db, id int64) (Run, error)`, `ListRuns(db, repoPath string) ([]Run, error)`. Task 7 (act engine) calls `CreateRun`, `UpdateRunStatus`, `AppendRunLog`. Task 9 (API) calls `GetRun`, `GetRunLogs`, `ListRuns`.

- [ ] **Step 1: Write the failing tests**

Create `runs_test.go`:
```go
package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRuns_CreateGetUpdateList(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	id, err := CreateRun(db, Run{
		RepoPath:     "/repo/a",
		WorkflowFile: ".github/workflows/ci.yml",
		Event:        "push",
		Inputs:       "{}",
		Status:       StatusQueued,
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	run, err := GetRun(db, id)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusQueued || run.WorkflowFile != ".github/workflows/ci.yml" {
		t.Fatalf("unexpected run: %+v", run)
	}

	started := time.Now().Unix()
	if err := UpdateRunStatus(db, id, StatusRunning, &started, nil); err != nil {
		t.Fatalf("update to running: %v", err)
	}
	run, _ = GetRun(db, id)
	if run.Status != StatusRunning || !run.StartedAt.Valid {
		t.Fatalf("expected running with started_at set, got %+v", run)
	}

	finished := time.Now().Unix()
	if err := UpdateRunStatus(db, id, StatusSuccess, nil, &finished); err != nil {
		t.Fatalf("update to success: %v", err)
	}
	run, _ = GetRun(db, id)
	if run.Status != StatusSuccess || !run.FinishedAt.Valid || !run.StartedAt.Valid {
		t.Fatalf("expected success with both timestamps set, got %+v", run)
	}

	if err := AppendRunLog(db, id, 1, "first line"); err != nil {
		t.Fatalf("append log 1: %v", err)
	}
	if err := AppendRunLog(db, id, 2, "second line"); err != nil {
		t.Fatalf("append log 2: %v", err)
	}
	logs, err := GetRunLogs(db, id)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(logs) != 2 || logs[0] != "first line" || logs[1] != "second line" {
		t.Fatalf("unexpected logs order: %v", logs)
	}

	id2, _ := CreateRun(db, Run{RepoPath: "/repo/a", WorkflowFile: "x.yml", Event: "push", Inputs: "{}", Status: StatusQueued, CreatedAt: now + 1})
	runs, err := ListRuns(db, "/repo/a")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != id2 {
		t.Fatalf("expected newest-first order with id2 first, got %+v", runs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestRuns_CreateGetUpdateList -v`
Expected: FAIL — undefined: `CreateRun`, `Run`, `StatusQueued`, etc.

- [ ] **Step 3: Implement `runs.go`**

Create `runs.go`:
```go
package main

import (
	"database/sql"
)

type RunStatus string

const (
	StatusQueued    RunStatus = "queued"
	StatusRunning   RunStatus = "running"
	StatusSuccess   RunStatus = "success"
	StatusFailed    RunStatus = "failed"
	StatusCancelled RunStatus = "cancelled"
)

type Run struct {
	ID           int64         `json:"id"`
	RepoPath     string        `json:"repoPath"`
	WorkflowFile string        `json:"workflowFile"`
	Event        string        `json:"event"`
	Inputs       string        `json:"inputs"`
	Status       RunStatus     `json:"status"`
	StartedAt    sql.NullInt64 `json:"startedAt"`
	FinishedAt   sql.NullInt64 `json:"finishedAt"`
	CreatedAt    int64         `json:"createdAt"`
}

func CreateRun(db *sql.DB, r Run) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO runs (repo_path, workflow_file, event, inputs, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		r.RepoPath, r.WorkflowFile, r.Event, r.Inputs, string(r.Status), r.CreatedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdateRunStatus(db *sql.DB, id int64, status RunStatus, startedAt, finishedAt *int64) error {
	_, err := db.Exec(
		`UPDATE runs SET status = ?, started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at) WHERE id = ?`,
		string(status), startedAt, finishedAt, id,
	)
	return err
}

func AppendRunLog(db *sql.DB, runID int64, lineNo int, text string) error {
	_, err := db.Exec(
		`INSERT INTO run_logs (run_id, line_no, text) VALUES (?, ?, ?)`,
		runID, lineNo, text,
	)
	return err
}

func GetRunLogs(db *sql.DB, runID int64) ([]string, error) {
	rows, err := db.Query(`SELECT text FROM run_logs WHERE run_id = ? ORDER BY line_no`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		lines = append(lines, text)
	}
	return lines, rows.Err()
}

func GetRun(db *sql.DB, id int64) (Run, error) {
	var r Run
	var status string
	err := db.QueryRow(
		`SELECT id, repo_path, workflow_file, event, inputs, status, started_at, finished_at, created_at FROM runs WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.RepoPath, &r.WorkflowFile, &r.Event, &r.Inputs, &status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt)
	r.Status = RunStatus(status)
	return r, err
}

func ListRuns(db *sql.DB, repoPath string) ([]Run, error) {
	rows, err := db.Query(
		`SELECT id, repo_path, workflow_file, event, inputs, status, started_at, finished_at, created_at FROM runs WHERE repo_path = ? ORDER BY created_at DESC`,
		repoPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		var r Run
		var status string
		if err := rows.Scan(&r.ID, &r.RepoPath, &r.WorkflowFile, &r.Event, &r.Inputs, &status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Status = RunStatus(status)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestRuns_CreateGetUpdateList -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add runs.go runs_test.go
git commit -m "feat: add run history store"
```

---

### Task 6: act argv builder

**Files:**
- Create: `actrunner_argv.go`
- Test: `actrunner_argv_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `RunRequest{RepoPath, WorkflowFile, Event, Inputs, ExtraSecrets, ExtraVars}`, `BuildArgv(req RunRequest, secretFile, varFile string) []string`. Task 7 (act engine) and Task 9 (API) both use `RunRequest`; Task 7 calls `BuildArgv`.

- [ ] **Step 1: Write the failing tests**

Create `actrunner_argv_test.go`:
```go
package main

import (
	"reflect"
	"testing"
)

func TestBuildArgv_NoInputs(t *testing.T) {
	req := RunRequest{WorkflowFile: ".github/workflows/ci.yml", Event: "push"}
	got := BuildArgv(req, "/tmp/secrets.env", "/tmp/vars.env")
	want := []string{"push", "-W", ".github/workflows/ci.yml", "--secret-file", "/tmp/secrets.env", "--var-file", "/tmp/vars.env"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildArgv_InputsAreSortedForDeterminism(t *testing.T) {
	req := RunRequest{
		WorkflowFile: "deploy.yml",
		Event:        "workflow_dispatch",
		Inputs:       map[string]string{"zeta": "1", "alpha": "hello world"},
	}
	got := BuildArgv(req, "/tmp/s.env", "/tmp/v.env")
	want := []string{
		"workflow_dispatch", "-W", "deploy.yml", "--secret-file", "/tmp/s.env", "--var-file", "/tmp/v.env",
		"--input", "alpha=hello world",
		"--input", "zeta=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestBuildArgv -v`
Expected: FAIL — undefined: `RunRequest`, `BuildArgv`.

- [ ] **Step 3: Implement `actrunner_argv.go`**

Create `actrunner_argv.go`:
```go
package main

import (
	"fmt"
	"sort"
)

type RunRequest struct {
	RepoPath     string            `json:"repoPath"`
	WorkflowFile string            `json:"workflowFile"`
	Event        string            `json:"event"`
	Inputs       map[string]string `json:"inputs"`
	ExtraSecrets map[string]string `json:"extraSecrets"`
	ExtraVars    map[string]string `json:"extraVars"`
}

func BuildArgv(req RunRequest, secretFile, varFile string) []string {
	argv := []string{req.Event, "-W", req.WorkflowFile, "--secret-file", secretFile, "--var-file", varFile}

	keys := make([]string, 0, len(req.Inputs))
	for k := range req.Inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		argv = append(argv, "--input", fmt.Sprintf("%s=%s", k, req.Inputs[k]))
	}
	return argv
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestBuildArgv -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add actrunner_argv.go actrunner_argv_test.go
git commit -m "feat: add act argv builder"
```

---

### Task 7: act invocation engine + FIFO queue

**Files:**
- Create: `actrunner.go`
- Test: `actrunner_test.go`

**Interfaces:**
- Consumes: `ListSecrets`, `GetSecretValue` (Task 3), `CreateRun`, `UpdateRunStatus`, `AppendRunLog`, `RunStatus` values (Task 5), `RunRequest`, `BuildArgv` (Task 6).
- Produces: `LineHandler func(runID int64, line string)`, `NewEngine(db *sql.DB, key []byte, actBin string, onLine LineHandler) *Engine`, `(*Engine) Enqueue(req RunRequest) (int64, error)`, `(*Engine) Cancel(runID int64) bool`. Task 9 (API/main.go) constructs the `Engine` and calls `Enqueue`/`Cancel`; the `onLine` callback Task 9 passes in is `hub.Broadcast` from Task 8.

- [ ] **Step 1: Write the failing tests**

Create `actrunner_test.go`:
```go
package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeStub(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestEngine_SuccessfulRun(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-success.sh", "#!/bin/sh\necho \"ran with: $@\"\necho \"line two\" 1>&2\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var mu sync.Mutex
	var lines []string
	engine := NewEngine(db, make([]byte, keySize), stub, func(runID int64, line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, line)
	})

	runID, err := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForStatus(t, db, runID, StatusSuccess)

	mu.Lock()
	defer mu.Unlock()
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 streamed lines, got %v", lines)
	}
}

func TestEngine_FailingRun(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-fail.sh", "#!/bin/sh\necho \"about to fail\"\nexit 1\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil)
	runID, err := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForStatus(t, db, runID, StatusFailed)
}

func TestEngine_RunsQueueSequentially(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-slow.sh", "#!/bin/sh\nsleep 0.3\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil)

	id1, _ := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	id2, _ := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})

	waitForStatus(t, db, id1, StatusSuccess)
	waitForStatus(t, db, id2, StatusSuccess)

	run1, _ := GetRun(db, id1)
	run2, _ := GetRun(db, id2)
	if run2.StartedAt.Int64 < run1.FinishedAt.Int64 {
		t.Fatalf("expected run2 to start after run1 finished (sequential queue); run1=%+v run2=%+v", run1, run2)
	}
}

func TestEngine_Cancel(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-longsleep.sh", "#!/bin/sh\nsleep 5\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil)
	runID, _ := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})

	waitForStatus(t, db, runID, StatusRunning)

	if !engine.Cancel(runID) {
		t.Fatal("expected Cancel to return true for a running run")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := GetRun(db, runID)
		if run.Status == StatusCancelled || run.Status == StatusFailed {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected run to be cancelled/failed within 2s of Cancel")
}

func waitForStatus(t *testing.T, db *sql.DB, runID int64, want RunStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := GetRun(db, runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if run.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %d did not reach status %q within timeout", runID, want)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestEngine -v`
Expected: FAIL — undefined: `NewEngine`, `Engine`.

- [ ] **Step 3: Implement `actrunner.go`**

Create `actrunner.go`:
```go
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type LineHandler func(runID int64, line string)

type Engine struct {
	db      *sql.DB
	key     []byte
	actBin  string
	onLine  LineHandler
	queue   chan queuedRun
	mu      sync.Mutex
	running map[int64]*exec.Cmd
}

type queuedRun struct {
	runID int64
	req   RunRequest
}

func NewEngine(db *sql.DB, key []byte, actBin string, onLine LineHandler) *Engine {
	e := &Engine{
		db:      db,
		key:     key,
		actBin:  actBin,
		onLine:  onLine,
		queue:   make(chan queuedRun, 100),
		running: map[int64]*exec.Cmd{},
	}
	go e.worker()
	return e
}

func encodeInputs(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (e *Engine) Enqueue(req RunRequest) (int64, error) {
	runID, err := CreateRun(e.db, Run{
		RepoPath:     req.RepoPath,
		WorkflowFile: req.WorkflowFile,
		Event:        req.Event,
		Inputs:       encodeInputs(req.Inputs),
		Status:       StatusQueued,
		CreatedAt:    time.Now().Unix(),
	})
	if err != nil {
		return 0, err
	}
	e.queue <- queuedRun{runID: runID, req: req}
	return runID, nil
}

func (e *Engine) Cancel(runID int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	cmd, ok := e.running[runID]
	if !ok || cmd.Process == nil {
		return false
	}
	return cmd.Process.Kill() == nil
}

func (e *Engine) worker() {
	for qr := range e.queue {
		e.runOne(qr.runID, qr.req)
	}
}

func (e *Engine) runOne(runID int64, req RunRequest) {
	started := time.Now().Unix()
	UpdateRunStatus(e.db, runID, StatusRunning, &started, nil)

	secretFile, varFile, cleanup, err := e.writeTempFiles(req)
	if err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}
	defer cleanup()

	argv := BuildArgv(req, secretFile, varFile)
	cmd := exec.Command(e.actBin, argv...)
	cmd.Dir = req.RepoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}

	if err := cmd.Start(); err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}

	e.mu.Lock()
	e.running[runID] = cmd
	e.mu.Unlock()

	var lineMu sync.Mutex
	lineNo := 0
	var wg sync.WaitGroup
	stream := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lineMu.Lock()
			lineNo++
			n := lineNo
			lineMu.Unlock()
			text := scanner.Text()
			AppendRunLog(e.db, runID, n, text)
			if e.onLine != nil {
				e.onLine(runID, text)
			}
		}
	}
	wg.Add(2)
	go stream(stdout)
	go stream(stderr)
	wg.Wait()

	waitErr := cmd.Wait()

	e.mu.Lock()
	delete(e.running, runID)
	e.mu.Unlock()

	status := StatusSuccess
	if waitErr != nil {
		status = StatusFailed
		if cmd.ProcessState != nil && strings.Contains(cmd.ProcessState.String(), "signal: killed") {
			status = StatusCancelled
		}
	}
	e.finish(runID, status, started)
}

func (e *Engine) finish(runID int64, status RunStatus, started int64) {
	finished := time.Now().Unix()
	UpdateRunStatus(e.db, runID, status, &started, &finished)
}

func (e *Engine) writeTempFiles(req RunRequest) (secretFile, varFile string, cleanup func(), err error) {
	secrets, err := ListSecrets(e.db, req.RepoPath, KindSecret)
	if err != nil {
		return "", "", nil, err
	}
	vars, err := ListSecrets(e.db, req.RepoPath, KindVar)
	if err != nil {
		return "", "", nil, err
	}

	sf, err := e.writeDotenvTemp("act-secrets-*.env", req.RepoPath, secrets, req.ExtraSecrets)
	if err != nil {
		return "", "", nil, err
	}
	vf, err := e.writeDotenvTemp("act-vars-*.env", req.RepoPath, vars, req.ExtraVars)
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

func (e *Engine) writeDotenvTemp(pattern, repoPath string, entries []SecretEntry, extra map[string]string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return "", err
	}
	for _, entry := range entries {
		val, err := GetSecretValue(e.db, e.key, repoPath, entry.Kind, entry.Key)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(f, "%s=%s\n", entry.Key, val)
	}
	for k, v := range extra {
		fmt.Fprintf(f, "%s=%s\n", k, v)
	}
	return f.Name(), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestEngine -v`
Expected: PASS (all 4 subtests). This test spawns real shell scripts but never touches Docker or the real `act` binary.

- [ ] **Step 5: Commit**

```bash
git add actrunner.go actrunner_test.go
git commit -m "feat: add act invocation engine with FIFO queue"
```

---

### Task 8: WebSocket log hub

**Files:**
- Create: `ws.go`
- Test: `ws_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `NewHub() *Hub`, `(*Hub) Broadcast(runID int64, line string)`, `(*Hub) ServeWS(w http.ResponseWriter, r *http.Request, runID int64) error`. Task 9 (main.go/API) constructs the `Hub`, passes `hub.Broadcast` as the `Engine`'s `LineHandler`, and calls `hub.ServeWS` from the `/ws/runs/{id}` handler.

- [ ] **Step 1: Write the failing test**

Create `ws_test.go`:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHub_ReplayThenLiveBroadcast(t *testing.T) {
	hub := NewHub()

	hub.Broadcast(42, "buffered line 1")
	hub.Broadcast(42, "buffered line 2")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, 42)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read buffered 1: %v", err)
	}
	if string(msg1) != "buffered line 1" {
		t.Fatalf("expected buffered line 1, got %q", msg1)
	}

	_, msg2, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read buffered 2: %v", err)
	}
	if string(msg2) != "buffered line 2" {
		t.Fatalf("expected buffered line 2, got %q", msg2)
	}

	time.Sleep(50 * time.Millisecond) // let the client register before broadcasting live
	hub.Broadcast(42, "live line")

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg3, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read live: %v", err)
	}
	if string(msg3) != "live line" {
		t.Fatalf("expected live line, got %q", msg3)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestHub_ReplayThenLiveBroadcast -v`
Expected: FAIL — undefined: `NewHub`.

- [ ] **Step 3: Implement `ws.go`**

Create `ws.go`:
```go
package main

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // local-only tool, no cross-origin threat model
}

type wsClient struct {
	send chan string
}

type Hub struct {
	mu      sync.Mutex
	buffers map[int64][]string
	clients map[int64]map[*wsClient]bool
}

func NewHub() *Hub {
	return &Hub{
		buffers: map[int64][]string{},
		clients: map[int64]map[*wsClient]bool{},
	}
}

func (h *Hub) Broadcast(runID int64, line string) {
	h.mu.Lock()
	h.buffers[runID] = append(h.buffers[runID], line)
	var recipients []*wsClient
	for c := range h.clients[runID] {
		recipients = append(recipients, c)
	}
	h.mu.Unlock()

	for _, c := range recipients {
		select {
		case c.send <- line:
		default:
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, runID int64) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := &wsClient{send: make(chan string, 256)}

	h.mu.Lock()
	buffered := append([]string(nil), h.buffers[runID]...)
	if h.clients[runID] == nil {
		h.clients[runID] = map[*wsClient]bool{}
	}
	h.clients[runID][client] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients[runID], client)
		h.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for _, line := range buffered {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return err
		}
	}

	for {
		select {
		case line := <-client.send:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				return err
			}
		case <-done:
			return nil
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestHub_ReplayThenLiveBroadcast -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add ws.go ws_test.go
git commit -m "feat: add WebSocket log streaming hub"
```

---

### Task 9: HTTP API + main entrypoint

**Files:**
- Create: `api.go`
- Create: `main.go`
- Test: `api_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–8 (`OpenDB`, `LoadOrCreateKey`/`DefaultKeyPath`, secrets functions, `ScanWorkflows`, runs functions, `RunRequest`/`BuildArgv`, `Engine`/`NewEngine`, `Hub`/`NewHub`).
- Produces: `NewRouter(db *sql.DB, key []byte, engine *Engine, hub *Hub, actBin string) *http.ServeMux`, `main()`. Nothing later depends on these — this is the wiring layer.

- [ ] **Step 1: Write the failing tests**

Create `api_test.go`:
```go
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestRouter(t *testing.T, actStub string) (*http.ServeMux, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	key := make([]byte, keySize)
	hub := NewHub()
	engine := NewEngine(db, key, actStub, hub.Broadcast)
	return NewRouter(db, key, engine, hub, actStub), db, dir
}

func TestAPI_ScanSecretsAndRunLifecycle(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	wfContent := "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".github", "workflows", "ci.yml"), []byte(wfContent), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\necho \"stub output\"\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	// scan
	scanBody, _ := json.Marshal(map[string]string{"path": repoDir})
	resp, err := http.Post(server.URL+"/api/scan", "application/json", bytes.NewReader(scanBody))
	if err != nil {
		t.Fatalf("scan request: %v", err)
	}
	var workflows []WorkflowInfo
	json.NewDecoder(resp.Body).Decode(&workflows)
	resp.Body.Close()
	if len(workflows) != 1 || workflows[0].Events[0] != "push" {
		t.Fatalf("unexpected scan result: %+v", workflows)
	}

	// secrets: upsert, list, delete
	upsertBody, _ := json.Marshal(map[string]string{"repoPath": repoDir, "kind": "secret", "key": "TOKEN", "value": "abc"})
	resp, err = http.Post(server.URL+"/api/secrets", "application/json", bytes.NewReader(upsertBody))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("upsert secret: err=%v status=%v", err, resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/api/secrets?repoPath=" + repoDir + "&kind=secret")
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	var entries []SecretEntry
	json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) != 1 || entries[0].Key != "TOKEN" {
		t.Fatalf("unexpected secrets list: %+v", entries)
	}

	// run: create, poll until done, check logs
	runBody, _ := json.Marshal(map[string]any{
		"repoPath":     repoDir,
		"workflowFile": ".github/workflows/ci.yml",
		"event":        "push",
	})
	resp, err = http.Post(server.URL+"/api/runs", "application/json", bytes.NewReader(runBody))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	var created struct {
		RunID int64 `json:"runId"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	var detail map[string]any
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(server.URL + "/api/runs/" + itoa(created.RunID))
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		json.NewDecoder(resp.Body).Decode(&detail)
		resp.Body.Close()
		run := detail["run"].(map[string]any)
		if run["status"] == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	logs := detail["logs"].([]any)
	if len(logs) == 0 || !strings.Contains(logs[0].(string), "stub output") {
		t.Fatalf("expected stub output in logs, got %v", logs)
	}
}

func itoa(n int64) string {
	return string(rune('0' + n%10)) // fine for single-digit run IDs in this small test DB
}
```

Note: `itoa` is a deliberately tiny helper — this test DB never accumulates more than single-digit run IDs, so a full `strconv.FormatInt` isn't needed here (use `strconv.FormatInt` in real handlers, only this test helper is simplified).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestAPI_ScanSecretsAndRunLifecycle -v`
Expected: FAIL — undefined: `NewRouter`.

- [ ] **Step 3: Implement `api.go`**

Create `api.go`:
```go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func NewRouter(db *sql.DB, key []byte, engine *Engine, hub *Hub, actBin string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		actOut, actErr := exec.CommandContext(ctx, actBin, "--version").CombinedOutput()
		dockerErr := exec.CommandContext(ctx, "docker", "info").Run()

		writeJSON(w, http.StatusOK, map[string]any{
			"actOK":      actErr == nil,
			"actVersion": string(actOut),
			"dockerOK":   dockerErr == nil,
		})
	})

	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		workflows, err := ScanWorkflows(body.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, workflows)
	})

	mux.HandleFunc("GET /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		repoPath := r.URL.Query().Get("repoPath")
		kind := SecretKind(r.URL.Query().Get("kind"))
		entries, err := ListSecrets(db, repoPath, kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})

	mux.HandleFunc("POST /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath string     `json:"repoPath"`
			Kind     SecretKind `json:"kind"`
			Key      string     `json:"key"`
			Value    string     `json:"value"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := UpsertSecret(db, key, body.RepoPath, body.Kind, body.Key, body.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath string     `json:"repoPath"`
			Kind     SecretKind `json:"kind"`
			Key      string     `json:"key"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := DeleteSecret(db, body.RepoPath, body.Kind, body.Key); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath     string            `json:"repoPath"`
			WorkflowFile string            `json:"workflowFile"`
			Event        string            `json:"event"`
			Inputs       map[string]string `json:"inputs"`
			ExtraSecrets map[string]string `json:"extraSecrets"`
			ExtraVars    map[string]string `json:"extraVars"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runID, err := engine.Enqueue(RunRequest{
			RepoPath:     body.RepoPath,
			WorkflowFile: body.WorkflowFile,
			Event:        body.Event,
			Inputs:       body.Inputs,
			ExtraSecrets: body.ExtraSecrets,
			ExtraVars:    body.ExtraVars,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"runId": runID})
	})

	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		repoPath := r.URL.Query().Get("repoPath")
		runs, err := ListRuns(db, repoPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	})

	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		run, err := GetRun(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		logs, err := GetRunLogs(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": run, "logs": logs})
	})

	mux.HandleFunc("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		if !engine.Cancel(id) {
			http.Error(w, "run not active", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /ws/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		hub.ServeWS(w, r, id)
	})

	return mux
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestAPI_ScanSecretsAndRunLifecycle -v`
Expected: PASS

- [ ] **Step 5: Write `main.go`** (no test — this is the wiring entrypoint, exercised by the manual end-to-end test in Task 13)

Create `main.go`:
```go
package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	dbPath := flag.String("db", "local-action.db", "path to sqlite database file")
	actBin := flag.String("act-bin", "act", "path to act executable")
	flag.Parse()

	keyPath, err := DefaultKeyPath()
	if err != nil {
		log.Fatalf("resolve key path: %v", err)
	}
	key, err := LoadOrCreateKey(keyPath)
	if err != nil {
		log.Fatalf("load encryption key: %v", err)
	}

	db, err := OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	hub := NewHub()
	engine := NewEngine(db, key, *actBin, hub.Broadcast)

	mux := NewRouter(db, key, engine, hub, *actBin)

	staticFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("load embedded UI: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	log.Printf("local-action listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
```

This references `webDist`, which Task 13 defines in `embed.go`. `go build` for this task alone will fail until Task 13 adds `embed.go` — that's expected; `go test ./...` (which is what this task verifies) still passes because tests don't invoke `main()`.

- [ ] **Step 6: Commit**

```bash
git add api.go api_test.go main.go
git commit -m "feat: add HTTP API and main entrypoint"
```

---

### Task 10: React scaffold + workflow list + run trigger

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.js`
- Create: `web/index.html`
- Create: `web/src/main.jsx`
- Create: `web/src/api.js`
- Create: `web/src/App.jsx`
- Create: `web/src/components/WorkflowsPanel.jsx`
- Create: `web/src/style.css`
- Create: `.gitignore`

**Interfaces:**
- Consumes: backend REST API from Task 9 (`/api/scan`, `/api/runs`).
- Produces: `App` component with a `tab` state machine (`workflows`/`secrets`/`history`) and `repoPath` state (persisted to `localStorage`) passed down as props. Task 11's `SecretsPanel` and Task 12's `HistoryPanel` are rendered by `App` and receive `repoPath` as a prop; Task 12 also receives `activeRunId` (set by `WorkflowsPanel`'s `onRunStarted(runId)` callback, defined in this task).

- [ ] **Step 1: Add `.gitignore`**

Create `.gitignore`:
```
web/node_modules/
local-action.db
local-action
```

- [ ] **Step 2: Scaffold the Vite project**

Create `web/package.json`:
```json
{
  "name": "local-action-web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@vitejs/plugin-react": "^4.3.1",
    "vite": "^5.4.0"
  }
}
```

Create `web/vite.config.js`:
```js
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8090',
      '/ws': { target: 'ws://localhost:8090', ws: true },
    },
  },
})
```

Create `web/index.html`:
```html
<!doctype html>
<html>
  <head>
    <meta charset="UTF-8" />
    <title>local-action</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
```

Run:
```bash
cd web && npm install
```
Expected: `node_modules/` and `package-lock.json` created, no errors.

- [ ] **Step 3: Write the API client**

Create `web/src/api.js`:
```js
async function request(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new Error(await res.text())
  }
  if (res.status === 204) return null
  return res.json()
}

export const api = {
  health: () => request('GET', '/api/health'),
  scan: (path) => request('POST', '/api/scan', { path }),
  listSecrets: (repoPath, kind) =>
    request('GET', `/api/secrets?repoPath=${encodeURIComponent(repoPath)}&kind=${kind}`),
  upsertSecret: (repoPath, kind, key, value) =>
    request('POST', '/api/secrets', { repoPath, kind, key, value }),
  deleteSecret: (repoPath, kind, key) => request('DELETE', '/api/secrets', { repoPath, kind, key }),
  createRun: (payload) => request('POST', '/api/runs', payload),
  listRuns: (repoPath) => request('GET', `/api/runs?repoPath=${encodeURIComponent(repoPath)}`),
  getRun: (id) => request('GET', `/api/runs/${id}`),
  cancelRun: (id) => request('POST', `/api/runs/${id}/cancel`),
}
```

- [ ] **Step 4: Write the workflows panel**

Create `web/src/components/WorkflowsPanel.jsx`:
```jsx
import { useState } from 'react'
import { api } from '../api.js'

export default function WorkflowsPanel({ repoPath, setRepoPath, onRunStarted }) {
  const [workflows, setWorkflows] = useState([])
  const [error, setError] = useState(null)
  const [selections, setSelections] = useState({})

  async function scan() {
    setError(null)
    try {
      const result = await api.scan(repoPath)
      setWorkflows(result || [])
    } catch (err) {
      setError(err.message)
    }
  }

  function selectEvent(file, event) {
    setSelections((prev) => ({ ...prev, [file]: { event, inputs: {} } }))
  }

  function setInput(file, name, value) {
    setSelections((prev) => ({
      ...prev,
      [file]: { ...prev[file], inputs: { ...prev[file].inputs, [name]: value } },
    }))
  }

  async function run(workflow) {
    const selection = selections[workflow.file]
    if (!selection) return
    const { runId } = await api.createRun({
      repoPath,
      workflowFile: workflow.file,
      event: selection.event,
      inputs: selection.inputs,
    })
    onRunStarted(runId)
  }

  return (
    <div>
      <div className="row">
        <input
          value={repoPath}
          onChange={(e) => setRepoPath(e.target.value)}
          placeholder="/path/to/repo"
        />
        <button onClick={scan}>Scan</button>
      </div>
      {error && <p className="error">{error}</p>}
      {workflows.map((wf) => {
        const selection = selections[wf.file] || { event: '', inputs: {} }
        const dispatchInputs = wf.dispatchInputs || []
        return (
          <div className="card" key={wf.file}>
            <h3>{wf.name}</h3>
            <p>{wf.file}</p>
            {wf.parseError ? (
              <p className="error">{wf.parseError}</p>
            ) : (
              <>
                <select value={selection.event} onChange={(e) => selectEvent(wf.file, e.target.value)}>
                  <option value="">Select event</option>
                  {wf.events.map((ev) => (
                    <option key={ev} value={ev}>
                      {ev}
                    </option>
                  ))}
                </select>
                {selection.event === 'workflow_dispatch' &&
                  dispatchInputs.map((input) => (
                    <div key={input.name}>
                      <label>{input.name}</label>
                      <input
                        placeholder={input.default}
                        onChange={(e) => setInput(wf.file, input.name, e.target.value)}
                      />
                    </div>
                  ))}
                <button disabled={!selection.event} onClick={() => run(wf)}>
                  Run
                </button>
              </>
            )}
          </div>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 5: Write `App.jsx` and `main.jsx`**

Create `web/src/App.jsx`:
```jsx
import { useState } from 'react'
import WorkflowsPanel from './components/WorkflowsPanel.jsx'

export default function App() {
  const [repoPath, setRepoPath] = useState(localStorage.getItem('repoPath') || '')
  const [tab, setTab] = useState('workflows')
  const [activeRunId, setActiveRunId] = useState(null)

  function updateRepoPath(path) {
    setRepoPath(path)
    localStorage.setItem('repoPath', path)
  }

  function onRunStarted(runId) {
    setActiveRunId(runId)
    setTab('history')
  }

  return (
    <div className="app">
      <nav>
        <button onClick={() => setTab('workflows')}>Workflows</button>
        <button onClick={() => setTab('secrets')}>Secrets</button>
        <button onClick={() => setTab('history')}>History</button>
      </nav>
      {tab === 'workflows' && (
        <WorkflowsPanel repoPath={repoPath} setRepoPath={updateRepoPath} onRunStarted={onRunStarted} />
      )}
      {tab === 'secrets' && <p>Secrets panel placeholder — added in Task 11.</p>}
      {tab === 'history' && <p>History panel placeholder — added in Task 12.</p>}
    </div>
  )
}
```

Create `web/src/main.jsx`:
```jsx
import { createRoot } from 'react-dom/client'
import App from './App.jsx'
import './style.css'

createRoot(document.getElementById('root')).render(<App />)
```

Create `web/src/style.css`:
```css
body {
  font-family: system-ui, sans-serif;
  margin: 0;
  padding: 1rem;
}
.app nav {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}
.row {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  margin-bottom: 0.5rem;
}
.card {
  border: 1px solid #ccc;
  border-radius: 4px;
  padding: 0.75rem;
  margin-bottom: 0.75rem;
}
.error {
  color: #c0392b;
}
```

- [ ] **Step 6: Manually verify in the browser**

Run:
```bash
cd web && npm run dev
```
Expected: Vite dev server starts (e.g. `http://localhost:5173`). Open it — the "Workflows" tab shows a path input and Scan button, "Secrets"/"History" tabs show placeholders. (Scan will fail since the Go backend isn't running yet in this task — that's expected; full end-to-end check happens in Task 13.) Stop the dev server after confirming the page renders (Ctrl+C).

- [ ] **Step 7: Commit**

```bash
git add .gitignore web/package.json web/package-lock.json web/vite.config.js web/index.html web/src
git commit -m "feat: scaffold React frontend with workflow list and run trigger"
```

---

### Task 11: Secrets manager UI

**Files:**
- Create: `web/src/components/SecretsPanel.jsx`
- Modify: `web/src/App.jsx`

**Interfaces:**
- Consumes: `api.listSecrets`, `api.upsertSecret`, `api.deleteSecret` (Task 10's `api.js`); `repoPath` prop from `App`.
- Produces: `SecretsPanel` component, rendered by `App` in place of the Task 10 placeholder.

- [ ] **Step 1: Write the secrets panel**

Create `web/src/components/SecretsPanel.jsx`:
```jsx
import { useEffect, useState } from 'react'
import { api } from '../api.js'

export default function SecretsPanel({ repoPath }) {
  const [kind, setKind] = useState('secret')
  const [entries, setEntries] = useState([])
  const [name, setName] = useState('')
  const [value, setValue] = useState('')

  async function load() {
    if (!repoPath) return
    setEntries(await api.listSecrets(repoPath, kind))
  }

  useEffect(() => {
    load()
  }, [repoPath, kind])

  async function save() {
    await api.upsertSecret(repoPath, kind, name, value)
    setName('')
    setValue('')
    load()
  }

  async function remove(key) {
    await api.deleteSecret(repoPath, kind, key)
    load()
  }

  return (
    <div>
      <div className="row">
        <label>
          <input type="radio" checked={kind === 'secret'} onChange={() => setKind('secret')} /> Secrets
        </label>
        <label>
          <input type="radio" checked={kind === 'var'} onChange={() => setKind('var')} /> Vars
        </label>
      </div>
      <ul>
        {entries.map((entry) => (
          <li key={entry.key}>
            {entry.key} <button onClick={() => remove(entry.key)}>Delete</button>
          </li>
        ))}
      </ul>
      <div className="row">
        <input placeholder="KEY" value={name} onChange={(e) => setName(e.target.value)} />
        <input placeholder="value" value={value} onChange={(e) => setValue(e.target.value)} />
        <button onClick={save} disabled={!name}>
          Save
        </button>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Wire it into `App.jsx`**

Edit `web/src/App.jsx` — add the import:
```jsx
import SecretsPanel from './components/SecretsPanel.jsx'
```
and replace the secrets placeholder line:
```jsx
{tab === 'secrets' && <p>Secrets panel placeholder — added in Task 11.</p>}
```
with:
```jsx
{tab === 'secrets' && <SecretsPanel repoPath={repoPath} />}
```

- [ ] **Step 3: Manually verify in the browser**

Run:
```bash
cd web && npm run dev
```
Expected: "Secrets" tab renders the Secret/Var radio toggle, an empty list, and the add-entry form (calls will 404/fail until the backend is running — full check in Task 13). Stop the dev server (Ctrl+C).

- [ ] **Step 4: Commit**

```bash
git add web/src/components/SecretsPanel.jsx web/src/App.jsx
git commit -m "feat: add secrets manager UI"
```

---

### Task 12: Run history + live log viewer UI

**Files:**
- Create: `web/src/components/HistoryPanel.jsx`
- Create: `web/src/components/LogViewer.jsx`
- Modify: `web/src/App.jsx`

**Interfaces:**
- Consumes: `api.listRuns`, `api.getRun`, `api.cancelRun` (Task 10's `api.js`); backend `/ws/runs/{id}` WebSocket endpoint (Task 9); `repoPath` and `activeRunId` props from `App`.
- Produces: `HistoryPanel` and `LogViewer` components, rendered by `App` in place of the Task 10 placeholder.

- [ ] **Step 1: Write the log viewer**

Create `web/src/components/LogViewer.jsx`:
```jsx
import { useEffect, useRef, useState } from 'react'

export default function LogViewer({ runId, onCancel }) {
  const [lines, setLines] = useState([])
  const socketRef = useRef(null)

  useEffect(() => {
    setLines([])
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws/runs/${runId}`)
    socket.onmessage = (event) => {
      setLines((prev) => [...prev, event.data])
    }
    socketRef.current = socket
    return () => socket.close()
  }, [runId])

  return (
    <div className="log-viewer">
      <div className="row">
        <h3>Run #{runId}</h3>
        <button onClick={onCancel}>Cancel</button>
      </div>
      <pre>{lines.join('\n')}</pre>
    </div>
  )
}
```

- [ ] **Step 2: Write the history panel**

Create `web/src/components/HistoryPanel.jsx`:
```jsx
import { useEffect, useState } from 'react'
import { api } from '../api.js'
import LogViewer from './LogViewer.jsx'

export default function HistoryPanel({ repoPath, activeRunId }) {
  const [runs, setRuns] = useState([])
  const [selectedId, setSelectedId] = useState(activeRunId)

  async function load() {
    if (!repoPath) return
    setRuns(await api.listRuns(repoPath))
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 2000)
    return () => clearInterval(interval)
  }, [repoPath])

  useEffect(() => {
    if (activeRunId) setSelectedId(activeRunId)
  }, [activeRunId])

  return (
    <div className="row">
      <ul className="run-list">
        {runs.map((run) => (
          <li key={run.id} onClick={() => setSelectedId(run.id)}>
            #{run.id} {run.workflowFile} [{run.event}] — {run.status}
          </li>
        ))}
      </ul>
      {selectedId && <LogViewer runId={selectedId} onCancel={() => api.cancelRun(selectedId)} />}
    </div>
  )
}
```

- [ ] **Step 3: Wire it into `App.jsx`**

Edit `web/src/App.jsx` — add the import:
```jsx
import HistoryPanel from './components/HistoryPanel.jsx'
```
and replace the history placeholder line:
```jsx
{tab === 'history' && <p>History panel placeholder — added in Task 12.</p>}
```
with:
```jsx
{tab === 'history' && <HistoryPanel repoPath={repoPath} activeRunId={activeRunId} />}
```

- [ ] **Step 4: Manually verify in the browser**

Run:
```bash
cd web && npm run dev
```
Expected: "History" tab renders an empty run list (backend not running yet, so no runs to show — full check in Task 13). Stop the dev server (Ctrl+C).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/HistoryPanel.jsx web/src/components/LogViewer.jsx web/src/App.jsx
git commit -m "feat: add run history and live log viewer UI"
```

---

### Task 13: go:embed wiring + end-to-end verification

**Files:**
- Create: `embed.go`
- Create: `web/dist/index.html` (placeholder, overwritten by the real build)
- Create: `testdata/sample-repo/.github/workflows/hello.yml`

**Interfaces:**
- Consumes: `web/dist/*` (built by `npm run build` in Task 10's toolchain), referenced by `main.go` (Task 9) via the package-level `webDist` variable this task defines.
- Produces: `webDist embed.FS` — the last piece `main.go` needs to compile.

- [ ] **Step 1: Add the embed placeholder**

Create `web/dist/index.html` (so `go build` has something to embed even before the first real frontend build):
```html
<!doctype html>
<html><body>Run `npm run build` in web/ to produce the real UI.</body></html>
```

- [ ] **Step 2: Write `embed.go`**

Create `embed.go`:
```go
package main

import "embed"

//go:embed all:web/dist
var webDist embed.FS
```

- [ ] **Step 3: Verify the whole test suite passes and the binary builds**

Run:
```bash
go build -o local-action .
go test ./... -v
```
Expected: build succeeds, all tests from Tasks 1–9 pass.

- [ ] **Step 4: Build the real frontend**

Run:
```bash
cd web && npm run build && cd ..
```
Expected: `web/dist/` now contains the real built `index.html` + JS/CSS assets (overwriting the placeholder).

Run:
```bash
go build -o local-action .
```
Expected: rebuilds with the real UI embedded.

- [ ] **Step 5: Add a sample workflow for manual end-to-end testing**

Create `testdata/sample-repo/.github/workflows/hello.yml`:
```yaml
name: Hello
on:
  workflow_dispatch:
    inputs:
      name:
        description: "Who to greet"
        default: "world"
        required: false
jobs:
  greet:
    runs-on: ubuntu-latest
    steps:
      - run: echo "Hello, ${{ inputs.name }}!"
```

- [ ] **Step 6: Manual end-to-end test with real act + Docker**

Prerequisites: `act` and Docker must be installed and working on this machine (`act --version` and `docker info` both succeed).

Run:
```bash
./local-action
```
Expected: logs `local-action listening on :8090`.

In a browser, open `http://localhost:8090`:
1. On the "Workflows" tab, enter the absolute path to `testdata/sample-repo` and click Scan. Expected: "Hello" workflow appears with a `workflow_dispatch` event option.
2. Select `workflow_dispatch`, expected: a "name" input field appears (default placeholder "world").
3. Click Run. Expected: view switches to "History" tab, showing the new run.
4. Expected: live log lines stream in via the log viewer, ending with a line containing `Hello, world!` (or the name you typed), and the run's status becomes `success`.
5. Go to "Secrets" tab, add a test secret/var for this repo path, delete it. Expected: no errors, list updates correctly both times.

Stop the server (Ctrl+C) once verified.

- [ ] **Step 7: Commit**

```bash
git add embed.go web/dist testdata
git commit -m "feat: wire embedded UI and add sample workflow for e2e testing"
```

## Spec Coverage Check

- Repo scanner, local path only: Task 4.
- Secrets/vars manager, encrypted at rest: Tasks 2, 3, 11.
- Run form, event dropdown, workflow_dispatch inputs: Task 10.
- Run queue, single worker, FIFO: Task 7.
- act invoker via `os/exec`: Tasks 6, 7.
- WebSocket log streamer with buffered replay: Task 8, wired into UI in Task 12.
- History store (runs + run_logs), list/detail UI: Tasks 5, 12.
- Error handling: act/Docker health check (Task 9's `/api/health`), invalid YAML surfaced inline (Task 4 + Task 10's UI), failed run status + logs (Task 7 + Task 12), queued-run snapshot isolation (Task 7's `RunRequest` snapshot at `Enqueue` time).
- Single static binary, native host deployment: Task 13.
- No Postgres, no multi-user auth, no git clone, no per-job selection, no concurrent runs, no Dockerized app distribution: none of these appear anywhere in the plan (by omission, as scoped).
