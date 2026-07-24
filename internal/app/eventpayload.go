package app

import "database/sql"

// GetEventPayload returns the saved event JSON payload for a workflow, or ""
// if none is saved. Used to fill act's -e/--eventpath so github.event.*
// context is populated for locally-triggered runs.
func GetEventPayload(db *sql.DB, repoPath, workflowFile string) (string, error) {
	var payload string
	err := db.QueryRow(
		`SELECT payload FROM event_payloads WHERE repo_path = ? AND workflow_file = ?`,
		repoPath, workflowFile,
	).Scan(&payload)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return payload, nil
}

// SaveEventPayload upserts the payload for a workflow. Saving an empty
// string clears it back to "no payload" by deleting the row.
func SaveEventPayload(db *sql.DB, repoPath, workflowFile, payload string) error {
	if payload == "" {
		_, err := db.Exec(
			`DELETE FROM event_payloads WHERE repo_path = ? AND workflow_file = ?`,
			repoPath, workflowFile,
		)
		return err
	}
	_, err := db.Exec(
		`INSERT INTO event_payloads (repo_path, workflow_file, payload) VALUES (?, ?, ?)
		 ON CONFLICT(repo_path, workflow_file) DO UPDATE SET payload = excluded.payload`,
		repoPath, workflowFile, payload,
	)
	return err
}
