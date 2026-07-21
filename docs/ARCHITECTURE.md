# Architecture

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

## Request flow: triggering a run

1. Browser `POST /api/runs` with `{repoPath, workflowFile, event, inputs}`.
2. `Engine.Enqueue` (actrunner.go) inserts a `queued` row via `CreateRun` (runs.go) and pushes onto a buffered channel.
3. The engine's single worker goroutine (`Engine.worker`) pops the run, exclusively — only one `act` process ever runs at a time.
4. `runOne` writes the repo's secrets/vars to short-lived `0600` temp dotenv files (`writeDotenvTemp`), builds the `act` argv (`BuildArgv`, actrunner_argv.go), and spawns `act` with `cmd.Dir = repoPath`.
5. `act`'s stdout/stderr are streamed line-by-line into two places: `AppendRunLog` (persisted to SQLite `run_logs`) and `Hub.Broadcast` (live WebSocket fan-out to any connected client, `ws.go`).
6. On exit, the run's status is set to `success`/`failed`/`cancelled` (`UpdateRunStatus`), temp files are removed, and `Hub.Forget` releases the in-memory WebSocket buffer for that run (the DB remains the durable source of truth for replay).

## Request flow: viewing a run's logs

- `GET /api/runs/{id}` returns `{run, logs}` — `logs` is the full persisted log from SQLite, used by the frontend (`LogViewer.jsx`) to backfill anything that happened before the viewer connected, for runs that have already reached a terminal state.
- `GET /ws/runs/{id}` upgrades to a WebSocket. The hub replays whatever it still has buffered for that run (only meaningful while the run is active or very recently finished — buffers are freed on completion) and then streams new lines live.

## Concurrency invariants

- **One `act` process at a time.** Enforced structurally: a single buffered channel drained by a single worker goroutine (`Engine.worker`). No locking needed for exclusion — there's simply one consumer.
- **`db.SetMaxOpenConns(1)` + `PRAGMA busy_timeout=5000`** (db.go). `modernc.org/sqlite` has no built-in wait-on-lock behavior across multiple pooled connections; the concurrent writers this app has (log-streaming goroutines, status updates, HTTP reads) would otherwise intermittently hit `SQLITE_BUSY`. Forcing everything through one connection with a busy timeout serializes access safely. Fine for this app's scale (single local user).
- **`e.running` map** (actrunner.go) is guarded by a mutex on every access (registration, `Cancel`, deletion). The process is registered in the map *before* the DB status flips to `running`, so a client that observes "running" via polling can never race ahead of `Cancel()`'s ability to find and kill the process.
- **WebSocket hub buffer/registration** happen under one lock in `ServeWS`, so a line broadcast concurrently with a new client connecting can't be dropped or double-delivered.

## Storage

SQLite (pure-Go driver, no cgo — keeps the binary a portable single static executable), three tables:

- `secrets` — `(repo_path, kind, key) -> value_encrypted`. `kind` is `secret` or `var`.
- `runs` — one row per triggered run: workflow/event/inputs/status/timestamps.
- `run_logs` — `(run_id, line_no) -> text`, the durable log.

Encryption key: 32 random bytes, generated once, stored at `$XDG_CONFIG_HOME/local-action/seed.key` (`0600`). AES-256-GCM for secret/var values.

## Known limitations (by design, see the original spec for rationale)

- Single user, no auth — never expose beyond `127.0.0.1` without understanding the risk.
- Local filesystem repo paths only — no git clone/remote/auth handling.
- Whole-workflow runs only — no per-job selection.
- One run at a time — no concurrency.
- `scanner.go`'s `ScanWorkflows` returns `nil` (not `[]`) when a repo has no workflow files; the frontend defends against this (`result || []`) but it's an inconsistency with the rest of the API's empty-list handling, worth normalizing if it ever bites.
