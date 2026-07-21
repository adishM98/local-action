package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func NewRouter(db *sql.DB, key []byte, engine *Engine, hub *Hub, actBin string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		actOut, actErr := exec.CommandContext(ctx, actBin, "--version").CombinedOutput()
		dockerErr := exec.CommandContext(ctx, "docker", "info").Run()

		writeJSON(w, http.StatusOK, map[string]any{
			"actOK":      actErr == nil,
			"actVersion": string(actOut),
			"dockerOK":   dockerErr == nil,
		})
	})

	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		workflows, err := ScanWorkflows(body.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, workflows)
	})

	mux.HandleFunc("GET /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		repoPath := r.URL.Query().Get("repoPath")
		kind := SecretKind(r.URL.Query().Get("kind"))
		entries, err := ListSecrets(db, repoPath, kind)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})

	mux.HandleFunc("POST /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath string     `json:"repoPath"`
			Kind     SecretKind `json:"kind"`
			Key      string     `json:"key"`
			Value    string     `json:"value"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := UpsertSecret(db, key, body.RepoPath, body.Kind, body.Key, body.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath string     `json:"repoPath"`
			Kind     SecretKind `json:"kind"`
			Key      string     `json:"key"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := DeleteSecret(db, body.RepoPath, body.Kind, body.Key); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath     string            `json:"repoPath"`
			WorkflowFile string            `json:"workflowFile"`
			Event        string            `json:"event"`
			Inputs       map[string]string `json:"inputs"`
			ExtraSecrets map[string]string `json:"extraSecrets"`
			ExtraVars    map[string]string `json:"extraVars"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runID, err := engine.Enqueue(RunRequest{
			RepoPath:     body.RepoPath,
			WorkflowFile: body.WorkflowFile,
			Event:        body.Event,
			Inputs:       body.Inputs,
			ExtraSecrets: body.ExtraSecrets,
			ExtraVars:    body.ExtraVars,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"runId": runID})
	})

	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		repoPath := r.URL.Query().Get("repoPath")
		runs, err := ListRuns(db, repoPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	})

	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		run, err := GetRun(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		logs, err := GetRunLogs(db, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run": run, "logs": logs})
	})

	mux.HandleFunc("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		if !engine.Cancel(id) {
			http.Error(w, "run not active", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /ws/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		hub.ServeWS(w, r, id)
	})

	return mux
}
