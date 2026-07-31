package workflows

import "database/sql"

// GetWorkflowCategories returns every manually-overridden sidebar category
// for a repo, keyed by workflow file. A workflow with no entry here uses
// its auto-detected category instead (WorkflowInfo.AutoCategory). Fetched
// as one map per repo since the sidebar needs every visible workflow's
// override at once, not one round trip per workflow.
func GetWorkflowCategories(db *sql.DB, repoPath string) (map[string]string, error) {
	rows, err := db.Query(
		`SELECT workflow_file, category FROM workflow_categories WHERE repo_path = ?`,
		repoPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := map[string]string{}
	for rows.Next() {
		var file, category string
		if err := rows.Scan(&file, &category); err != nil {
			return nil, err
		}
		categories[file] = category
	}
	return categories, rows.Err()
}

// SaveWorkflowCategory upserts a workflow's category override. Saving an
// empty string clears it back to "use the auto-detected category" by
// deleting the row.
func SaveWorkflowCategory(db *sql.DB, repoPath, workflowFile, category string) error {
	if category == "" {
		_, err := db.Exec(
			`DELETE FROM workflow_categories WHERE repo_path = ? AND workflow_file = ?`,
			repoPath, workflowFile,
		)
		return err
	}
	_, err := db.Exec(
		`INSERT INTO workflow_categories (repo_path, workflow_file, category) VALUES (?, ?, ?)
		 ON CONFLICT(repo_path, workflow_file) DO UPDATE SET category = excluded.category`,
		repoPath, workflowFile, category,
	)
	return err
}
