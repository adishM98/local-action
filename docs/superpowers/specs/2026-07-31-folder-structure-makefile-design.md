# Design: package split, web/ relocation, Makefile cleanup

Date: 2026-07-31

## Problem

`internal/app` is a single flat package holding 16 files across unrelated
concerns (HTTP routing, run engine, secrets/crypto, workflow scanning,
websocket hub, DB schema). The frontend lives at `cmd/local-action/web/`,
nested inside a Go binary package for no reason other than a `go:embed`
constraint. The Makefile's `dev` target works but its background/trap shell
logic is fragile, `make test` silently skips the frontend's own test script,
and `fmt` only checks formatting despite its name.

## 1. Package split

`internal/app` (16 files, one package) splits into six domain packages:

```
internal/db/         db.go
internal/runs/       runs.go, actrunner.go, actrunner_argv.go, gitinfo.go
internal/secrets/    secrets.go, crypto.go
internal/workflows/  scanner.go, workflowcategory.go, eventpayload.go
internal/ws/         ws.go
internal/httpapi/    api.go
```

Rationale for each grouping:
- **db**: `OpenDB` + schema + migrations. Depended on by `main` only (each
  domain package takes `*sql.DB` as a parameter, matching the existing
  style — no package depends on `db` directly except `main`, which opens it
  and passes it down).
- **runs**: the `Run` model/CRUD, the `Engine` (run queue + `act` subprocess
  execution), `BuildArgv`, and `gitInfo` — `gitInfo` has exactly one caller
  (`Engine.Enqueue`) so it stays with the engine rather than becoming its
  own package.
- **secrets**: the encrypted secrets/vars store and the AES-256-GCM
  crypto helpers it depends on.
- **workflows**: everything keyed by `(repoPath, workflowFile)` that isn't a
  run or a secret — scanning `.github/workflows/*.yml`, category overrides,
  and saved event payloads. These three files don't share code today but
  share a shape (small metadata store scoped to a workflow file) and are
  the natural catch-all for "workflow-level config."
- **ws**: the websocket `Hub`, unchanged, already single-purpose.
- **httpapi**: `NewRouter` and nothing else — pure wiring, imports the other
  five.

Dependency graph: `httpapi → runs, secrets, workflows, ws, db`;
`runs → secrets` (the engine decrypts secrets/vars before writing them to
the temp dotenv files `act` reads). No cycles.

Exported names are unchanged (`runs.CreateRun`, `secrets.UpsertSecret`,
etc.) — dropping the repetition (`runs.Create`) is a separate, unrequested
cleanup and out of scope. The diff is: move each file to its new directory,
change its `package` line, fix cross-package references (mostly adding a
package qualifier), fix imports in `cmd/local-action/main.go` and
`internal/httpapi/api.go`. `_test.go` files move with the file they test.

## 2. `web/` relocation

`cmd/local-action/web/` moves to `/web`. `go:embed` directives can only
reach files inside the directory tree of the `.go` file that declares
them, so the embed itself has to move too:

- Delete `cmd/local-action/embed.go`.
- Add `web/embed.go`:
  ```go
  package web

  import "embed"

  //go:embed all:dist
  var Dist embed.FS
  ```
- `cmd/local-action/main.go` imports `local-action/web` and does
  `fs.Sub(web.Dist, "dist")` instead of referencing a local `webDist` var.

Net effect: `web/` becomes an ordinary top-level frontend project directory
(source, `package.json`, build output all together); the only tie back to
the Go binary is a one-line import in `main.go`.

## 3. Makefile

`web/vite.config.js` already proxies `/api` and `/ws` to `localhost:8090`,
so `make dev` (backend via `go run`, frontend via `vite`) is already the
"one command" unified dev flow in spirit. The complaint is the shell
plumbing (background the backend, trap to kill it on exit), which gets
tightened — trap installed before the backend is backgrounded, so there's
no window where an early Ctrl-C leaves it orphaned — not replaced with a
different architecture.

Separately: `web/package.json` already defines a `test` script
(`node --test 'src/**/*.test.js'`) that `make test` never invokes, so
`format.test.js` and `logparse.test.js` currently don't run via `make
test` at all. Fixed as part of this pass.

Target list (`WEB_DIR` now `web`, was `cmd/local-action/web`):

| Target | Change |
|---|---|
| `build` | unchanged behavior |
| `run` | unchanged behavior |
| `dev` | same approach, trap installed before backgrounding |
| `test` | now also runs `cd $(WEB_DIR) && npm test` |
| `lint` | renamed from today's `fmt` (which only checks: `gofmt -l .` + `go vet ./...`) |
| `fmt` | new — `gofmt -w .`, actually writes |
| `install` | new — `go mod download` + `npm install` in `$(WEB_DIR)`, no build |
| `db-reset` | new — `rm -f local-action.db` |
| `clean` | unchanged (still removes binary, db, dist, node_modules) |

## Testing

No new test coverage needed — this is a structural move, not a behavior
change. Existing Go tests move with their files and must still pass
unmodified (package-internal tests stay `package <newpkg>`, no test logic
changes). `go build ./...`, `go test ./...`, and `make build` all passing
is the verification bar. Frontend is untouched except the directory move
and the new `web/embed.go`.

## Out of scope

- Renaming exported functions to drop package-name stutter.
- Any change to `docs/` layout (raised as a folder-structure gripe but
  deferred — separate, unrelated cleanup).
- Replacing the Vite-proxy dev setup with a file-watcher/build-tag based
  one (considered, rejected — reinvents what Vite's dev server already
  does).
