# UI Refresh Phase 5: run detail as a drawer + step progress — design spec

Date: 2026-07-24
Status: approved (autonomous)

## Run detail as a side drawer

- `App.jsx` gains a `drawerRunId` state, independent of `view` (which stays `runs`/`secrets` only — `run` is retired). Clicking a run row, or a run being started (`RunWorkflowMenu`'s `onStarted`), sets `drawerRunId` instead of changing `view` — the runs list stays mounted underneath, preserving scroll position and filter state, which is the actual UX win of a drawer over full navigation.
- New `Drawer.jsx`: fixed-position overlay, backdrop (click to close) + right-anchored panel (~680px, full height, slide-in transition). Closes on backdrop click or Escape key.
- `RunDetail`'s "← All runs" link becomes a "✕ Close" button calling `onClose` (clears `drawerRunId`); internals (WS streaming, polling, job/step cards, cancel/re-run) are unchanged — only the chrome around it changes.
- Re-run from inside the drawer swaps `drawerRunId` to the new run's id (stays open) rather than navigating.
- Drawer is a global overlay: if open while the user switches to Secrets, it stays open on top (simplest mental model — an explicit close is always required, matching how modals/drawers behave elsewhere).

## Live step-progress indicator

- No true "total step count" is available (we only learn about a step when its start line streams in — we don't have the workflow's own step list precounted). Rather than fabricate a total, `JobCard` shows **steps observed so far**: `Step {completed}/{seen} running` (a slim progress bar too), computed from `job.steps` — visible only while the job is unresolved and the run is still `running`. This is honest about what we actually know, not a claimed true total.

## Testing

- No new pure-JS logic worth a `node:test` — the progress numbers are a direct `.filter().length` over already-parsed `job.steps`, same judgment as prior presentational-only changes.
- Manual verification: trigger a live run, confirm the drawer opens automatically, closes on Escape/backdrop, and the step counter increments as steps complete.
