package workflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeWorkflow(t *testing.T, repoPath, filename, content string) {
	t.Helper()
	dir := filepath.Join(repoPath, ".github", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
}

func TestParseWorkflowFile_DetectsSecretsAndVarsAnywhereInFile(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "deploy.yml", `name: Deploy
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      REGION: ${{ vars.AWS_REGION }}
    steps:
      - run: |
          curl -H "Authorization: ${{ secrets.API_TOKEN }}"
      - uses: some/action@v1
        with:
          password: ${{ secrets.REGISTRY_PW }}
      - run: echo ${{ secrets.API_TOKEN }}
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	wantSecrets := []string{"API_TOKEN", "REGISTRY_PW"}
	if !reflect.DeepEqual(wf.UsedSecrets, wantSecrets) {
		t.Errorf("UsedSecrets: got %v, want %v (deduped+sorted)", wf.UsedSecrets, wantSecrets)
	}
	wantVars := []string{"AWS_REGION"}
	if !reflect.DeepEqual(wf.UsedVars, wantVars) {
		t.Errorf("UsedVars: got %v, want %v", wf.UsedVars, wantVars)
	}
}

func TestParseWorkflowFile_ExcludesGithubToken(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
      - run: echo ${{ secrets.NPM_TOKEN }}
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if !reflect.DeepEqual(wf.UsedSecrets, []string{"NPM_TOKEN"}) {
		t.Errorf("expected GITHUB_TOKEN excluded, got %v", wf.UsedSecrets)
	}
}

func TestParseWorkflowFile_NoReferencesYieldsEmptySlice(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if len(wf.UsedSecrets) != 0 || len(wf.UsedVars) != 0 {
		t.Errorf("expected no detected secrets/vars, got secrets=%v vars=%v", wf.UsedSecrets, wf.UsedVars)
	}
	b, err := json.Marshal(wf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"usedSecrets"`) || strings.Contains(string(b), `"usedVars"`) {
		t.Errorf("expected omitempty to drop empty usedSecrets/usedVars, got %s", b)
	}
}

func TestParseWorkflowFile_IgnoresMalformedReferences(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ secrets['BRACKET_STYLE'] }}
      - run: echo ${{secrets.NO_SPACE}}
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if !reflect.DeepEqual(wf.UsedSecrets, []string{"NO_SPACE"}) {
		t.Errorf("expected only NO_SPACE (bracket-style skipped), got %v", wf.UsedSecrets)
	}
}

func TestParseWorkflowFile_AutoDetectsEventPayloadFromSimpleIfCondition(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on:
  push:
  pull_request:
  workflow_dispatch:
jobs:
  build:
    if: ${{ github.event.action == 'labeled' && github.event.label.name == 'run-ci' }}
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	want := `{"action":"labeled","label":{"name":"run-ci"}}`
	if wf.AutoEventPayload != want {
		t.Errorf("got %q, want %q", wf.AutoEventPayload, want)
	}
}

func TestParseWorkflowFile_AutoDetectSupportsDoubleQuotedLiterals(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    if: ${{ github.event.action == "opened" }}
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := `{"action":"opened"}`
	if workflows[0].AutoEventPayload != want {
		t.Errorf("got %q, want %q", workflows[0].AutoEventPayload, want)
	}
}

func TestParseWorkflowFile_NoAutoPayloadWhenIfIsComplex(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    if: ${{ github.event.action == 'labeled' || github.event.action == 'synchronize' }}
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if workflows[0].AutoEventPayload != "" {
		t.Errorf("expected no auto payload for an || condition, got %q", workflows[0].AutoEventPayload)
	}
	if !workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload true — the condition references github.event.*, it just couldn't be auto-solved")
	}
}

func TestParseWorkflowFile_NoAutoPayloadWhenNoIfCondition(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if workflows[0].AutoEventPayload != "" {
		t.Errorf("expected no auto payload without an if: condition, got %q", workflows[0].AutoEventPayload)
	}
	if workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload false — there's no if: condition at all, nothing depends on event data")
	}
}

func TestParseWorkflowFile_AutoPayloadSkipsNonEventComparisons(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    if: ${{ github.ref == 'refs/heads/main' }}
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if workflows[0].AutoEventPayload != "" {
		t.Errorf("expected no auto payload for a non github.event.* comparison, got %q", workflows[0].AutoEventPayload)
	}
	if workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload false — the condition doesn't reference github.event.* at all")
	}
}

