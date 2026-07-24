# GitHub-style UI redesign — design spec

Date: 2026-07-24
Status: approved pending user review

## Goal

Make local-action look and behave like GitHub's Actions UI, since users already know it. Covers five user findings:

1. UI should look like GitHub.
2. Docker health indicator showed red while Docker was actually running.
3. Secrets/vars should be scoped either repo-wide or to a specific workflow, chosen when adding.
4. Interaction patterns should match GitHub Actions.
5. Triggering a run should show that run's live view, not dump the user on the History tab.

## Approach (chosen: A)

Evolve the existing Vite/React SPA in place. No new runtime dependencies, no router, no component library. Hand-rolled GitHub Primer-like dark CSS. Run `act` with `--json` so log lines carry structured job/step fields; group logs client-side into GitHub-style collapsible job/step sections.

Rejected: `@primer/react` (heavy dependency tree inside an embedded binary for ~5 components), full rewrite with react-router (3 views don't need URL routing).

## 1. Layout & navigation

Single dark theme, Primer-like palette: `#0d1117` background, `#161b22` panels, `#30363d` borders, GitHub green/red/yellow status colors.

- **Top bar** (kept): repo path input with recent-paths datalist (this plays the role of GitHub's repo name), copy button, act/docker health dots.
- **Left sidebar**: "All workflows" entry + one entry per scanned workflow. Scan happens automatically on repo-path change (blur/enter) — the manual Scan button is removed. Bottom of sidebar: "Secrets and variables" link.
- **Main area** — three views, plain component state (no router):
  1. **Runs list**: runs for the selected workflow (or all workflows). Row = status icon, workflow name + run number, event, relative time, duration. A "Run workflow ▾" button opens a dropdown with event picker and `workflow_dispatch` inputs form, matching GitHub's affordance.
  2. **Run detail**: opened by clicking a run row, and immediately after triggering a run (fixes finding 5).
  3. **Secrets and variables** page (section 3).
- The History tab is removed as a concept — the runs list is the history, filterable per workflow.

## 2. Run detail page & log grouping

- **Header**: status badge, workflow name, run number, event, trigger time, duration. Cancel button while running. Re-run button re-POSTs `/api/runs` with the run's stored workflow/event/inputs (no new endpoint).
- **Engine change**: `BuildArgv` adds `--json`. Each act log line is then JSON with `jobID`, `step`, `stepID`, `stepResult`, `jobResult`, `raw_output` (verified against act 0.2.76 output).
- **Client-side parser** groups lines:
  - One card per job (`jobID`), status icon from the `jobResult` line.
  - One collapsible row per step (`step`), ✓/✗/spinner from `stepResult`. Auto-expanded while running, auto-collapsed on success (GitHub behavior). Lines with `raw_output: true` are the user's actual command output; act's docker chatter lands in "Set up job" as on GitHub.
  - Monospace, line numbers.
- **Fallback**: lines that don't parse as JSON (act sometimes prints bare text, and all pre-redesign runs) render in a catch-all "Output" section. Nothing is lost.
- **Transport/storage unchanged**: WebSocket still streams raw lines; `run_logs` still stores raw lines; finished runs backfill from `GET /api/runs/{id}`. Parsing is purely client-side.

## 3. Secrets & variables scoping

- **Data model**: `secrets` table gains `workflow_file TEXT NOT NULL DEFAULT ''`; uniqueness becomes `(repo_path, kind, key, workflow_file)`. Empty string = repo-wide. Existing rows keep working unchanged (they default to repo-wide). Migration: `ALTER TABLE` + index rebuild on startup.
- **Injection precedence**: for a run of workflow W, merge repo-wide + W-specific values; W-specific wins on name clash. Same encrypted-at-rest AES-256-GCM storage and 0600 temp dotenv mechanism as today.
- **UX** (GitHub "Secrets and variables" settings page style):
  - Tabs: Secrets / Variables. Table: name, scope badge (`Repository` or the workflow filename), delete button.
  - "New secret / New variable" form: name, value, scope picker — (•) All workflows in this repo, ( ) Specific workflow ▾ (dropdown of scanned workflows).
  - The "Run workflow ▾" dropdown shows "N secrets, M vars will be injected", linking to the secrets page pre-filtered to that workflow.
  - Values are write-only after save (update = overwrite), matching GitHub.

## 4. Docker health UX

- `/api/health` additionally returns `dockerError` (stderr snippet from `docker info`) so the UI can say why it's red.
- Frontend polls every 5s while any check is failing, 30s when healthy. Clicking a dot forces an immediate recheck.
- Runs view shows a warning banner when docker is down: "Docker not running — runs will fail."
- Diagnosis of finding 2: backend and frontend detection both work; the red dot came from `docker info` exceeding the 3s health timeout while Docker Desktop was starting, then a 20s poll gap. Faster unhealthy-state polling plus the error message fix the perceived bug.

## 5. Backend API changes (summary)

| Change | Where |
|---|---|
| `--json` in act argv | `actrunner_argv.go` |
| `workflow_file` column + precedence merge | `secrets.go`, `db.go`, `actrunner.go` |
| Secrets endpoints accept/return `workflowFile` | `api.go` |
| `/api/health` returns `dockerError` | `api.go` |
| `ScanWorkflows` returns `[]` not `nil` (documented wart) | `scanner.go` |

No changes to: runs table, run_logs, WebSocket hub, queueing, encryption.

## 6. Error handling

- Scan failure: inline message in sidebar ("not a repo path" / "no workflows found").
- Run trigger failure: error shown on the run form.
- WebSocket disconnect during a live run: "reconnecting…" indicator, fall back to polling `GET /api/runs/{id}`.

## 7. Testing

- Go: secrets precedence merge, migration preserves existing rows, argv includes `--json`, scanner `[]` fix, existing suite stays green.
- Frontend: the log parser is the only non-trivial JS logic — one plain `node:test` file, no new dev dependencies.

## Out of scope

- Multi-user/auth, remote repos, per-job runs, concurrent runs (existing documented limitations stand).
- Pixel-perfect Primer fidelity — "unmistakably GitHub-like" is the bar.
- Light theme.

## Note (unrelated to code)

User's installed act 0.2.76 self-reports CVE-2026-34041 / CVE-2026-34042; upgrade to ≥ 0.2.86 recommended.
