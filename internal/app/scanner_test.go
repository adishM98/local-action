package app

import (
	"os"
	"path/filepath"
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
