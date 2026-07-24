# UI Refresh Phase 4: branch/commit capture + pagination — design spec

Date: 2026-07-24
Status: approved (autonomous)

## Branch/commit capture

- `runs` table gains `branch TEXT NOT NULL DEFAULT ''`, `commit_sha TEXT NOT NULL DEFAULT ''` — plain `ALTER TABLE ADD COLUMN` (unlike the earlier `secrets` migration, these aren't part of a primary key, so no table rebuild needed). Idempotent, same `pragma_table_info` guard pattern.
- `internal/app/gitinfo.go`: `gitInfo(repoPath string) (branch, sha string)` shells `git -C repoPath rev-parse --abbrev-ref HEAD` and `git -C repoPath rev-parse --short HEAD`, each with a 2s timeout. Any error (not a git repo, detached HEAD edge cases, git not installed) → both return `""`, never fails the run.
- `Engine.Enqueue` calls `gitInfo(req.RepoPath)` and stores the result on the `Run` row at creation time — captured once, not re-checked as the run progresses (the local checkout could change mid-run, but "what branch/commit was checked out when I clicked Run" is the meaningful answer here, same as GitHub Actions itself).
- `Run.Branch`, `Run.CommitSHA` exposed in the JSON API (`branch`, `commitSha`), included in `ListRuns`/`GetRun`.

## Pagination

Client-side only — deliberately not a backend feature. Stat cards (phase 1) must reflect *all* runs for the selected workflow; a server-paginated list would either break that guarantee or require a second unpaginated query for stats alone. At this app's realistic scale (local dev, one repo, dozens–low-hundreds of runs), loading the full set and paginating the rendered rows client-side is simpler and avoids that split. Revisit only if real usage shows this doesn't hold.

- `RunsView`: 25 rows per page, applied to `filtered` (post search/status/event/branch filters) — matches the "Showing X of Y" + Previous/Next pattern from the PRD.
- Page resets to 1 whenever `workflowFile`, search, or any filter changes.

## Branch filter + display

- Toolbar (phase 3) gains a fourth dropdown: Branch, populated from distinct branches in the current (pre-filter) run set — same pattern as the Event dropdown.
- Run rows show branch on the metadata line: `event • branch • relativeTime` (branch omitted from the line entirely when empty, e.g. repoPath isn't a git repo).
- Run detail header meta gains ` • branch` similarly; short commit SHA shown next to it when present.

## Testing

- Go: `gitInfo` against a real temp git repo (init + commit) returns correct branch/short-sha; against a non-repo directory returns `("", "")` with no error. Migration test mirrors `TestOpenDB_MigratesOldSecretsSchema`'s shape (seed pre-migration schema, open, verify columns + idempotent re-open). `CreateRun`/`GetRun`/`ListRuns` round-trip `Branch`/`CommitSHA`. One `api_test.go` case creating a run against a real git-initialized temp repo and asserting the persisted run has non-empty branch/commit.
- No new JS test — pagination slicing is a direct array `.slice()`, not complex enough to warrant one; the branch/event filter reuses the already-tested `filterRuns` shape (extended, not new logic).
