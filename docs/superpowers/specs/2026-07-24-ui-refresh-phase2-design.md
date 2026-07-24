# UI Refresh Phase 2: sidebar categorization — design spec

Date: 2026-07-24
Status: approved (autonomous — user authorized proceeding through all remaining phases without per-phase check-in)

## Decisions

- **Categories** (6): 🚀 Deployment, 🧪 Testing, 🔒 Security, ⚙️ CI/Build, 📚 Docs, Other (catch-all, no icon).
- **Categorization**: keyword heuristic over `name + file` (lowercased), checked in priority order so more-specific categories win before generic ones:
  1. Security: `security`, `vulnerab`, `licen`, `compliance`, `grype`, `cve`
  2. Deployment: `deploy`, `publish`, `release`, `render`, `netlify`
  3. Testing: `test`, `cypress`, `coverage`, `e2e`
  4. Docs: `docs`, `storybook`
  5. CI/Build: `build`, `docker`, ` ci`/`^ci`, `packer`, `ami`, `image`
  6. Other: no match
- **Manual override**: new table `workflow_categories (repo_path, workflow_file, category)`, same upsert/clear-on-empty pattern as `event_payloads`. Fetched as one map (`GET /api/workflow-categories?repoPath=`) since the sidebar needs every visible workflow's override at once — not one fetch per workflow.
- **Effective category** = `override[file] || autoCategory || 'Other'`.
- **Collapse/expand** state: localStorage only (`sidebarCollapsed`, keyed by category name) — pure UI preference, not worth a DB round trip.
- **Override UI**: small `<select>` next to each sidebar item, visible on row hover, first option "Auto (guessed)" to clear back to the heuristic.

## Backend

- `WorkflowInfo.AutoCategory string` (always computed in `ParseWorkflowFile`, cheap keyword match — no DB).
- `internal/app/workflowcategory.go`: `GetWorkflowCategories(db, repoPath) (map[string]string, error)`, `SaveWorkflowCategory(db, repoPath, workflowFile, category string) error`.
- `GET /api/workflow-categories?repoPath=`, `POST /api/workflow-categories` (`{repoPath, workflowFile, category}`).

## Frontend

- `Sidebar.jsx` groups `workflows` by effective category, renders one collapsible section per category that has ≥1 workflow (empty categories don't render), icon + name in the heading, chevron toggles collapse (localStorage-backed).
- Category override `<select>` appears on row hover only — doesn't clutter the default view.

## Testing

- Go: `autoCategoryFor(name, file string) string` unit tests covering the priority order against representative real names (e.g. "Grype - Docker Image Vulnerability" → Security despite containing "docker"; "Manual Docker Build and Push" → CI/Build; "Deploy Storybook to Netlify" → Deployment despite containing "storybook"). `workflow_categories` CRUD round-trip + clear-on-empty, mirroring `eventpayload_test.go`.
- No new JS test — sidebar grouping/collapse is presentational, same judgment as prior UI-only changes this session.