// TestParseWorkflowFile_NeedsEventPayloadTrueWhenSolved guards the case
// where auto-detection DID succeed: NeedsEventPayload should still be true
// (the workflow genuinely depends on event data), even though the frontend
// won't show a manual field here since AutoEventPayload is already set.
func TestParseWorkflowFile_NeedsEventPayloadTrueWhenSolved(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    if: ${{ github.event.action == 'opened' }}
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload true for a solved github.event.* condition")
	}
	if workflows[0].AutoEventPayload == "" {
		t.Fatal("expected AutoEventPayload to be solved for this simple condition")
	}
}

func TestParseWorkflowFile_AutoPayloadUsesFirstSolvableJob(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  unsolvable:
    if: ${{ github.event.action == 'labeled' || github.event.action == 'x' }}
    runs-on: ubuntu-latest
    steps: []
  solvable:
    if: ${{ github.event.action == 'opened' }}
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := `{"action":"opened"}`
	if workflows[0].AutoEventPayload != want {
		t.Errorf("got %q, want %q", workflows[0].AutoEventPayload, want)
	}
}

// TestParseWorkflowFile_AutoSolvesContainsLabelCheck covers the most common
// real-world "only run when this PR has label X" idiom, which uses
// contains() over the labels.*.name glob rather than a plain equality —
// distinct from (and more common than) the single github.event.label.name
// == 'x' case already covered above.
func TestParseWorkflowFile_AutoSolvesContainsLabelCheck(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: pull_request_target
jobs:
  build:
    if: contains(github.event.pull_request.labels.*.name, 'run-ci')
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := `{"pull_request":{"labels":[{"name":"run-ci"}]}}`
	if workflows[0].AutoEventPayload != want {
		t.Errorf("got %q, want %q", workflows[0].AutoEventPayload, want)
	}
	if !workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload true")
	}
}

// TestParseWorkflowFile_AutoSolvesCombinedActionAndLabelCheck covers the
// contains() label check combined via && with a plain equality clause —
// e.g. gating on both the PR action and a specific label being present.
func TestParseWorkflowFile_AutoSolvesCombinedActionAndLabelCheck(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: pull_request_target
jobs:
  build:
    if: github.event.action == 'labeled' && contains(github.event.pull_request.labels.*.name, 'run-ci')
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want2 := `{"action":"labeled","pull_request":{"labels":[{"name":"run-ci"}]}}`
	if workflows[0].AutoEventPayload != want2 {
		t.Errorf("got %q, want %q", workflows[0].AutoEventPayload, want2)
	}
}

// TestParseWorkflowFile_SuggestsPayloadWhenUnsolvable guards the fallback:
// an || condition can't be solved outright (a partial solve could produce
// a payload that makes it evaluate true for the wrong real event), but its
// individual clauses are still recognizable, so SuggestedEventPayload
// should seed the manual field with something real instead of a generic
// unrelated example.
func TestParseWorkflowFile_SuggestsPayloadWhenUnsolvable(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: pull_request_target
jobs:
  build:
    if: github.event.action == 'labeled' || contains(github.event.pull_request.labels.*.name, 'run-ci')
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if workflows[0].AutoEventPayload != "" {
		t.Errorf("expected no confident auto payload for an || condition, got %q", workflows[0].AutoEventPayload)
	}
	if !workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload true")
	}
	wantSuggested := `{"action":"labeled","pull_request":{"labels":[{"name":"run-ci"}]}}`
	if workflows[0].SuggestedEventPayload != wantSuggested {
		t.Errorf("SuggestedEventPayload: got %q, want %q", workflows[0].SuggestedEventPayload, wantSuggested)
	}
}

// TestParseWorkflowFile_NoSuggestionWhenNothingRecognizable guards against
// suggestEventPayload fabricating a guess when the condition doesn't
// contain any pattern it understands (e.g. a function call it doesn't
// know, over a field it can't parse).
func TestParseWorkflowFile_NoSuggestionWhenNothingRecognizable(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: pull_request_target
jobs:
  build:
    if: startsWith(github.event.pull_request.title, 'release')
    runs-on: ubuntu-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if workflows[0].SuggestedEventPayload != "" {
		t.Errorf("expected no suggestion for an unrecognized function call, got %q", workflows[0].SuggestedEventPayload)
	}
	if !workflows[0].NeedsEventPayload {
		t.Error("expected NeedsEventPayload true — it does reference github.event.*, just not in a pattern we can solve or suggest")
	}
}

