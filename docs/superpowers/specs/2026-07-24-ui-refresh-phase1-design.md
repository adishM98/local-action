# UI Refresh Phase 1: visual polish, dashboard stats, richer rows — design spec

Date: 2026-07-24
Status: approved pending user review

## Context

Full PRD ("Local Action UI Refresh") lists 20 changes spanning visual polish, new backend data (branch/commit SHA, pagination, filters), and an architecture change (run detail as a side drawer). Too large for one spec — decomposed into phases:

- **Phase 1 (this spec)**: typography/spacing polish, status pill badges, dashboard stat cards, richer run rows. No schema changes, no new APIs — everything computed from data already loaded.
- Phase 2 (later): sidebar regrouping with categories/icons, breadcrumb.
- Phase 3 (later): search + status/event filters (client-side).
- Phase 4 (later): branch/commit SHA capture (new `runs` column, git introspection at run time) + pagination.
- Phase 5 (later): run detail as a side drawer (replaces full-page navigation) + live step-progress bar.
- Phase 6 (later): recent-activity timeline, richer empty states.

## Decisions carried from brainstorming

- **Status colors stay GitHub's real convention**, not the PRD's suggested Vercel/Linear palette: success green `#3fb950`, failed red `#f85149`, running amber `#d29922` (pulsing), queued/cancelled muted gray. `--blue` stays reserved for links/info, never status — avoids clashing with the existing accent/link scheme and keeps the original "look like GitHub" brief intact.
- **Dashboard stats window**: all runs for the selected workflow (or across the whole repo on "All workflows"), not a rolling last-N — matches what's already loaded via `ListRuns`, no new query.
- **Branch/commit fields are NOT included yet** (Phase 4) — rows show trigger event + relative time + duration only.

## 1. Typography & spacing

- Run row workflow name: 16px → **18px, weight 600** (was already 600 at 14px effectively via `.run-row__name`; bumping size).
- Row metadata line (event • relative time): **13px, `--muted`**.
- Timestamp/duration text: **12px, gray** (`--muted`, already the case for `.run-row__duration` — kept, just confirmed).
- Spacing tokens, applied via existing CSS custom properties pattern:
  - Card padding (stat cards, job cards): **20px** (job-card header padding stays as-is structurally; stat cards use this fresh).
  - Row padding (run rows): **18px** vertical/horizontal (up from `10px 16px`).
  - Sidebar item spacing: **12px** vertical rhythm between groups (up from current tighter 4px gaps — sidebar heading/item padding adjusted).
  - Section spacing (between stat-card row, filter area if added later, and the run list): **24px**.

## 2. Status pills

Replace the bare `StatusIcon` glyph-only rendering in run rows and the run-detail header with a colored pill: rounded background (dim tint of the status color, e.g. `rgba(63,185,80,0.15)` for success), status-color text, small glyph retained inside. Component: extend `StatusIcon.jsx` with a `pill` prop (or a new `StatusBadge.jsx` wrapping it) — reused in `RunsView`'s rows and `RunDetail`'s header. Job/step icons inside the log view stay as plain glyphs (pills would be too heavy for a dense list of steps).

## 3. Dashboard stat cards

A row of 5 small cards rendered below the page title in `RunsView`, above the run-rows list:

| Runs | Passed | Failed | Running | Avg duration |
|---|---|---|---|---|

- **Runs**: `runs.length` (post-filter: for a specific workflow, only that workflow's runs; for "All workflows", every run for the repo).
- **Passed / Failed**: count where `status === 'success'` / `status === 'failed'`.
- **Running**: count where `status === 'running'` (queued counted separately, not shown as its own card per this PRD's card list — cancelled/queued runs still count toward "Runs" total but aren't broken out as their own card here).
- **Avg duration**: mean of `finishedAt - startedAt` (both must be set — i.e. status is success/failed/cancelled) formatted via a new small helper, `0` finished runs → card shows `—` instead of `NaN`/`0s`.
- Cards are hidden entirely when `runs.length === 0` (no stats to show — the existing "No runs yet" empty state covers that case).
- Pure client-side computation in `RunsView.jsx` from the `runs` array already fetched via `api.listRuns` — no backend change.

## 4. Richer run rows

Each row in `.run-rows` becomes:

```
[pill: Passed]  CI #12                                    6m 11s
                workflow_dispatch • 2m ago
```

- Status pill (left).
- Workflow name + run number, 18px/600 (top line).
- `event • relativeTime`, 13px muted (second line) — replaces the current single combined meta line, same data, restyled.
- Duration, right-aligned, 12px muted — unchanged data source (`duration()` from `format.js`), just restyled per the spacing/type updates above.
- Hover: subtle elevation (`box-shadow`) + border tint in addition to the existing background change, per the "better row hover" ask. No overflow menu (`⋮`) yet — deferred to Phase 5's drawer, since a menu with no actions to offer today (re-run/cancel already live at row-click → detail page) would be empty chrome.

## Testing

- New pure function `computeRunStats(runs)` (in `format.js`, alongside `duration`/`relativeTime`) gets `node:test` coverage: empty array → all zeros/`—`; mixed statuses → correct counts; average duration correct across only-finished runs; a `running` (unfinished) run excluded from the average.
- No backend changes — no Go tests needed for this phase.
- Visual verification: manual check against the running server (this is a CSS/layout-heavy change) — no automated visual regression tooling in this project.

## Out of scope (deferred to later phases)

- Sidebar regrouping/icons/collapsible sections (Phase 2).
- Search bar, status/branch/event/date filters (Phase 3 — branch/date need Phase 4's data first).
- Branch, commit SHA, pagination (Phase 4).
- Run detail as a side drawer, live step-progress bar (Phase 5).
- Recent-activity timeline, richer empty states beyond what's already in place (Phase 6).
- Breadcrumbs (deferred — PRD itself notes this is "useful once repositories/workspaces are added," which they aren't).
