# local-action

Selfhosted web UI for running GitHub Actions workflows locally, via Docker — wraps [`act`](https://github.com/nektos/act) so you get a browser UI (secrets, env/vars, run history, live logs) instead of memorizing CLI flags.

Single user, no auth, no accounts. Meant to run on your own machine or a home server, bound to `localhost` only.

## Requirements

- **Go 1.22+** (only if building from source)
- **Docker**, installed and running
- **[`act`](https://github.com/nektos/act)**, installed and on your `PATH` (`act --version` should work)
- **Node.js + npm** (only if rebuilding the frontend)

## Quick start

```bash
make run
```

Builds the frontend (first time only, or after `web/src` changes), builds the Go binary, and starts it. Open `http://localhost:8090`.

Other targets:

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

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:8090` | Listen address. Change only if you understand the risk — see [Security](#security). |
| `-db` | `local-action.db` | Path to the SQLite database file (run history + encrypted secrets). |
| `-act-bin` | `act` | Path to the `act` executable, if not on `PATH`. |

## Using it

1. **Workflows tab** — enter the absolute path to a local repo (must contain `.github/workflows/`), click Scan. Pick a workflow, pick which trigger event to simulate, fill in any `workflow_dispatch` inputs, click Run.
2. **Secrets tab** — add secrets/vars scoped to that repo path. Values are encrypted at rest and injected into the run as a temporary dotenv file, deleted immediately after the run finishes.
3. **History tab** — see past and in-progress runs for the current repo path. Click one to watch its log stream live (or read back the full log if it already finished).

Runs execute one at a time, FIFO — triggering a second run while one is in progress queues it.

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

## Project layout

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

## Security

- No authentication. Anyone who can reach the listening address can trigger workflow runs (arbitrary code execution via Docker) and manage secrets. Keep `-addr` on `127.0.0.1` (the default) unless you understand and accept that risk — don't expose this to a network you don't fully trust.
- Secrets/vars are encrypted at rest with AES-256-GCM. The encryption key lives at `$XDG_CONFIG_HOME/local-action/seed.key` (or the OS equivalent), generated once on first run, permissions `0600`.
- Decrypted secret values are written to short-lived temp dotenv files (`0600`) only for the duration of a run, then deleted.

## Development

```bash
make test   # go test ./... + npm test
make lint   # gofmt -l . && go vet ./... (check only)
make fmt    # gofmt -w . (formats in place)
```

See `docs/ARCHITECTURE.md` for how the pieces fit together, and `docs/superpowers/` for the original design spec and implementation plan.
