package secrets

import (
	"database/sql"
	"errors"
)

type SecretKind string

const (
	KindSecret SecretKind = "secret"
	KindVar    SecretKind = "var"
)

type SecretEntry struct {
	RepoPath     string     `json:"repoPath"`
	Kind         SecretKind `json:"kind"`
	Key          string     `json:"key"`
	WorkflowFile string     `json:"workflowFile"` // "" = repo-wide (all workflows in the repo)
	// Revealable is the per-entry choice made when it was saved: true means
	// GetSecretValue will decrypt and return it (our "edit later" feature);
	// false means it's write-only like a real GitHub secret — the value was
	// encrypted at rest same as any other entry, but GetSecretValue refuses
	// to ever decrypt it again, regardless of who asks. Chosen once at
	// save time, not retroactive: it governs future reads of whatever value
	// is current as of the next save, not the entry's entire history.
	Revealable bool `json:"revealable"`
}

func UpsertSecret(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, value, workflowFile string, revealable bool) error {
	ciphertext, err := Encrypt(encKey, []byte(value))
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO secrets (repo_path, kind, key, workflow_file, value_encrypted, revealable) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_path, kind, key, workflow_file) DO UPDATE SET value_encrypted = excluded.value_encrypted, revealable = excluded.revealable`,
		repoPath, string(kind), name, workflowFile, ciphertext, revealable,
	)
	return err
}

// ListSecrets returns every entry for the repo, both repo-wide and
// workflow-scoped. Callers filter by WorkflowFile as needed.
func ListSecrets(db *sql.DB, repoPath string, kind SecretKind) ([]SecretEntry, error) {
	rows, err := db.Query(
		`SELECT repo_path, kind, key, workflow_file, revealable FROM secrets WHERE repo_path = ? AND kind = ? ORDER BY workflow_file, key`,
		repoPath, string(kind),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []SecretEntry{}
	for rows.Next() {
		var e SecretEntry
		var kindStr string
		if err := rows.Scan(&e.RepoPath, &kindStr, &e.Key, &e.WorkflowFile, &e.Revealable); err != nil {
			return nil, err
		}
		e.Kind = SecretKind(kindStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetSecretValue decrypts and returns an entry's value — but only when it
// was saved with revealable=true. A write-only entry's ciphertext is still
// sitting in the same column (SecretsForRun still needs to decrypt it to
// actually inject the value into a run), but this path refuses it
// unconditionally: that refusal, not the encryption itself, is what makes
// "write-only" a real promise instead of just a UI convention the caller
// could bypass by hitting the endpoint directly.
func GetSecretValue(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, workflowFile string) (string, error) {
	var ciphertext []byte
	var revealable bool
	err := db.QueryRow(
		`SELECT value_encrypted, revealable FROM secrets WHERE repo_path = ? AND kind = ? AND key = ? AND workflow_file = ?`,
		repoPath, string(kind), name, workflowFile,
	).Scan(&ciphertext, &revealable)
	if err != nil {
		return "", err
	}
	if !revealable {
		return "", errors.New("this entry was saved as write-only and can't be viewed again")
	}
	plaintext, err := Decrypt(encKey, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func DeleteSecret(db *sql.DB, repoPath string, kind SecretKind, name, workflowFile string) error {
	_, err := db.Exec(
		`DELETE FROM secrets WHERE repo_path = ? AND kind = ? AND key = ? AND workflow_file = ?`,
		repoPath, string(kind), name, workflowFile,
	)
	return err
}

// SecretsForRun returns the decrypted values injected into a run of
// workflowFile: repo-wide entries overlaid by workflow-specific entries
// (workflow wins on name clash). The ORDER BY puts ” (repo-wide) first so
// the overlay is a simple map overwrite.
func SecretsForRun(db *sql.DB, encKey []byte, repoPath, workflowFile string, kind SecretKind) (map[string]string, error) {
	rows, err := db.Query(
		`SELECT key, value_encrypted FROM secrets
		 WHERE repo_path = ? AND kind = ? AND workflow_file IN ('', ?)
		 ORDER BY workflow_file, key`,
		repoPath, string(kind), workflowFile,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var name string
		var ciphertext []byte
		if err := rows.Scan(&name, &ciphertext); err != nil {
			return nil, err
		}
		plaintext, err := Decrypt(encKey, ciphertext)
		if err != nil {
			return nil, err
		}
		values[name] = string(plaintext)
	}
	return values, rows.Err()
}
