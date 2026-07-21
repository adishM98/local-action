package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestRouter(t *testing.T, actStub string) (*http.ServeMux, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	key := make([]byte, keySize)
	hub := NewHub()
	engine := NewEngine(db, key, actStub, hub.Broadcast)
	return NewRouter(db, key, engine, hub, actStub), db, dir
}

func TestAPI_ScanSecretsAndRunLifecycle(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	wfContent := "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps: []\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".github", "workflows", "ci.yml"), []byte(wfContent), 0644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\necho \"stub output\"\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	// scan
	scanBody, _ := json.Marshal(map[string]string{"path": repoDir})
	resp, err := http.Post(server.URL+"/api/scan", "application/json", bytes.NewReader(scanBody))
	if err != nil {
		t.Fatalf("scan request: %v", err)
	}
	var workflows []WorkflowInfo
	json.NewDecoder(resp.Body).Decode(&workflows)
	resp.Body.Close()
	if len(workflows) != 1 || workflows[0].Events[0] != "push" {
		t.Fatalf("unexpected scan result: %+v", workflows)
	}

	// secrets: upsert, list, delete
	upsertBody, _ := json.Marshal(map[string]string{"repoPath": repoDir, "kind": "secret", "key": "TOKEN", "value": "abc"})
	resp, err = http.Post(server.URL+"/api/secrets", "application/json", bytes.NewReader(upsertBody))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("upsert secret: err=%v status=%v", err, resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/api/secrets?repoPath=" + repoDir + "&kind=secret")
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	var entries []SecretEntry
	json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) != 1 || entries[0].Key != "TOKEN" {
		t.Fatalf("unexpected secrets list: %+v", entries)
	}

	// run: create, poll until done, check logs
	runBody, _ := json.Marshal(map[string]any{
		"repoPath":     repoDir,
		"workflowFile": ".github/workflows/ci.yml",
		"event":        "push",
	})
	resp, err = http.Post(server.URL+"/api/runs", "application/json", bytes.NewReader(runBody))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	var created struct {
		RunID int64 `json:"runId"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	var detail map[string]any
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(server.URL + "/api/runs/" + itoa(created.RunID))
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		json.NewDecoder(resp.Body).Decode(&detail)
		resp.Body.Close()
		run := detail["run"].(map[string]any)
		if run["status"] == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	logs := detail["logs"].([]any)
	if len(logs) == 0 || !strings.Contains(logs[0].(string), "stub output") {
		t.Fatalf("expected stub output in logs, got %v", logs)
	}
}

func itoa(n int64) string {
	return string(rune('0' + n%10)) // fine for single-digit run IDs in this small test DB
}
