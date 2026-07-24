# Auto-detect secrets/vars from workflow file — design spec

Date: 2026-07-24
Status: approved pending user review

## Goal

Reading a workflow's YAML by hand to find which `secrets.X` / `vars.X` it references, then typing each name into the Secrets page, is tedious. Scan the workflow file for these references and let the user one-click prefill the "New secret/variable" form instead of typing names from memory.

## 1. Backend detection

`WorkflowInfo` (`internal/app/scanner.go`) gains two fields:

```go
UsedSecrets []string `json:"usedSecrets,omitempty"`
UsedVars    []string `json:"usedVars,omitempty"`
```

`ParseWorkflowFile` runs two regexes over the raw file bytes it already has in hand (no second file read):

```go
var secretRefRe = regexp.MustCompile(`\$\{\{\s*secrets\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
var varRefRe    = regexp.MustCompile(`\$\{\{\s*vars\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
```

Matches are deduped and sorted. `GITHUB_TOKEN` is excluded from `UsedSecrets` — it's act/GitHub's automatically-supplied token, not something a user stores here.

A plain regex sweep (not a YAML-node walk) because GitHub Actions allows these references almost anywhere a string can appear — `env:`, `with:`, `if:`, inside `run:` script bodies, container `credentials:` — and modeling every legal location is unnecessary when the raw text already contains the exact token needed.

Malformed or unusual syntax (`secrets['NAME']` bracket form, missing `$`) simply doesn't match — no error, nothing detected for that reference. This is a convenience feature, not a validator.

## 2. Frontend: quick-add chips

`SecretsPage` (`cmd/local-action/web/src/components/SecretsPage.jsx`), when its "Filter by workflow" dropdown has a specific workflow selected (not "All"):

- Above the "New secret/variable" form, show a row: **"Detected in this workflow — click to add:"** followed by one chip per name in `workflow.usedSecrets` (Secrets tab) or `workflow.usedVars` (Variables tab).
- A name is omitted from the chip row once it's already stored for that workflow — either repo-wide (`workflowFile: ""`) or scoped to this exact workflow file — since adding it again would just be an update, not a "missing" item.
- Clicking a chip: sets the Name field to that value, sets Scope to "Specific workflow" = the currently filtered workflow, and focuses the Value field so the user only has to type the secret's value.
- No chip row when the filter is "All workflows", or when every detected name for the selected workflow is already stored.

No change to `RunWorkflowMenu`'s existing "N secrets, M vars will be injected" count — that already reflects what's actually stored, independent of this detection feature.

## 3. Edge cases / testing

- Regex runs on every scanned workflow file on every `/api/scan` call — negligible cost at the file sizes involved (workflow YAML files, not logs).
- Go tests: detects `secrets.X`/`vars.X` in various positions (env, run body, with), excludes `GITHUB_TOKEN`, dedups repeats, ignores non-matching bracket syntax, empty result for a workflow with no references.
- No new JS tests: the chip click is a trivial form-prefill (three `setState` calls), not new pure logic worth a `node:test`.

## Out of scope

- "All workflows" union scan (chips only appear per-selected-workflow, per user's choice above).
- Validating/flagging secrets referenced but never runnable (that's what the run dropdown's injection count already surfaces at run time).
- Detecting `env` context variables or non-secrets/vars expressions.
