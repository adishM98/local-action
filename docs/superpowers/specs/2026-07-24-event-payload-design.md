# Custom event payload for `if:`-gated workflows — design spec

Date: 2026-07-24
Status: approved pending user review

## Goal

Jobs gated with `if:` conditions on event context (e.g. `if: github.event.action == 'labeled' && github.event.label.name == 'run-ci'`) never run locally, because `act` has no flag to skip `if:` evaluation — the condition is genuinely false since `github.event` is empty without an explicit event payload. `act` supports `-e/--eventpath <file>`, a JSON file used as `github.event`. This feature lets the user supply and save that payload per workflow, so gated jobs can be tested locally by giving `act` the same context data GitHub would have sent.

This is not a bypass of `if:` — it's giving the condition the inputs it needs to evaluate true, same as GitHub would.

## 1. Storage

New table (plain text, not encrypted — this is test-fixture data, not a secret):

```sql
CREATE TABLE IF NOT EXISTS event_payloads (
  repo_path TEXT NOT NULL,
  workflow_file TEXT NOT NULL,
  payload TEXT NOT NULL,
  PRIMARY KEY (repo_path, workflow_file)
);
```

One payload per workflow (not repo-wide/global like secrets — the payload shape is inherently tied to one workflow's specific `if:` conditions).

New functions in `internal/app/secrets.go`-adjacent file (`internal/app/eventpayload.go`):
- `GetEventPayload(db, repoPath, workflowFile) (string, error)` — empty string if none saved.
- `SaveEventPayload(db, repoPath, workflowFile, payload string) error` — upsert; deletes the row if `payload == ""` (clearing it back to "no payload").

New endpoints in `api.go`:
- `GET /api/event-payload?repoPath=&workflowFile=` → `{"payload": "..."}`
- `POST /api/event-payload` body `{repoPath, workflowFile, payload}` → validates `payload` is either empty or well-formed JSON (`json.Valid`), 400 with a clear message otherwise; upserts/clears; 204 on success.

## 2. Run wiring

`RunRequest` gains `EventPayload string `json:"eventPayload"`` (raw JSON text, may be empty).

`BuildArgv(req, secretFile, varFile, envFile, eventPayloadFile string) []string` — appends `-e eventPayloadFile` only when `eventPayloadFile != ""`. Fully backward compatible: every workflow that doesn't set a payload gets today's exact argv.

`Engine.writeTempFiles` writes `req.EventPayload` to a 0600 temp file (same pattern as the secret/var/empty-env files) only when non-empty; passes `""` for `eventPayloadFile` otherwise, so `BuildArgv` omits `-e`. Cleaned up alongside the other temp files.

`POST /api/runs` handler passes `body.EventPayload` through to `RunRequest`. Validated as JSON there too (400 on malformed) — the run endpoint doesn't only trust client-side validation, since it's also usable directly via curl/API.

## 3. UI

`RunWorkflowMenu`'s dropdown panel gets a collapsed-by-default disclosure: **"Event payload (JSON) ▾"**, placed below the dispatch inputs. Opening it fetches `GET /api/event-payload` for `(repoPath, workflow.file)` into a textarea (monospace, a few rows tall). Placeholder text shows a minimal example shape (e.g. `{"action": "labeled", "label": {"name": "run-ci"}}`) so the user isn't starting from a blank box.

On clicking "Run workflow": if the textarea is non-empty, `JSON.parse` it client-side first — malformed JSON shows an inline error and blocks the run (no request sent). If valid (or empty), `POST /api/event-payload` saves it (or clears the row if emptied), then the run is created with `eventPayload` included in the request body.

## 4. Testing

- Go: `GetEventPayload`/`SaveEventPayload` round-trip + clear-on-empty; `BuildArgv` with and without an event payload file; `POST /api/event-payload` rejects malformed JSON with 400; `POST /api/runs` with a payload end-to-end (stub `act` script that echoes the `-e` file's contents, asserting it reaches the process).
- No new JS test file — the textarea + fetch-on-open + client-side `JSON.parse` check is the same shape as existing secrets-count fetching in `RunWorkflowMenu`, not new logic worth a `node:test`.

## Out of scope

- Presets/guided builders for common event shapes (label added, PR opened) — raw JSON only, matches how `act`'s own `-e` flag works and keeps this from becoming an ever-expanding preset library.
- Payload sharing across workflows (each workflow's saved payload is independent, matching how differently each workflow's `if:` conditions read event fields).
