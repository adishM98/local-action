package app

import (
	"database/sql"
)

type RunStatus string

const (
	StatusQueued    RunStatus = "queued"
	StatusRunning   RunStatus = "running"
	StatusSuccess   RunStatus = "success"
	StatusFailed    RunStatus = "failed"
	StatusCancelled RunStatus = "cancelled"
)

type Run struct {
	ID           int64         `json:"id"`
	RepoPath     string        `json:"repoPath"`
	WorkflowFile string        `json:"workflowFile"`
	Event        string        `json:"event"`
	Inputs       string        `json:"inputs"`
	Status       RunStatus     `json:"status"`
	StartedAt    sql.NullInt64 `json:"startedAt"`
	FinishedAt   sql.NullInt64 `json:"finishedAt"`
	CreatedAt    int64         `json:"createdAt"`
	Branch       string        `json:"branch"`
	CommitSHA    string        `json:"commitSha"`
}

func CreateRun(db *sql.DB, r Run) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO runs (repo_path, workflow_file, event, inputs, status, created_at, branch, commit_sha) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RepoPath, r.WorkflowFile, r.Event, r.Inputs, string(r.Status), r.CreatedAt, r.Branch, r.CommitSHA,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdateRunStatus(db *sql.DB, id int64, status RunStatus, startedAt, finishedAt *int64) error {
	_, err := db.Exec(
		`UPDATE runs SET status = ?, started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at) WHERE id = ?`,
		string(status), startedAt, finishedAt, id,
	)
	return err
}

func AppendRunLog(db *sql.DB, runID int64, lineNo int, text string) error {
	_, err := db.Exec(
		`INSERT INTO run_logs (run_id, line_no, text) VALUES (?, ?, ?)`,
		runID, lineNo, text,
	)
	return err
}

func GetRunLogs(db *sql.DB, runID int64) ([]string, error) {
	rows, err := db.Query(`SELECT text FROM run_logs WHERE run_id = ? ORDER BY line_no`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []string{}
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		lines = append(lines, text)
	}
	return lines, rows.Err()
}

func GetRun(db *sql.DB, id int64) (Run, error) {
	var r Run
	var status string
	err := db.QueryRow(
		`SELECT id, repo_path, workflow_file, event, inputs, status, started_at, finished_at, created_at, branch, commit_sha FROM runs WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.RepoPath, &r.WorkflowFile, &r.Event, &r.Inputs, &status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt, &r.Branch, &r.CommitSHA)
	r.Status = RunStatus(status)
	return r, err
}

func ListRuns(db *sql.DB, repoPath string) ([]Run, error) {
	rows, err := db.Query(
		`SELECT id, repo_path, workflow_file, event, inputs, status, started_at, finished_at, created_at, branch, commit_sha FROM runs WHERE repo_path = ? ORDER BY created_at DESC`,
		repoPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []Run{}
	for rows.Next() {
		var r Run
		var status string
		if err := rows.Scan(&r.ID, &r.RepoPath, &r.WorkflowFile, &r.Event, &r.Inputs, &status, &r.StartedAt, &r.FinishedAt, &r.CreatedAt, &r.Branch, &r.CommitSHA); err != nil {
			return nil, err
		}
		r.Status = RunStatus(status)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
