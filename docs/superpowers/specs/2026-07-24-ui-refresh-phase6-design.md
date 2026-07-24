# UI Refresh Phase 6: recent activity + richer empty states — design spec

Date: 2026-07-24
Status: approved (autonomous)

## Recent activity timeline

- Shown only on the "All workflows" view (a single workflow's own runs list is already a recency-sorted feed — a second one on the same page would be redundant chrome).
- Last 5 runs across the repo (from the already-loaded, unfiltered `runs`), one line each: status icon, `{workflow name} {verb}`, relative time. Verb derived from status: success→succeeded, failed→failed, running→started, cancelled→cancelled, queued→queued.
- Positioned between the stat cards and the toolbar.

## Richer empty states

- Zero runs for the selected workflow: heading + explanatory line pointing at the existing "Run workflow ▾" button (no duplicate CTA button — that would mean lifting `RunWorkflowMenu`'s open-state out of its own component for no real benefit).
- Zero runs across the whole repo (All workflows, freshly scanned): similar, adjusted copy.
- Filtered-to-nothing case (existing "No runs match these filters.") is already reasonably clear — left as is.

## Testing

No new pure logic — this phase is presentational copy/layout only, same judgment as prior UI-only passes.