func TestAutoCategoryFor_PriorityOrderMatchesRealWorkflowNames(t *testing.T) {
	cases := []struct {
		name, file, want string
	}{
		{"Grype - Docker Image Vulnerability Scan", "grype.yml", "Security"},
		{"License Compliance Check", "license.yml", "Security"},
		{"Manual Docker Build and Push", "docker-build.yml", "CI/Build"},
		{"Deploy Storybook to Netlify", "deploy-storybook.yml", "Deployment"},
		{"Cypress AppBuilder", "cypress-appbuilder.yml", "Testing"},
		{"Update test system (LTS and pre-release)", "update-test.yml", "Testing"},
		{"CI", "ci.yml", "CI/Build"},
		{"Merge Submodule PRs", "merge-submodules.yml", "Other"},
		{"Vulnerability CI", "vuln-ci.yml", "Security"},
		{"AWS AMI build using Packer config", "ami-packer.yml", "CI/Build"},
		{"Render PR deploy Docs", "render-docs.yml", "Deployment"},
	}
	for _, c := range cases {
		got := autoCategoryFor(c.name, c.file)
		if got != c.want {
			t.Errorf("autoCategoryFor(%q, %q) = %q, want %q", c.name, c.file, got, c.want)
		}
	}
}

func TestParseWorkflowFile_SetsAutoCategory(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "deploy.yml", "name: Deploy to Netlify\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if workflows[0].AutoCategory != "Deployment" {
		t.Errorf("got %q, want Deployment", workflows[0].AutoCategory)
	}
}

func TestParseWorkflowFile_UbuntuRunnerNotFlagged(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(workflows[0].IncompatibleRunners) != 0 {
		t.Errorf("expected no incompatible runners, got %v", workflows[0].IncompatibleRunners)
	}
}

func TestParseWorkflowFile_FlagsWindowsRunner(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: windows-latest\n    steps: []\n")
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"windows-latest"}
	if !reflect.DeepEqual(workflows[0].IncompatibleRunners, want) {
		t.Errorf("got %v, want %v", workflows[0].IncompatibleRunners, want)
	}
}

func TestParseWorkflowFile_FlagsMacosRunner(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: macos-14\n    steps: []\n")
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"macos-14"}
	if !reflect.DeepEqual(workflows[0].IncompatibleRunners, want) {
		t.Errorf("got %v, want %v", workflows[0].IncompatibleRunners, want)
	}
}

func TestParseWorkflowFile_FlagsSelfHostedLabelList(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: [self-hosted, linux, x64]\n    steps: []\n")
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"self-hosted"}
	if !reflect.DeepEqual(workflows[0].IncompatibleRunners, want) {
		t.Errorf("got %v, want %v", workflows[0].IncompatibleRunners, want)
	}
}

func TestParseWorkflowFile_FlagsGroupLabelsMapForm(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    runs-on:
      group: my-group
      labels: [self-hosted, windows]
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"self-hosted", "windows"}
	if !reflect.DeepEqual(workflows[0].IncompatibleRunners, want) {
		t.Errorf("got %v, want %v", workflows[0].IncompatibleRunners, want)
	}
}

// TestParseWorkflowFile_MatrixTemplatedRunnerSkipsStaticCheck guards against
// false positives/negatives: a matrix-driven runs-on can't be resolved
// without expanding the matrix (act's job, not ours), so it's simply
// skipped rather than flagged or guessed at.
func TestParseWorkflowFile_MatrixTemplatedRunnerSkipsStaticCheck(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest]
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(workflows[0].IncompatibleRunners) != 0 {
		t.Errorf("expected matrix-templated runs-on to be skipped, got %v", workflows[0].IncompatibleRunners)
	}
}

