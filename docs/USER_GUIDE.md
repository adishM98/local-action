# User Guide

A full walkthrough of every screen and feature. For install/build instructions see the [README](../README.md); this doc assumes the app is already running at `http://localhost:8090`.

## Pointing it at a repo

The sidebar shows an editable path field until you set one. Enter the absolute path to a local repo containing `.github/workflows/` — it must be a real path on the machine running the server, not a GitHub URL. Once set, the sidebar shows the repo name and its current git branch; click that line to change the path again. Up to 8 recently-used paths are remembered and offered as autocomplete.

## Overview

The landing page. What it shows depends on state:

- **No repo path set** — just a prompt to enter one.
- **Repo set, nothing has ever run** — an empty state pointing you at the sidebar.
- **Otherwise** — four stat tiles (Workflows / Running / Failed / Passing), then:
  - **Running** — any runs currently in progress, if there are any.
  - **Failures** — the 5 most recent failed runs, if there are any.
  - **Recent runs** — the 6 most recent runs across every workflow.
  - **Repository health** — success rate, average build time, average queue time, longest run, plus a 7-day success-rate bar chart (hover a bar for the day's success %).
  - **Pinned workflows** — your ⭐'d workflows (see Favorites below), if any.

Every row is clickable and opens that run or workflow directly.

## Workflow Explorer (sidebar)

Workflows are grouped into categories, auto-detected from the workflow's name and file path, most-specific match wins:

| Category | Matched on |
|---|---|
| Security | security, vulnerab*, licen*, compliance, grype, cve |
| Deployment | deploy, publish, render, netlify |
| Testing | test, cypress, coverage, e2e |
| Docs | docs, storybook |
| CI/Build | build, docker, packer, ami, image, or a whole-word "ci" |
| Other | anything that matches none of the above |

(A workflow named "Grype - Docker Image Vulnerability Scan" lands in Security, not CI/Build — security keywords are checked first.) Click a category header to collapse/expand it.

- **Search** — filters the list by name. Press **⌘K**/**Ctrl+K** anywhere in the app to jump straight to this field — the only global keyboard shortcut currently wired up.
- **Filter** — narrow the list to workflows whose *last run* was Running, Failed, Success, or Never run.
- **Favorites** — click the star on any workflow row to pin it; pinned workflows get their own section above the category list and show up on the Overview page.
- Drag the sidebar's right edge to resize it, or click the collapse icon to shrink it to a thin strip.

## Running a workflow

Select a workflow, then click **Run workflow ▾**:

1. **Event** — pick which trigger to simulate, from the workflow's declared `on:` triggers.
2. **Inputs** — if you picked `workflow_dispatch`, its declared inputs render as text fields, dropdowns (for `choice` inputs), or checkboxes (for `boolean` inputs). Required inputs are marked with `*`.
3. **Event payload (JSON)** — only appears when a job's `if:` condition depends on event data (like a PR label) that GitHub Actions can't derive locally on its own:
   - If every relevant condition can be solved with full confidence, this field is skipped entirely — the payload is built automatically.
   - Otherwise it's shown pre-filled with a best-effort guess (merged from every job in the workflow, not just one), with a note to double-check it before running. A **"Reset to suggestion"** link appears if you (or a previous run) edited it away from that guess.
   - If nothing could be guessed at all, it's shown empty for you to fill in by hand.
   - Whatever you submit here is remembered per-workflow for next time.
4. Click **Run workflow**.

If the workflow's `runs-on` targets `windows-*`, `macos-*`, or `self-hosted`, a banner warns that `act` only emulates Linux locally — expect that job to fail or diverge from real CI.

## Watching a run

Opens a slide-out drawer with a live log stream, grouped by job then by step. Steps auto-expand while running or failed, and auto-collapse on success (a manual click always overrides this). While a run is in progress:

- **Cancel** stops it.
- Once it reaches a final state (passed/failed/cancelled), **Re-run** replays the same event/inputs as a new run.

Runs execute one at a time, FIFO — starting a second run while one is in progress queues it rather than running both at once.

## Secrets & variables

Add secrets or variables scoped either to the whole repo or to one specific workflow — a workflow-scoped entry only applies when that workflow runs. Values are write-only: once saved, you can't view them again, only replace or delete them. If a workflow references a secret/var that isn't stored yet, it's listed under "Detected in this workflow" as a one-click chip to add it.

Values are encrypted at rest (AES-256-GCM) and only exist in plaintext in a short-lived temp file for the duration of a run, deleted immediately after.

## Theme

The Sun/Moon/Monitor icon next to the Docker/Act status pills switches between System, Light, and Dark. "System" follows your OS preference live.

## Troubleshooting

- **Docker/Act pills red** — click either one to re-check. A red Docker pill means runs will fail outright; hover for the specific error.
- **A workflow's event payload doesn't reflect a code change you just made** — the frontend hot-reloads under `make dev`, but the Go backend does not; restart the server (`make dev` again, or rebuild with `make run`) after backend changes.
- **The Event payload field shows an old value after a fix** — a previously-saved payload for that workflow takes priority over a freshly computed suggestion. Use **Reset to suggestion** to discard it.
