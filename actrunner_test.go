package main

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
	})

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

func TestEngine_FailingRun(t *testing.T) {
	dir := t.TempDir()
	stub := writeStub(t, dir, "fake-act-fail.sh", "#!/bin/sh\necho \"about to fail\"\nexit 1\n")

	db, err := OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	engine := NewEngine(db, make([]byte, keySize), stub, nil)
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

	engine := NewEngine(db, make([]byte, keySize), stub, nil)

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

	engine := NewEngine(db, make([]byte, keySize), stub, nil)
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
