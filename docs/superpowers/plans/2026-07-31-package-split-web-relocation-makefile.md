# Package Split, web/ Relocation, Makefile Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the flat `internal/app` package into six domain packages, move the frontend from `cmd/local-action/web/` to a top-level `web/`, and clean up the Makefile (unified dev command, `lint`/`fmt`/`install`/`db-reset` targets, frontend tests actually running under `make test`).

**Architecture:** This is a structural move with **no behavior change** — every task's "test" step is "existing tests still pass," not new test-driven behavior. Tasks are ordered so `go build ./... && go test ./...` stays green after every single task: symbols are extracted from `internal/app` one leaf-dependency at a time (packages with no internal dependents first), and every current caller of a moving symbol — in `main.go`, in `api.go`, and in any `_test.go` file still resident in `internal/app` — gets its call site fixed in the same task that moves the symbol away.

**Tech Stack:** Go 1.25 stdlib (`net/http`, `database/sql`, `embed`), `modernc.org/sqlite`, `gopkg.in/yaml.v3`, `github.com/gorilla/websocket`. Frontend: Vite + React, Node's built-in `node --test`.

## Global Constraints

- Module path is `local-action` (from `go.mod`) — internal imports are `local-action/internal/<pkg>`.
- No exported function/type is renamed. `runs.CreateRun`, `secrets.UpsertSecret`, etc. keep their existing names (see spec's "Out of scope" — dropping package-name stutter is a separate, unrequested cleanup).
- Every task ends with `go build ./...` and `go test ./...` (or the relevant subset while `internal/app` still exists) passing, then a commit. Never leave the tree in a non-building state between commits.
- `_test.go` files move together with the production file(s) they test, in the same task.
- A subtlety used repeatedly below: `foo, err := db.OpenDB(path)` compiles even though the new local variable `foo`/`db` (as applicable) has the same name as the package `db` — Go's scoping rule is that a variable's scope begins *after* its declaring statement, so the package name is still resolved correctly on the line that shadows it. This only becomes a problem if the **same function** needs to reference the package again *after* the shadowing line — checked for every call site below; none do.

---

### Task 1: Extract `internal/db`

**Files:**
- Create: `internal/db/db.go` (moved from `internal/app/db.go`)
- Create: `internal/db/db_test.go` (moved from `internal/app/db_test.go`)
- Modify: `cmd/local-action/main.go`
- Modify: `internal/app/api_test.go`
- Modify: `internal/app/actrunner_test.go`
- Modify: `internal/app/secrets_test.go`
- Modify: `internal/app/runs_test.go`
- Modify: `internal/app/workflowcategory_test.go`
- Modify: `internal/app/eventpayload_test.go`

**Interfaces:**
- Produces: `db.OpenDB(path string) (*sql.DB, error)` — every other task's files that open a database call this.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/db
git mv internal/app/db.go internal/db/db.go
git mv internal/app/db_test.go internal/db/db_test.go
```

- [ ] **Step 2: Change the package line in both moved files**

In `internal/db/db.go` and `internal/db/db_test.go`, change:
```go
package app
```
to:
```go
package db
```
No other edits needed in these two files — `OpenDB` calls itself and its own migration helpers unqualified, same package as before.

- [ ] **Step 3: Fix `cmd/local-action/main.go`**

Add `"local-action/internal/db"` to the import block, then change:
```go
	db, err := app.OpenDB(*dbPath)
```
to:
```go
	db, err := db.OpenDB(*dbPath)
```

- [ ] **Step 4: Fix `internal/app/api_test.go`**

Add `"local-action/internal/db"` to the import block. In `newTestRouter`, change:
```go
	db, err := OpenDB(filepath.Join(dir, "test.db"))
```
to:
```go
	db, err := db.OpenDB(filepath.Join(dir, "test.db"))
```

- [ ] **Step 5: Fix `internal/app/actrunner_test.go`**

Add `"local-action/internal/db"` to the import block. Replace every occurrence of `OpenDB(filepath.Join(dir, "test.db"))` with `db.OpenDB(filepath.Join(dir, "test.db"))` — there are 7 occurrences, one at the top of each of: `TestEngine_SuccessfulRun`, `TestEngine_NeverAutoLoadsRepoDotenv`, `TestEngine_OnFinishCalledOnceAfterTerminalStatus`, `TestEngine_FailingRun`, `TestEngine_MissingActBinaryLeavesExplanationInLog`, `TestEngine_RunsQueueSequentially`, `TestEngine_Cancel`.

- [ ] **Step 6: Fix `internal/app/secrets_test.go`**

Add `"local-action/internal/db"` to the import block. Replace every occurrence of `OpenDB(filepath.Join(t.TempDir(), "test.db"))` with `db.OpenDB(filepath.Join(t.TempDir(), "test.db"))` — 4 occurrences, one at the top of each of: `TestSecrets_UpsertListGetDelete`, `TestListSecrets_EmptyMarshalsAsEmptyArray`, `TestSecretsForRun_WorkflowOverridesRepoWide`, `TestSecretsForRun_CorruptedValueFails`.

- [ ] **Step 7: Fix `internal/app/runs_test.go`**

Add `"local-action/internal/db"` to the import block. Replace every occurrence of `OpenDB(filepath.Join(t.TempDir(), "test.db"))` with `db.OpenDB(filepath.Join(t.TempDir(), "test.db"))` — 3 occurrences, in `TestRuns_CreateGetUpdateList`, `TestListRuns_EmptyMarshalsAsEmptyArray`, `TestGetRunLogs_EmptyMarshalsAsEmptyArray`.

- [ ] **Step 8: Fix `internal/app/workflowcategory_test.go`**

Add `"local-action/internal/db"` to the import block. In `TestWorkflowCategories_SaveGetAndClear`, change:
```go
	db, err := OpenDB(filepath.Join(t.TempDir(), "test.db"))
```
to:
```go
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
```

- [ ] **Step 9: Fix `internal/app/eventpayload_test.go`**

Add `"local-action/internal/db"` to the import block. Replace both occurrences of `OpenDB(filepath.Join(t.TempDir(), "test.db"))` with `db.OpenDB(filepath.Join(t.TempDir(), "test.db"))` — in `TestEventPayload_SaveGetRoundTrip` and `TestEventPayload_SavingEmptyClearsRow`.

- [ ] **Step 10: Build and test**

```bash
go build ./...
go test ./...
```
Expected: builds clean, all tests pass (same set as before this task — no test was added or removed, only relocated).

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/db from internal/app"
```

---

### Task 2: Extract `internal/ws`

**Files:**
- Create: `internal/ws/ws.go` (moved from `internal/app/ws.go`)
- Create: `internal/ws/ws_test.go` (moved from `internal/app/ws_test.go`)
- Modify: `cmd/local-action/main.go`
- Modify: `internal/app/api.go`
- Modify: `internal/app/api_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `ws.Hub`, `ws.NewHub() *Hub`, `(*Hub).Broadcast(runID int64, line string)`, `(*Hub).Forget(runID int64)`, `(*Hub).ServeWS(w http.ResponseWriter, r *http.Request, runID int64) error` — `internal/httpapi` (Task 6) wires these into routes.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/ws
git mv internal/app/ws.go internal/ws/ws.go
git mv internal/app/ws_test.go internal/ws/ws_test.go
```

- [ ] **Step 2: Change the package line**

In both moved files, change `package app` to `package ws`. No other edits — `ws_test.go` calls `NewHub`, `hub.Broadcast`, `hub.ServeWS`, `hub.Forget`, and reaches into `hub.mu`/`hub.buffers` directly (white-box test), all same-package, unaffected by the move.

- [ ] **Step 3: Fix `cmd/local-action/main.go`**

Add `"local-action/internal/ws"` to the import block, then change:
```go
	hub := app.NewHub()
```
to:
```go
	hub := ws.NewHub()
```

- [ ] **Step 4: Fix `internal/app/api.go`**

Add `"local-action/internal/ws"` to the import block. In the `NewRouter` signature, change:
```go
func NewRouter(db *sql.DB, key []byte, engine *Engine, hub *Hub, actBin string) *http.ServeMux {
```
to:
```go
func NewRouter(db *sql.DB, key []byte, engine *Engine, hub *ws.Hub, actBin string) *http.ServeMux {
```
(The `hub.ServeWS(w, r, id)` call inside the `GET /ws/runs/{id}` handler needs no change — it's a method call on the now-typed `*ws.Hub` parameter.)

- [ ] **Step 5: Fix `internal/app/api_test.go`**

Add `"local-action/internal/ws"` to the import block. In `newTestRouter`, change:
```go
	hub := NewHub()
```
to:
```go
	hub := ws.NewHub()
```

- [ ] **Step 6: Build and test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/ws from internal/app"
```

---

### Task 3: Extract `internal/secrets`

**Files:**
- Create: `internal/secrets/secrets.go` (moved from `internal/app/secrets.go`)
- Create: `internal/secrets/crypto.go` (moved from `internal/app/crypto.go`)
- Create: `internal/secrets/secrets_test.go` (moved from `internal/app/secrets_test.go`)
- Create: `internal/secrets/crypto_test.go` (moved from `internal/app/crypto_test.go`)
- Modify: `cmd/local-action/main.go`
- Modify: `internal/app/api.go`
- Modify: `internal/app/api_test.go`
- Modify: `internal/app/actrunner.go`
- Modify: `internal/app/actrunner_test.go`

**Interfaces:**
- Produces: `secrets.SecretKind`, `secrets.KindSecret`, `secrets.KindVar`, `secrets.SecretEntry`, `secrets.UpsertSecret`, `secrets.ListSecrets`, `secrets.GetSecretValue`, `secrets.DeleteSecret`, `secrets.SecretsForRun`, `secrets.Encrypt`, `secrets.Decrypt`, `secrets.LoadOrCreateKey`, `secrets.DefaultKeyPath`, `secrets.KeySize` (renamed from unexported `keySize` — see Step 2, needed because callers outside this package must now build correctly-sized key byte slices).

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/secrets
git mv internal/app/secrets.go internal/secrets/secrets.go
git mv internal/app/crypto.go internal/secrets/crypto.go
git mv internal/app/secrets_test.go internal/secrets/secrets_test.go
git mv internal/app/crypto_test.go internal/secrets/crypto_test.go
```

- [ ] **Step 2: Change package lines and export `keySize`**

In all four moved files, change `package app` to `package secrets`.

In `internal/secrets/crypto.go`, change:
```go
const keySize = 32 // AES-256
```
to:
```go
const KeySize = 32 // AES-256
```
Then, in the same file, update its two internal uses:
```go
	if decErr != nil || len(key) != keySize {
```
→
```go
	if decErr != nil || len(key) != KeySize {
```
and:
```go
	key := make([]byte, keySize)
```
→
```go
	key := make([]byte, KeySize)
```

`internal/secrets/crypto_test.go` and `internal/secrets/secrets_test.go` both use `keySize` (unqualified, same package) — replace every occurrence of `keySize` with `KeySize` in both files (no package prefix needed, they're in package `secrets` now too). `secrets_test.go`: occurrences inside `TestSecrets_UpsertListGetDelete`, `TestSecretsForRun_WorkflowOverridesRepoWide`, `TestSecretsForRun_CorruptedValueFails` (each has one `make([]byte, keySize)`). `crypto_test.go`: `TestLoadOrCreateKey_PersistsAcrossCalls` (`len(key1) != keySize`) and `TestEncryptDecrypt_Roundtrip` (`make([]byte, keySize)`).

- [ ] **Step 3: Fix `cmd/local-action/main.go`**

Add `"local-action/internal/secrets"` to the import block, then change:
```go
	keyPath, err := app.DefaultKeyPath()
	if err != nil {
		log.Fatalf("resolve key path: %v", err)
	}
	key, err := app.LoadOrCreateKey(keyPath)
```
to:
```go
	keyPath, err := secrets.DefaultKeyPath()
	if err != nil {
		log.Fatalf("resolve key path: %v", err)
	}
	key, err := secrets.LoadOrCreateKey(keyPath)
```

- [ ] **Step 4: Fix `internal/app/api.go`**

Add `"local-action/internal/secrets"` to the import block. Make these changes:

In the `POST /api/secrets` handler's body struct, change:
```go
			Kind         SecretKind `json:"kind"`
```
to:
```go
			Kind         secrets.SecretKind `json:"kind"`
```
(Same edit applies in the `DELETE /api/secrets` handler's body struct — both occurrences.)

Change:
```go
		entries, err := ListSecrets(db, repoPath, kind)
```
to:
```go
		entries, err := secrets.ListSecrets(db, repoPath, kind)
```

Change:
```go
		kind := SecretKind(r.URL.Query().Get("kind"))
```
to:
```go
		kind := secrets.SecretKind(r.URL.Query().Get("kind"))
```

Change:
```go
		if err := UpsertSecret(db, key, body.RepoPath, body.Kind, body.Key, body.Value, body.WorkflowFile); err != nil {
```
to:
```go
		if err := secrets.UpsertSecret(db, key, body.RepoPath, body.Kind, body.Key, body.Value, body.WorkflowFile); err != nil {
```

Change:
```go
		if err := DeleteSecret(db, body.RepoPath, body.Kind, body.Key, body.WorkflowFile); err != nil {
```
to:
```go
		if err := secrets.DeleteSecret(db, body.RepoPath, body.Kind, body.Key, body.WorkflowFile); err != nil {
```

- [ ] **Step 5: Fix `internal/app/api_test.go`**

Add `"local-action/internal/secrets"` to the import block.

Change:
```go
	key := make([]byte, keySize)
```
to:
```go
	key := make([]byte, secrets.KeySize)
```

Change both occurrences of:
```go
	var entries []SecretEntry
```
to:
```go
	var entries []secrets.SecretEntry
```
(in `TestAPI_ScanSecretsAndRunLifecycle` and `TestAPI_WorkflowScopedSecrets`).

- [ ] **Step 6: Fix `internal/app/actrunner.go`**

Add `"local-action/internal/secrets"` to the import block. In `writeTempFiles`, change:
```go
	secrets, err := SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, KindSecret)
	if err != nil {
		return "", "", "", "", nil, err
	}
	vars, err := SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, KindVar)
```
to:
```go
	secretValues, err := secrets.SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, secrets.KindSecret)
	if err != nil {
		return "", "", "", "", nil, err
	}
	vars, err := secrets.SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, secrets.KindVar)
```
(The local variable `secrets` is renamed to `secretValues` because it would otherwise shadow the newly-imported `secrets` package for the rest of the function — the very next statement calls `secrets.SecretsForRun` again for vars, which would break under the old name.) Then, further down in the same function, change:
```go
	sf, err := writeDotenvTemp("act-secrets-*.env", secrets, req.ExtraSecrets)
```
to:
```go
	sf, err := writeDotenvTemp("act-secrets-*.env", secretValues, req.ExtraSecrets)
```

- [ ] **Step 7: Fix `internal/app/actrunner_test.go`**

Add `"local-action/internal/secrets"` to the import block. Replace every occurrence of `make([]byte, keySize)` with `make([]byte, secrets.KeySize)` — 7 occurrences, one in each `TestEngine_*` test function listed in Task 1 Step 5.

- [ ] **Step 8: Build and test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/secrets from internal/app"
```

---

### Task 4: Extract `internal/workflows`

**Files:**
- Create: `internal/workflows/scanner.go` (moved from `internal/app/scanner.go`)
- Create: `internal/workflows/workflowcategory.go` (moved from `internal/app/workflowcategory.go`)
- Create: `internal/workflows/eventpayload.go` (moved from `internal/app/eventpayload.go`)
- Create: `internal/workflows/scanner_test.go`, `internal/workflows/workflowcategory_test.go`, `internal/workflows/eventpayload_test.go` (moved from the matching `internal/app/*_test.go`)
- Modify: `internal/app/api.go`
- Modify: `internal/app/api_test.go`

**Interfaces:**
- Produces: `workflows.WorkflowInfo`, `workflows.DispatchInput`, `workflows.ScanWorkflows(repoPath string) ([]WorkflowInfo, error)`, `workflows.ParseWorkflowFile`, `workflows.GetWorkflowCategories`, `workflows.SaveWorkflowCategory`, `workflows.GetEventPayload`, `workflows.SaveEventPayload`.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/workflows
git mv internal/app/scanner.go internal/workflows/scanner.go
git mv internal/app/workflowcategory.go internal/workflows/workflowcategory.go
git mv internal/app/eventpayload.go internal/workflows/eventpayload.go
git mv internal/app/scanner_test.go internal/workflows/scanner_test.go
git mv internal/app/workflowcategory_test.go internal/workflows/workflowcategory_test.go
git mv internal/app/eventpayload_test.go internal/workflows/eventpayload_test.go
```

- [ ] **Step 2: Change package lines**

In all six moved files, change `package app` to `package workflows`. No other edits needed: `scanner_test.go` calls `ScanWorkflows`/`autoCategoryFor` unqualified (same package); `workflowcategory_test.go` and `eventpayload_test.go` already call `db.OpenDB` (qualified in Task 1) plus `GetWorkflowCategories`/`SaveWorkflowCategory`/`GetEventPayload`/`SaveEventPayload` unqualified (same package) — all still correct after the move.

- [ ] **Step 3: Fix `internal/app/api.go`**

Add `"local-action/internal/workflows"` to the import block.

Change:
```go
		workflows, err := ScanWorkflows(body.Path)
```
to:
```go
		workflowList, err := workflows.ScanWorkflows(body.Path)
```
(Renaming the local variable from `workflows` to `workflowList` avoids shadowing the `workflows` package — this handler only calls `ScanWorkflows` once, so it would technically compile either way, but the rename keeps the identifier unambiguous for anyone editing this handler later.) Update the following line in the same handler from:
```go
		writeJSON(w, http.StatusOK, workflows)
```
to:
```go
		writeJSON(w, http.StatusOK, workflowList)
```

Change:
```go
		categories, err := GetWorkflowCategories(db, repoPath)
```
to:
```go
		categories, err := workflows.GetWorkflowCategories(db, repoPath)
```

Change:
```go
		if err := SaveWorkflowCategory(db, body.RepoPath, body.WorkflowFile, body.Category); err != nil {
```
to:
```go
		if err := workflows.SaveWorkflowCategory(db, body.RepoPath, body.WorkflowFile, body.Category); err != nil {
```

Change:
```go
		payload, err := GetEventPayload(db, repoPath, workflowFile)
```
to:
```go
		payload, err := workflows.GetEventPayload(db, repoPath, workflowFile)
```

Change:
```go
		if err := SaveEventPayload(db, body.RepoPath, body.WorkflowFile, body.Payload); err != nil {
```
to:
```go
		if err := workflows.SaveEventPayload(db, body.RepoPath, body.WorkflowFile, body.Payload); err != nil {
```

- [ ] **Step 4: Fix `internal/app/api_test.go`**

Add `"local-action/internal/workflows"` to the import block. Change:
```go
	var workflows []WorkflowInfo
```
to:
```go
	var workflows []workflows.WorkflowInfo
```
(This is the self-shadowing pattern noted in Global Constraints: the type `workflows.WorkflowInfo` resolves against the package before the local variable `workflows` comes into scope. Nothing later in this test function references the `workflows` package again, only the local slice, so this is safe. Verified by the build step below regardless.)

- [ ] **Step 5: Build and test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/workflows from internal/app"
```

---

### Task 5: Extract `internal/runs`

**Files:**
- Create: `internal/runs/runs.go`, `internal/runs/actrunner.go`, `internal/runs/actrunner_argv.go`, `internal/runs/gitinfo.go` (moved from `internal/app/`)
- Create: `internal/runs/runs_test.go`, `internal/runs/actrunner_test.go`, `internal/runs/actrunner_argv_test.go`, `internal/runs/gitinfo_test.go` (moved from `internal/app/`)
- Modify: `cmd/local-action/main.go`
- Modify: `internal/app/api.go`
- Modify: `internal/app/api_test.go`

**Interfaces:**
- Produces: `runs.RunStatus`, `runs.StatusQueued/Running/Success/Failed/Cancelled`, `runs.Run`, `runs.CreateRun`, `runs.UpdateRunStatus`, `runs.AppendRunLog`, `runs.GetRunLogs`, `runs.GetRun`, `runs.ListRuns`, `runs.Engine`, `runs.NewEngine(db *sql.DB, key []byte, actBin string, onLine LineHandler, onFinish FinishHandler) *Engine`, `(*Engine).Enqueue(req RunRequest) (int64, error)`, `(*Engine).Cancel(runID int64) bool`, `runs.RunRequest`, `runs.BuildArgv`.
- Consumes: `secrets.SecretsForRun`, `secrets.KindSecret`, `secrets.KindVar` (already qualified inside `actrunner.go` since Task 3 — no change needed here, just moves with the file).

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/runs
git mv internal/app/runs.go internal/runs/runs.go
git mv internal/app/actrunner.go internal/runs/actrunner.go
git mv internal/app/actrunner_argv.go internal/runs/actrunner_argv.go
git mv internal/app/gitinfo.go internal/runs/gitinfo.go
git mv internal/app/runs_test.go internal/runs/runs_test.go
git mv internal/app/actrunner_test.go internal/runs/actrunner_test.go
git mv internal/app/actrunner_argv_test.go internal/runs/actrunner_argv_test.go
git mv internal/app/gitinfo_test.go internal/runs/gitinfo_test.go
```

- [ ] **Step 2: Change package lines**

In all eight moved files, change `package app` to `package runs`. No other internal edits: every cross-file reference among these eight files (e.g. `actrunner.go` calling `CreateRun`, `actrunner_test.go` calling `NewEngine`/`RunRequest`/`StatusSuccess`/`GetRun`/`GetRunLogs`) was already same-package before the move and stays same-package after. The `secrets.*` and `db.*` qualified calls already inside `actrunner.go`/`actrunner_test.go` (from Tasks 1 and 3) are untouched by this move.

- [ ] **Step 3: Add duplicate test helpers to `internal/app/api_test.go`**

`api_test.go` currently calls two helpers defined in files that are leaving the package in this task: `waitForStatus` (defined at the bottom of `actrunner_test.go`) and `runGit` (defined in `gitinfo_test.go`). Test-file helpers are never visible across packages (even if exported), so `api_test.go` needs its own copies. Add these two functions to `internal/app/api_test.go` (e.g. at the end of the file), and add `"os/exec"` and `"local-action/internal/runs"` to its import block if not already present from earlier tasks:

```go
func waitForStatus(t *testing.T, db *sql.DB, runID int64, want runs.RunStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := runs.GetRun(db, runID)
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
```

- [ ] **Step 4: Fix the rest of `internal/app/api_test.go`**

Add `"local-action/internal/runs"` to the import block if Step 3 didn't already (it's the same import, just don't duplicate it).

In `newTestRouter`, change:
```go
	engine := NewEngine(db, key, actStub, hub.Broadcast, hub.Forget)
```
to:
```go
	engine := runs.NewEngine(db, key, actStub, hub.Broadcast, hub.Forget)
```

Replace both occurrences of `StatusSuccess` with `runs.StatusSuccess` — in `TestAPI_CreateRun_ReachesActWithEventPayload`'s `waitForStatus(t, db, created.RunID, StatusSuccess)` call and in `TestAPI_CreateRun_CapturesBranchAndCommitFromRealGitRepo`'s equivalent call.

In `TestAPI_CreateRun_ReachesActWithEventPayload`, change:
```go
	logs, err := GetRunLogs(db, created.RunID)
```
to:
```go
	logs, err := runs.GetRunLogs(db, created.RunID)
```

In `TestAPI_CreateRun_CapturesBranchAndCommitFromRealGitRepo`, change:
```go
	run, err := GetRun(db, created.RunID)
```
to:
```go
	run, err := runs.GetRun(db, created.RunID)
```

- [ ] **Step 5: Fix `internal/app/api.go`**

Add `"local-action/internal/runs"` to the import block. In the `NewRouter` signature, change:
```go
func NewRouter(db *sql.DB, key []byte, engine *Engine, hub *ws.Hub, actBin string) *http.ServeMux {
```
to:
```go
func NewRouter(db *sql.DB, key []byte, engine *runs.Engine, hub *ws.Hub, actBin string) *http.ServeMux {
```

In the `POST /api/runs` handler, change:
```go
		runID, err := engine.Enqueue(RunRequest{
```
to:
```go
		runID, err := engine.Enqueue(runs.RunRequest{
```

In the `GET /api/runs` handler, change:
```go
		runs, err := ListRuns(db, repoPath)
```
to:
```go
		runList, err := runs.ListRuns(db, repoPath)
```
(renaming the local variable to avoid shadowing the `runs` package — the very next line uses `writeJSON`, so update it too):
```go
		writeJSON(w, http.StatusOK, runs)
```
to:
```go
		writeJSON(w, http.StatusOK, runList)
```

In the `GET /api/runs/{id}` handler, change:
```go
		run, err := GetRun(db, id)
```
to:
```go
		run, err := runs.GetRun(db, id)
```
and:
```go
		logs, err := GetRunLogs(db, id)
```
to:
```go
		logs, err := runs.GetRunLogs(db, id)
```

- [ ] **Step 6: Fix `cmd/local-action/main.go`**

Add `"local-action/internal/runs"` to the import block, then change:
```go
	engine := app.NewEngine(db, key, *actBin, hub.Broadcast, hub.Forget)
```
to:
```go
	engine := runs.NewEngine(db, key, *actBin, hub.Broadcast, hub.Forget)
```

- [ ] **Step 7: Build and test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/runs from internal/app"
```

---

### Task 6: Extract `internal/httpapi`, retire `internal/app`

**Files:**
- Create: `internal/httpapi/api.go` (moved from `internal/app/api.go`)
- Create: `internal/httpapi/api_test.go` (moved from `internal/app/api_test.go`)
- Modify: `cmd/local-action/main.go`
- Modify: `docs/ARCHITECTURE.md`
- Delete: `internal/app/` (now empty)

**Interfaces:**
- Produces: `httpapi.NewRouter(db *sql.DB, key []byte, engine *runs.Engine, hub *ws.Hub, actBin string) *http.ServeMux`.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/httpapi
git mv internal/app/api.go internal/httpapi/api.go
git mv internal/app/api_test.go internal/httpapi/api_test.go
```

- [ ] **Step 2: Change package lines**

In both moved files, change `package app` to `package httpapi`. No other edits — every cross-package reference inside these two files (`runs.*`, `secrets.*`, `workflows.*`, `ws.*`) was already fully qualified by the end of Tasks 2–5; the two duplicated test helpers (`waitForStatus`, `runGit`) added in Task 5 Step 3 move along unchanged.

- [ ] **Step 3: Remove the now-empty `internal/app` directory**

```bash
rmdir internal/app
```
(If this fails because a stray file remains, `ls internal/app` and move or remove whatever's left — nothing should be there after Tasks 1–6.)

- [ ] **Step 4: Fix `cmd/local-action/main.go`**

Remove `"local-action/internal/app"` from the import block (this was its last remaining use) and add `"local-action/internal/httpapi"`. Change:
```go
	mux := app.NewRouter(db, key, engine, hub, *actBin)
```
to:
```go
	mux := httpapi.NewRouter(db, key, engine, hub, *actBin)
```

At this point `main.go`'s import block should contain (alongside the stdlib imports): `"local-action/internal/db"`, `"local-action/internal/httpapi"`, `"local-action/internal/runs"`, `"local-action/internal/secrets"`, `"local-action/internal/ws"` — and no more `"local-action/internal/app"`.

- [ ] **Step 5: Update `docs/ARCHITECTURE.md`**

Change the diagram and paragraph (lines 1–24) from:
```
Browser (React SPA, cmd/local-action/web/src)
   |  HTTP (REST) + WebSocket
   v
cmd/local-action (package main)
   |-- main.go          flags, wiring, embedded static file serving
   |-- embed.go         go:embed of web/dist (built frontend)
   |
   v  imports
internal/app (package app)
   |-- scanner.go       parse .github/workflows/*.yml (on: string/list/map, workflow_dispatch inputs)
   |-- secrets.go       encrypted secrets/vars CRUD (SQLite, AES-256-GCM via crypto.go)
   |-- runs.go          run history + log line storage (SQLite)
   |-- actrunner*.go    builds act argv, runs it via FIFO single-worker queue
   |-- ws.go            WebSocket hub: per-run buffered replay + live broadcast
   |-- api.go           HTTP routes wiring all of the above
   |
   v
act CLI  -->  Docker (host)
```

All backend logic lives in `internal/app` as one package (not split further — these files are small and interdependent enough that separate sub-packages would just add import ceremony). `cmd/local-action` is the thin entrypoint: flag parsing, wiring `internal/app`'s constructors together, and serving the embedded frontend. The frontend lives under `cmd/local-action/web/` rather than at the repo root because `go:embed` patterns can't reference paths outside the directory tree of the file that declares them.
```
to:
```
Browser (React SPA, web/src)
   |  HTTP (REST) + WebSocket
   v
cmd/local-action (package main)
   |-- main.go          flags, wiring, embedded static file serving
   |
   v  imports
internal/db          OpenDB, schema, migrations
internal/secrets     encrypted secrets/vars CRUD, AES-256-GCM
internal/workflows   .github/workflows/*.yml parsing, category overrides, saved event payloads
internal/runs        run history/log storage, the act-invoking Engine (FIFO single-worker queue)
internal/ws          WebSocket hub: per-run buffered replay + live broadcast
internal/httpapi     HTTP routes wiring all of the above
   |
   v
act CLI  -->  Docker (host)
```

Backend logic is split into domain packages under `internal/` (`db`, `secrets`, `workflows`, `runs`, `ws`, `httpapi`) instead of one flat package — each has one clear responsibility, and `httpapi` is pure wiring with no logic of its own. `cmd/local-action` is the thin entrypoint: flag parsing, wiring the packages' constructors together, and serving the embedded frontend. The frontend lives at top-level `web/` (see "Building the frontend" in the README for why `go:embed` needs its own file there).
```

Further down, change:
```
- **`db.SetMaxOpenConns(1)` + `PRAGMA busy_timeout=5000`** (db.go).
```
to:
```
- **`db.SetMaxOpenConns(1)` + `PRAGMA busy_timeout=5000`** (`internal/db/db.go`).
```

And change:
```
- `scanner.go`'s `ScanWorkflows` returns `nil` (not `[]`) when a repo has no workflow files; the frontend defends against this (`result || []`) but it's an inconsistency with the rest of the API's empty-list handling, worth normalizing if it ever bites.
```
to:
```
- `internal/workflows/scanner.go`'s `ScanWorkflows` returns `nil` (not `[]`) when a repo has no workflow files; the frontend defends against this (`result || []`) but it's an inconsistency with the rest of the API's empty-list handling, worth normalizing if it ever bites.
```

- [ ] **Step 6: Build and test**

```bash
go build ./...
go test ./...
```
Expected: `internal/app` no longer exists; six new packages under `internal/` build and test clean.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/httpapi, retire internal/app"
```

---

### Task 7: Relocate the frontend to top-level `web/`

**Files:**
- Move: `cmd/local-action/web/` → `web/` (all contents: `src/`, `dist/`, `package.json`, `package-lock.json`, `vite.config.js`, `index.html`)
- Delete: `cmd/local-action/embed.go`
- Create: `web/embed.go`
- Modify: `cmd/local-action/main.go`
- Modify: `.gitignore`
- Modify: `README.md`

**Interfaces:**
- Produces: `web.Dist embed.FS` — the embedded, built frontend, consumed by `main.go`.

- [ ] **Step 1: Move the frontend directory**

```bash
git mv cmd/local-action/web web
```
(`git mv` on a directory moves every tracked file inside it, including the committed `web/dist/*` build output — confirmed tracked via `git ls-files | grep dist` before this task.)

- [ ] **Step 2: Move the `go:embed` directive**

```bash
git rm cmd/local-action/embed.go
```
Create `web/embed.go`:
```go
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
```

- [ ] **Step 3: Fix `cmd/local-action/main.go`**

Add `"local-action/web"` to the import block. Change:
```go
	staticFS, err := fs.Sub(webDist, "web/dist")
```
to:
```go
	staticFS, err := fs.Sub(web.Dist, "dist")
```

- [ ] **Step 4: Fix `.gitignore`**

Change:
```
cmd/local-action/web/node_modules/
```
to:
```
web/node_modules/
```

- [ ] **Step 5: Update `README.md`**

In the Quick Start section, change:
```
Builds the frontend (first time only, or after `cmd/local-action/web/src` changes), builds the Go binary, and starts it. Open `http://localhost:8090`.
```
to:
```
Builds the frontend (first time only, or after `web/src` changes), builds the Go binary, and starts it. Open `http://localhost:8090`.
```

Replace the entire "## Building the frontend" section:
```
## Building the frontend

The UI is a Vite/React app under `cmd/local-action/web/`, built to static assets and embedded into the Go binary via `go:embed` (the frontend has to live alongside the binary's `main` package — `go:embed` can't reach outside its own directory tree). `make build`/`make run` handle this automatically (only rebuilds when `web/src` actually changed). To do it by hand instead:

```bash
cd cmd/local-action/web
npm install   # first time only
npm run build
cd ../../..
go build -o local-action ./cmd/local-action
```

For frontend-only iteration with hot reload, use `make dev`, or by hand (backend must be running separately on `:8090`):

```bash
cd cmd/local-action/web
npm run dev
```
```
with:
```
## Building the frontend

The UI is a Vite/React app at top-level `web/`, built to static assets and embedded into the Go binary via `go:embed` (declared in `web/embed.go` — `go:embed` can't reach outside the directory tree of the file that declares it, so the embed lives inside `web/` itself, and `cmd/local-action` imports the resulting `web.Dist` as an ordinary package). `make build`/`make run` handle this automatically (only rebuilds when `web/src` actually changed). To do it by hand instead:

```bash
cd web
npm install   # first time only
npm run build
cd ..
go build -o local-action ./cmd/local-action
```

For frontend-only iteration with hot reload, use `make dev`, or by hand (backend must be running separately on `:8090`):

```bash
cd web
npm run dev
```
```

Finally, in the "## Project layout" section, change:
```
cmd/local-action/     entrypoint (main.go), go:embed wiring, and the web/ frontend
  main.go             flags, wiring, HTTP server startup
  embed.go            //go:embed all:web/dist
  web/                Vite/React app (src/, dist/ built output, package.json)
internal/app/         all backend logic — one package, imported by cmd/local-action
  db.go               SQLite schema + OpenDB
  crypto.go           encryption key management, AES-GCM
  secrets.go          encrypted secrets/vars store
  scanner.go          .github/workflows/*.yml parsing
  runs.go             run history + log storage
  actrunner*.go       act argv builder + invocation engine (FIFO queue)
  ws.go               WebSocket log-streaming hub
  api.go              HTTP route wiring
testdata/sample-repo/  a real workflow file, for manual end-to-end testing
docs/                  architecture notes, design spec, implementation plan
```
to:
```
cmd/local-action/      entrypoint: main.go (flags, wiring, HTTP server startup)
web/                    Vite/React frontend (src/, dist/ built output, package.json, embed.go)
internal/
  db/                   SQLite schema + OpenDB
  secrets/              encrypted secrets/vars store, AES-GCM
  workflows/            .github/workflows/*.yml parsing, category overrides, saved event payloads
  runs/                 run history/log storage, act invocation engine (FIFO queue)
  ws/                   WebSocket log-streaming hub
  httpapi/              HTTP route wiring
testdata/sample-repo/   a real workflow file, for manual end-to-end testing
docs/                   architecture notes, design spec, implementation plan
```

- [ ] **Step 6: Build and verify the embed actually works**

```bash
cd web && npm install && npm run build && cd ..
go build -o local-action ./cmd/local-action
./local-action -addr 127.0.0.1:18090 &
sleep 1
curl -sf http://127.0.0.1:18090/ | grep -qi '<!doctype html' && echo "EMBED_OK"
kill %1
```
Expected: prints `EMBED_OK` (confirms `web.Dist` is actually embedded and served, not just that Go compiled).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move frontend from cmd/local-action/web to top-level web/"
```

---

### Task 8: Rewrite the Makefile

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

**Interfaces:** none (build tooling only).

- [ ] **Step 1: Replace the Makefile**

Replace the entire contents of `Makefile` with:
```makefile
WEB_DIR := web
WEB_SRC := $(shell find $(WEB_DIR)/src -type f) $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.js $(WEB_DIR)/index.html

.PHONY: build run dev test lint fmt install db-reset clean

build: $(WEB_DIR)/dist/index.html
	go build -o local-action ./cmd/local-action

$(WEB_DIR)/dist/index.html: $(WEB_SRC)
	cd $(WEB_DIR) && npm install && npm run build

run: build
	./local-action

# Runs backend + frontend dev server together. Ctrl-C stops both. The trap
# is installed immediately after backgrounding the backend (before the
# long-running `npm run dev` starts), and is the only cleanup — no separate
# trailing `kill`, since the trap already fires on normal exit too.
dev:
	@go run ./cmd/local-action & \
	BACKEND=$$!; \
	trap "kill $$BACKEND 2>/dev/null" EXIT INT TERM; \
	cd $(WEB_DIR) && npm run dev

test:
	go test ./...
	cd $(WEB_DIR) && npm test

lint:
	gofmt -l .
	go vet ./...

fmt:
	gofmt -w .

install:
	go mod download
	cd $(WEB_DIR) && npm install

db-reset:
	rm -f local-action.db

clean:
	rm -f local-action local-action.db
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules
```

- [ ] **Step 2: Update `README.md`'s command table**

Change:
```
| Command | Does |
|---|---|
| `make build` | Build frontend + binary, don't run |
| `make dev` | Backend (`go run .`) + frontend dev server (hot reload) together, Ctrl-C stops both |
| `make test` | `go test ./...` |
| `make fmt` | `gofmt -l .` + `go vet ./...` |
| `make clean` | Remove the binary, local DB, built frontend, and `node_modules` |
```
to:
```
| Command | Does |
|---|---|
| `make build` | Build frontend + binary, don't run |
| `make dev` | Backend (`go run .`) + frontend dev server (hot reload) together, Ctrl-C stops both |
| `make test` | `go test ./...` + frontend unit tests (`npm test`) |
| `make lint` | `gofmt -l .` + `go vet ./...` (check only, doesn't modify files) |
| `make fmt` | `gofmt -w .` — formats Go files in place |
| `make install` | Install Go + npm dependencies, no build |
| `make db-reset` | Remove the local SQLite DB (run history + secrets), keep the binary and built frontend |
| `make clean` | Remove the binary, local DB, built frontend, and `node_modules` |
```

Also update the "## Development" section near the bottom, changing:
```
## Development

```bash
make test   # go test ./...
make fmt    # gofmt -l . && go vet ./...
```
```
to:
```
## Development

```bash
make test   # go test ./... + npm test
make lint   # gofmt -l . && go vet ./... (check only)
make fmt    # gofmt -w . (formats in place)
```
```

- [ ] **Step 3: Verify every target runs**

```bash
make lint
make fmt
git diff --stat   # fmt should produce no diff if the tree was already formatted
make test
make install
make db-reset     # harmless no-op if local-action.db doesn't exist
make build
```
Expected: every command exits 0. `make fmt` should report no changes (the tree was already `gofmt`-clean before this plan started); if it does change anything, that's pre-existing unformatted code being fixed as a side effect — review the diff before committing.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "build: rewrite Makefile — unify WEB_DIR, add lint/fmt/install/db-reset, run frontend tests"
```

---

### Task 9: Final verification

**Files:** none (verification only).

- [ ] **Step 1: Full clean build from scratch**

```bash
make clean
make build
```
Expected: succeeds with no leftover state from previous tasks.

- [ ] **Step 2: Full test suite**

```bash
make test
```
Expected: all Go tests across `internal/db`, `internal/secrets`, `internal/workflows`, `internal/runs`, `internal/ws`, `internal/httpapi` pass, plus the frontend's `node --test` run (`format.test.js`, `logparse.test.js`) — confirm the frontend tests actually execute (their names should appear in the output), since before this plan `make test` never ran them.

- [ ] **Step 3: Lint check**

```bash
make lint
```
Expected: no output from `gofmt -l .` (nothing unformatted), `go vet ./...` exits 0.

- [ ] **Step 4: Smoke-test `make dev`**

```bash
make dev &
MAKE_PID=$!
sleep 3
curl -sf http://127.0.0.1:8090/api/health && echo
curl -sf http://127.0.0.1:5173/ >/dev/null && echo "VITE_OK"
kill $MAKE_PID 2>/dev/null
wait $MAKE_PID 2>/dev/null
sleep 1
# `make`'s own SIGTERM forwarding to its recipe shell doesn't reliably reach
# that shell's foreground child (npm/vite) — clean up directly by pattern
# rather than trusting the signal to have propagated all the way down.
pkill -f "go-build.*local-action" 2>/dev/null
pkill -f "vite" 2>/dev/null
sleep 1
pgrep -f "cmd/local-action|vite" && echo "LEAK: dev processes still running" || echo "CLEAN"
```
Expected: the health check returns JSON (`actOK`/`dockerOK` fields, regardless of their boolean values — this only confirms the backend answered), `VITE_OK` prints (confirms Vite's dev server came up on its default port 5173), and the final line prints `CLEAN` — no leftover `go run`/`vite` processes. If it prints `LEAK: ...` instead, note it as a known limitation of `make dev`'s signal handling (killing `make` doesn't guarantee its recipe's foreground child dies too) rather than blocking on it — it's outside this plan's scope to fix process-group signal propagation, only the trap-timing race called out in the design spec.

- [ ] **Step 5: Confirm no stale references remain**

```bash
grep -rn "internal/app" --include="*.go" --include="*.md" . || echo "CLEAN"
grep -rn "cmd/local-action/web" --include="*.go" --include="*.md" . || echo "CLEAN"
```
Expected: both print `CLEAN` (no matches) — every reference to the old package and the old frontend path has been updated across code and docs.

- [ ] **Step 6: Final commit (only if Steps 1–5 changed anything, e.g. gofmt fixes)**

```bash
git status --porcelain
```
If clean, nothing to commit — this task was verification-only. If not clean, review the diff, then:
```bash
git add -A
git commit -m "chore: final verification pass for package split / web relocation / Makefile cleanup"
```
