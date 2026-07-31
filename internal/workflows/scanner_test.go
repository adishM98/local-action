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
