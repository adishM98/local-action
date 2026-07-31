package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"local-action/internal/db"
)

func TestRuns_CreateGetUpdateList(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	id, err := CreateRun(db, Run{
		RepoPath:     "/repo/a",
		WorkflowFile: ".github/workflows/ci.yml",
		Event:        "push",
		Inputs:       "{}",
		Status:       StatusQueued,
		CreatedAt:    now,
		Branch:       "main",
		CommitSHA:    "abc1234",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	run, err := GetRun(db, id)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != StatusQueued || run.WorkflowFile != ".github/workflows/ci.yml" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if run.Branch != "main" || run.CommitSHA != "abc1234" {
		t.Fatalf("expected branch/commitSha to round-trip, got %+v", run)
	}

	started := time.Now().Unix()
	if err := UpdateRunStatus(db, id, StatusRunning, &started, nil); err != nil {
		t.Fatalf("update to running: %v", err)
	}
	run, _ = GetRun(db, id)
	if run.Status != StatusRunning || !run.StartedAt.Valid {
		t.Fatalf("expected running with started_at set, got %+v", run)
	}

	finished := time.Now().Unix()
	if err := UpdateRunStatus(db, id, StatusSuccess, nil, &finished); err != nil {
		t.Fatalf("update to success: %v", err)
	}
	run, _ = GetRun(db, id)
	if run.Status != StatusSuccess || !run.FinishedAt.Valid || !run.StartedAt.Valid {
		t.Fatalf("expected success with both timestamps set, got %+v", run)
	}

	if err := AppendRunLog(db, id, 1, "first line"); err != nil {
		t.Fatalf("append log 1: %v", err)
	}
	if err := AppendRunLog(db, id, 2, "second line"); err != nil {
		t.Fatalf("append log 2: %v", err)
	}
	logs, err := GetRunLogs(db, id)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(logs) != 2 || logs[0] != "first line" || logs[1] != "second line" {
		t.Fatalf("unexpected logs order: %v", logs)
	}

	id2, _ := CreateRun(db, Run{RepoPath: "/repo/a", WorkflowFile: "x.yml", Event: "push", Inputs: "{}", Status: StatusQueued, CreatedAt: now + 1})
	runs, err := ListRuns(db, "/repo/a")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != id2 {
		t.Fatalf("expected newest-first order with id2 first, got %+v", runs)
	}
}

// TestListRuns_EmptyMarshalsAsEmptyArray guards against ListRuns returning a
// nil slice for a zero-row result: encoding/json marshals nil as `null`,
// which crashes frontend consumers that call .map() on the response
// (HistoryPanel.jsx) for a repo with no run history yet.
func TestListRuns_EmptyMarshalsAsEmptyArray(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	runs, err := ListRuns(db, "/repo/does-not-exist")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no runs, got %+v", runs)
	}
	b, err := json.Marshal(runs)
	if err != nil {
		t.Fatalf("marshal runs: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("expected empty ListRuns to marshal as [], got %s", b)
	}
}

// TestGetRunLogs_EmptyMarshalsAsEmptyArray guards the same nil-slice-to-null
// pitfall for GetRunLogs, used by the log replay/history views.
func TestGetRunLogs_EmptyMarshalsAsEmptyArray(t *testing.T) {
	db, err := db.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	id, err := CreateRun(db, Run{RepoPath: "/repo/a", WorkflowFile: "wf.yml", Event: "push", Inputs: "{}", Status: StatusQueued, CreatedAt: time.Now().Unix()})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	logs, err := GetRunLogs(db, id)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no logs, got %+v", logs)
	}
	b, err := json.Marshal(logs)
	if err != nil {
		t.Fatalf("marshal logs: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("expected empty GetRunLogs to marshal as [], got %s", b)
	}
}
