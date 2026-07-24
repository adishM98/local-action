# UI Refresh Phase 3: search + filters — design spec

Date: 2026-07-24
Status: approved (autonomous)

## Scope

- **Runs list toolbar** (`RunsView`): search box (matches workflow name, event, run id, status text) + Status dropdown (All/Passed/Failed/Running/Queued/Cancelled) + Event dropdown (populated from distinct events actually present in the current run set). All client-side over the already-loaded `runs` array — no new API.
- **Sidebar quick search**: text input above the category groups, filters workflow nav items by name substring; groups with zero remaining matches don't render.
- **Deferred**: Branch filter (no branch data until Phase 4), Date filter (not core enough to justify scope here — revisit if asked).

## Implementation

- Pure function `filterRuns(runs, { search, status, event }, resolveName)` in `format.js`, `node:test` covered: empty filters → passthrough; search matches name/event/id/status case-insensitively; status/event filters combine with search (AND, not OR).
- `RunsView.jsx`: three new state values, filter toolbar above the stat cards, `visible` list passes through `filterRuns` after the existing workflow-scope filter.
- `Sidebar.jsx`: one new state value + a plain-JS `.filter()` on `workflows` before grouping (no new pure function needed — this one's a one-line substring check with no edge cases worth a table-driven test, unlike the multi-field runs case).
