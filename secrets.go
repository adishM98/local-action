package main

import (
	"database/sql"
)

type SecretKind string

const (
	KindSecret SecretKind = "secret"
	KindVar    SecretKind = "var"
)

type SecretEntry struct {
	RepoPath string     `json:"repoPath"`
	Kind     SecretKind `json:"kind"`
	Key      string     `json:"key"`
}

func UpsertSecret(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name, value string) error {
	ciphertext, err := Encrypt(encKey, []byte(value))
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO secrets (repo_path, kind, key, value_encrypted) VALUES (?, ?, ?, ?)
		 ON CONFLICT(repo_path, kind, key) DO UPDATE SET value_encrypted = excluded.value_encrypted`,
		repoPath, string(kind), name, ciphertext,
	)
	return err
}

func ListSecrets(db *sql.DB, repoPath string, kind SecretKind) ([]SecretEntry, error) {
	rows, err := db.Query(
		`SELECT repo_path, kind, key FROM secrets WHERE repo_path = ? AND kind = ? ORDER BY key`,
		repoPath, string(kind),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []SecretEntry
	for rows.Next() {
		var e SecretEntry
		var kindStr string
		if err := rows.Scan(&e.RepoPath, &kindStr, &e.Key); err != nil {
			return nil, err
		}
		e.Kind = SecretKind(kindStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func GetSecretValue(db *sql.DB, encKey []byte, repoPath string, kind SecretKind, name string) (string, error) {
	var ciphertext []byte
	err := db.QueryRow(
		`SELECT value_encrypted FROM secrets WHERE repo_path = ? AND kind = ? AND key = ?`,
		repoPath, string(kind), name,
	).Scan(&ciphertext)
	if err != nil {
		return "", err
	}
	plaintext, err := Decrypt(encKey, ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func DeleteSecret(db *sql.DB, repoPath string, kind SecretKind, name string) error {
	_, err := db.Exec(
		`DELETE FROM secrets WHERE repo_path = ? AND kind = ? AND key = ?`,
		repoPath, string(kind), name,
	)
	return err
}
