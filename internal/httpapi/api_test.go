package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"local-action/internal/db"
	"local-action/internal/runs"
	"local-action/internal/secrets"
	"local-action/internal/workflows"
	"local-action/internal/ws"
)

func newTestRouter(t *testing.T, actStub string) (*http.ServeMux, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := db.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	key := make([]byte, secrets.KeySize)
	hub := ws.NewHub()
	engine := runs.NewEngine(db, key, actStub, hub.Broadcast, hub.Forget)
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
	var workflows []workflows.WorkflowInfo
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
	var entries []secrets.SecretEntry
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

func TestAPI_RepoInfo_ReturnsBranchAndCommit(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "feature/repo-info")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "initial")

	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/repo-info?repoPath=" + repoDir)
	if err != nil {
		t.Fatalf("get repo-info: %v", err)
	}
	defer resp.Body.Close()
	var got struct {
		Branch    string `json:"branch"`
		CommitSha string `json:"commitSha"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Branch != "feature/repo-info" {
		t.Fatalf("branch: got %q, want feature/repo-info", got.Branch)
	}
	if len(got.CommitSha) < 7 {
		t.Fatalf("commitSha: got %q, expected a short hash", got.CommitSha)
	}
}

func TestAPI_RepoInfo_NotAGitRepo(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/repo-info?repoPath=" + t.TempDir())
	if err != nil {
		t.Fatalf("get repo-info: %v", err)
	}
	defer resp.Body.Close()
	var got struct {
		Branch    string `json:"branch"`
		CommitSha string `json:"commitSha"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Branch != "" || got.CommitSha != "" {
		t.Fatalf("expected empty branch/commitSha for a non-git dir, got %+v", got)
	}
}

func TestAPI_GetRun_NotFound(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Try to get a run that doesn't exist
	resp, err := http.Get(server.URL + "/api/runs/999")
	if err != nil {
		t.Fatalf("get non-existent run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %v", resp.StatusCode)
	}
}

func itoa(n int64) string {
	return string(rune('0' + n%10)) // fine for single-digit run IDs in this small test DB
}

func TestAPI_WorkflowScopedSecrets(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	post := func(payload map[string]string) {
		t.Helper()
		body, _ := json.Marshal(payload)
		resp, err := http.Post(server.URL+"/api/secrets", "application/json", bytes.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("upsert %v: err=%v status=%v", payload, err, resp.StatusCode)
		}
	}
	post(map[string]string{"repoPath": "/r", "kind": "secret", "key": "FOO", "value": "repo"})
	post(map[string]string{"repoPath": "/r", "kind": "secret", "key": "FOO", "value": "ci", "workflowFile": "ci.yml"})

	resp, err := http.Get(server.URL + "/api/secrets?repoPath=/r&kind=secret")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var entries []secrets.SecretEntry
	json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (repo-wide + workflow), got %+v", entries)
	}
	if entries[0].WorkflowFile != "" || entries[1].WorkflowFile != "ci.yml" {
		t.Fatalf("expected ordering repo-wide then ci.yml, got %+v", entries)
	}

	// Delete only the workflow-scoped one.
	delBody, _ := json.Marshal(map[string]string{"repoPath": "/r", "kind": "secret", "key": "FOO", "workflowFile": "ci.yml"})
	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/secrets", bytes.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: err=%v status=%v", err, resp.StatusCode)
	}
	resp, _ = http.Get(server.URL + "/api/secrets?repoPath=/r&kind=secret")
	entries = nil
	json.NewDecoder(resp.Body).Decode(&entries)
	resp.Body.Close()
	if len(entries) != 1 || entries[0].WorkflowFile != "" {
		t.Fatalf("expected only the repo-wide entry to remain, got %+v", entries)
	}
}

func TestAPI_HealthDockerErrorFromStderr(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\necho act version stub\n"), 0755); err != nil {
		t.Fatalf("write act stub: %v", err)
	}
	// A fake docker on PATH that prints noise to stdout and the real error
	// to stderr, then fails — mimics `docker info` with a dead daemon.
	if err := os.WriteFile(filepath.Join(stubDir, "docker"), []byte(
		"#!/bin/sh\necho 'Client: noise that must not appear'\necho 'Cannot connect to the Docker daemon' >&2\nexit 1\n",
	), 0755); err != nil {
		t.Fatalf("write docker stub: %v", err)
	}
	t.Setenv("PATH", stubDir)

	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["dockerOK"] != false {
		t.Fatalf("expected dockerOK false, got %v", body["dockerOK"])
	}
	got, _ := body["dockerError"].(string)
	if !strings.Contains(got, "Cannot connect to the Docker daemon") {
		t.Fatalf("expected stderr error in dockerError, got %q", got)
	}
	if strings.Contains(got, "noise") {
		t.Fatalf("stdout noise leaked into dockerError: %q", got)
	}
}

