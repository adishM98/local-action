package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeStub(t *testing.T, dir, name, script string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestEngine_SuccessfulRun(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-success.sh", "#!/bin/sh\necho \"ran with: $@\"\necho \"line two\" 1>&2\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var mu sync.Mutex
	var lines []string
	engine := NewEngine(db, make([]byte, keySize), stub, func(runID int64, line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, line)
	}, nil)

	runID, err := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForStatus(t, db, runID, StatusSuccess)

	mu.Lock()
	defer mu.Unlock()
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 streamed lines, got %v", lines)
	}
}

// TestEngine_OnFinishCalledOnceAfterTerminalStatus guards the wiring used to
// free the WebSocket hub's per-run buffer (Hub.Forget): onFinish must fire
// exactly once per run, and only after the run's status is already terminal
// in the DB, so a client can't be caught mid-replay.
func TestEngine_OnFinishCalledOnceAfterTerminalStatus(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-success.sh", "#!/bin/sh\necho hi\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var mu sync.Mutex
	var finishedIDs []int64
	var statusAtFinish RunStatus
	engine := NewEngine(db, make([]byte, keySize), stub, nil, func(runID int64) {
		mu.Lock()
		defer mu.Unlock()
		finishedIDs = append(finishedIDs, runID)
		run, _ := GetRun(db, runID)
		statusAtFinish = run.Status
	})

	runID, err := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForStatus(t, db, runID, StatusSuccess)
	time.Sleep(50 * time.Millisecond) // let onFinish run after the status update

	mu.Lock()
	defer mu.Unlock()
	if len(finishedIDs) != 1 || finishedIDs[0] != runID {
		t.Fatalf("expected onFinish called exactly once for run %d, got %v", runID, finishedIDs)
	}
	if statusAtFinish != StatusSuccess {
		t.Fatalf("expected run status to already be terminal (success) when onFinish fires, got %q", statusAtFinish)
	}
}

func TestEngine_FailingRun(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-fail.sh", "#!/bin/sh\necho \"about to fail\"\nexit 1\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil, nil)
	runID, err := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForStatus(t, db, runID, StatusFailed)
}

func TestEngine_RunsQueueSequentially(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-slow.sh", "#!/bin/sh\nsleep 0.3\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil, nil)

	id1, _ := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})
	id2, _ := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})

	waitForStatus(t, db, id1, StatusSuccess)
	waitForStatus(t, db, id2, StatusSuccess)

	run1, _ := GetRun(db, id1)
	run2, _ := GetRun(db, id2)
	if run2.StartedAt.Int64 < run1.FinishedAt.Int64 {
		t.Fatalf("expected run2 to start after run1 finished (sequential queue); run1=%+v run2=%+v", run1, run2)
	}
}

func TestEngine_Cancel(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-longsleep.sh", "#!/bin/sh\nsleep 5\nexit 0\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil, nil)
	runID, _ := engine.Enqueue(RunRequest{RepoPath: dir, WorkflowFile: "wf.yml", Event: "push"})

	waitForStatus(t, db, runID, StatusRunning)

	if !engine.Cancel(runID) {
		t.Fatal("expected Cancel to return true for a running run")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, _ := GetRun(db, runID)
		if run.Status == StatusCancelled || run.Status == StatusFailed {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected run to be cancelled/failed within 2s of Cancel")
}

func TestWriteDotenvTemp_CleansUpTempFileOnError(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	key := make([]byte, keySize)
	repoPath := dir

	// A valid secret that sorts before the corrupted one, so the write loop
	// gets partway through (writing plaintext to the temp file) before
	// GetSecretValue fails on the second entry.
	if err := UpsertSecret(db, key, repoPath, KindSecret, "AAA_GOOD", "good-value"); err != nil {
		t.Fatalf("upsert good secret: %v", err)
	}
	// Insert a secret with corrupted ciphertext directly via the DB so that
	// GetSecretValue's Decrypt call fails mid-loop.
	if _, err := db.Exec(
		`INSERT INTO secrets (repo_path, kind, key, value_encrypted) VALUES (?, ?, ?, ?)`,
		repoPath, string(KindSecret), "BBB_BAD", []byte("short"),
	); err != nil {
		t.Fatalf("insert corrupted secret: %v", err)
	}

	entries, err := ListSecrets(db, repoPath, KindSecret)
	if err != nil {
		t.Fatalf("list secrets: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 secret entries, got %d", len(entries))
	}

	pattern := filepath.Join(os.TempDir(), "act-secrets-leak-test-*.env")
	before, _ := filepath.Glob(pattern)

	e := &Engine{db: db, key: key}
	name, err := e.writeDotenvTemp("act-secrets-leak-test-*.env", repoPath, entries, nil)
	if err == nil {
		os.Remove(name)
		t.Fatal("expected writeDotenvTemp to fail for corrupted secret value")
	}

	after, _ := filepath.Glob(pattern)
	if len(after) > len(before) {
		t.Fatalf("expected no leaked temp file after error, before=%v after=%v", before, after)
	}
}

func waitForStatus(t *testing.T, db *sql.DB, runID int64, want RunStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := GetRun(db, runID)
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