// TestParseWorkflowFile_DedupesIncompatibleRunnersAcrossJobs guards the
// dedup+sort behavior when multiple jobs share the same incompatible label.
func TestParseWorkflowFile_DedupesIncompatibleRunnersAcrossJobs(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", `name: CI
on: push
jobs:
  a:
    runs-on: windows-latest
    steps: []
  b:
    runs-on: windows-latest
    steps: []
  c:
    runs-on: macos-latest
    steps: []
`)
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"macos-latest", "windows-latest"}
	if !reflect.DeepEqual(workflows[0].IncompatibleRunners, want) {
		t.Errorf("got %v, want %v", workflows[0].IncompatibleRunners, want)
	}
}

func TestScanWorkflows_StringEvent(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(workflows))
	}
	wf := workflows[0]
	if wf.Name != "CI" {
		t.Errorf("expected name CI, got %q", wf.Name)
	}
	if len(wf.Events) != 1 || wf.Events[0] != "push" {
		t.Errorf("expected events [push], got %v", wf.Events)
	}
}

func TestScanWorkflows_ListEvents(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "ci.yml", "name: CI\non: [push, pull_request]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if len(wf.Events) != 2 || wf.Events[0] != "push" || wf.Events[1] != "pull_request" {
		t.Fatalf("expected [push pull_request], got %v", wf.Events)
	}
}

func TestScanWorkflows_MapEventsWithDispatchInputs(t *testing.T) {
	repo := t.TempDir()
	content := `
name: Deploy
on:
  push:
  workflow_dispatch:
    inputs:
      environment:
        description: "Target environment"
        required: true
        default: staging
        type: choice
        options:
          - staging
          - production
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps: []
`
	writeWorkflow(t, repo, "deploy.yml", content)

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	wf := workflows[0]
	if len(wf.Events) != 2 {
		t.Fatalf("expected 2 events, got %v", wf.Events)
	}
	if len(wf.DispatchInputs) != 1 {
		t.Fatalf("expected 1 dispatch input, got %+v", wf.DispatchInputs)
	}
	input := wf.DispatchInputs[0]
	if input.Name != "environment" || !input.Required || input.Default != "staging" || input.Type != "choice" {
		t.Fatalf("unexpected input parsed: %+v", input)
	}
	if len(input.Options) != 2 || input.Options[0] != "staging" || input.Options[1] != "production" {
		t.Fatalf("unexpected options: %v", input.Options)
	}
}

func TestScanWorkflows_InvalidYAML(t *testing.T) {
	repo := t.TempDir()
	writeWorkflow(t, repo, "broken.yml", "name: Broken\non: [push\njobs: {}\n")

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan should not fail outright: %v", err)
	}
	if len(workflows) != 1 || workflows[0].ParseError == "" {
		t.Fatalf("expected a parse error on the broken file, got %+v", workflows)
	}
}

func TestScanWorkflows_NoWorkflowsDir(t *testing.T) {
	repo := t.TempDir()

	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("expected no error for missing workflows dir, got %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("expected no workflows, got %v", workflows)
	}
}

func TestScanWorkflows_PathIsAFileNotDirectory(t *testing.T) {
	repo := t.TempDir()
	filePath := filepath.Join(repo, "ci.yml")
	if err := os.WriteFile(filePath, []byte("on: push\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := ScanWorkflows(filePath)
	if err == nil {
		t.Fatal("expected an error when repoPath points at a file, got nil")
	}
	if !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("expected a clear 'must be a directory' error, got: %v", err)
	}
}

func TestScanWorkflows_PathDoesNotExist(t *testing.T) {
	_, err := ScanWorkflows(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a nonexistent repo path, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected a clear 'does not exist' error, got: %v", err)
	}
}

func TestScanWorkflows_EmptyResultsMarshalAsEmptyArray(t *testing.T) {
	// No workflows dir at all.
	repo := t.TempDir()
	workflows, err := ScanWorkflows(repo)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	b, _ := json.Marshal(workflows)
	if string(b) != "[]" {
		t.Fatalf("no-dir case: expected [], got %s", b)
	}

	// Empty workflows dir.
	repo2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo2, ".github", "workflows"), 0755); err != nil {
		t.Fatal(err)
	}
	workflows, err = ScanWorkflows(repo2)
	if err != nil {
		t.Fatalf("scan empty dir: %v", err)
	}
	b, _ = json.Marshal(workflows)
	if string(b) != "[]" {
		t.Fatalf("empty-dir case: expected [], got %s", b)
	}
}