func TestAPI_EventPayload_SaveGetAndRejectMalformed(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	// No payload saved yet.
	resp, err := http.Get(server.URL + "/api/event-payload?repoPath=/r&workflowFile=ci.yml")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var got struct {
		Payload string `json:"payload"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Payload != "" {
		t.Fatalf("expected empty payload, got %q", got.Payload)
	}

	// Save a valid payload.
	payload := `{"action":"labeled","label":{"name":"run-ci"}}`
	body, _ := json.Marshal(map[string]string{"repoPath": "/r", "workflowFile": "ci.yml", "payload": payload})
	resp, err = http.Post(server.URL+"/api/event-payload", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("save: err=%v status=%v", err, resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/api/event-payload?repoPath=/r&workflowFile=ci.yml")
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	got = struct {
		Payload string `json:"payload"`
	}{}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Payload != payload {
		t.Fatalf("got %q, want %q", got.Payload, payload)
	}

	// Malformed JSON is rejected with 400, doesn't overwrite the saved value.
	badBody, _ := json.Marshal(map[string]string{"repoPath": "/r", "workflowFile": "ci.yml", "payload": "{not json"})
	resp, err = http.Post(server.URL+"/api/event-payload", "application/json", bytes.NewReader(badBody))
	if err != nil {
		t.Fatalf("save malformed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %v", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(server.URL + "/api/event-payload?repoPath=/r&workflowFile=ci.yml")
	got = struct {
		Payload string `json:"payload"`
	}{}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Payload != payload {
		t.Fatalf("expected unchanged payload after rejected save, got %q", got.Payload)
	}
}

func TestAPI_CreateRun_RejectsMalformedEventPayload(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	body, _ := json.Marshal(map[string]any{
		"repoPath": "/r", "workflowFile": "ci.yml", "event": "push", "eventPayload": "{not json",
	})
	resp, err := http.Post(server.URL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed eventPayload, got %v", resp.StatusCode)
	}
}

func TestAPI_CreateRun_ReachesActWithEventPayload(t *testing.T) {
	repoDir := t.TempDir()
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	// Echo the -e file's contents so we can confirm the payload actually
	// reaches the act invocation, not just gets stored.
	script := "#!/bin/sh\nprev=\"\"\nfor arg in \"$@\"; do\n  if [ \"$prev\" = \"-e\" ]; then cat \"$arg\"; fi\n  prev=\"$arg\"\ndone\nexit 0\n"
	if err := os.WriteFile(stubPath, []byte(script), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, db, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	payload := `{"action":"labeled"}`
	body, _ := json.Marshal(map[string]any{
		"repoPath": repoDir, "workflowFile": "ci.yml", "event": "workflow_dispatch", "eventPayload": payload,
	})
	resp, err := http.Post(server.URL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	var created struct {
		RunID int64 `json:"runId"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	waitForStatus(t, db, created.RunID, runs.StatusSuccess)
	logs, err := runs.GetRunLogs(db, created.RunID)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(logs) == 0 || logs[0] != payload {
		t.Fatalf("expected act to receive the event payload verbatim, got %v", logs)
	}
}

func TestAPI_WorkflowCategories_SaveAndGet(t *testing.T) {
	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, _, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/workflow-categories?repoPath=/r")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var got map[string]string
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}

	body, _ := json.Marshal(map[string]string{"repoPath": "/r", "workflowFile": "ci.yml", "category": "Testing"})
	resp, err = http.Post(server.URL+"/api/workflow-categories", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("save: err=%v status=%v", err, resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/api/workflow-categories?repoPath=/r")
	if err != nil {
		t.Fatalf("get after save: %v", err)
	}
	got = nil
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["ci.yml"] != "Testing" {
		t.Fatalf("got %v, want ci.yml=Testing", got)
	}
}

func TestAPI_CreateRun_CapturesBranchAndCommitFromRealGitRepo(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "feature/foo")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "initial")
	if err := os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}

	stubDir := t.TempDir()
	stubPath := filepath.Join(stubDir, "fake-act.sh")
	if err := os.WriteFile(stubPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	mux, db, _ := newTestRouter(t, stubPath)
	server := httptest.NewServer(mux)
	defer server.Close()

	body, _ := json.Marshal(map[string]string{"repoPath": repoDir, "workflowFile": "ci.yml", "event": "push"})
	resp, err := http.Post(server.URL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	var created struct {
		RunID int64 `json:"runId"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	waitForStatus(t, db, created.RunID, runs.StatusSuccess)
	run, err := runs.GetRun(db, created.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Branch != "feature/foo" {
		t.Errorf("branch: got %q, want feature/foo", run.Branch)
	}
	if len(run.CommitSHA) < 7 {
		t.Errorf("commitSha: got %q, expected a short hash", run.CommitSHA)
	}
}

func TestTruncateTail(t *testing.T) {
	if got := truncateTail("short", 300); got != "short" {
		t.Fatalf("short string changed: %q", got)
	}
	long := strings.Repeat("x", 400) + "tail-end"
	got := truncateTail(long, 300)
	if len(got) > 300 || !strings.HasSuffix(got, "tail-end") {
		t.Fatalf("tail not kept: len=%d %q", len(got), got[:20])
	}
	// multi-byte boundary: é is 2 bytes; ensure result is valid UTF-8
	multi := strings.Repeat("é", 200) // 400 bytes
	got = truncateTail(multi, 301)    // 301 would split a rune without the boundary scan
	if !utf8.ValidString(got) {
		t.Fatal("truncateTail produced invalid UTF-8")
	}
}

func waitForStatus(t *testing.T, db *sql.DB, runID int64, want runs.RunStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := runs.GetRun(db, runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if run.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %d did not reach status %q within timeout", runID, want)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
