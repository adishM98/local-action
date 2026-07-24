package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

		var dockerStderr bytes.Buffer
		dockerCmd := exec.CommandContext(ctx, "docker", "info")
		dockerCmd.Stdout = io.Discard
		dockerCmd.Stderr = &dockerStderr
		dockerErr := dockerCmd.Run()

		resp := map[string]any{
			"actOK":      actErr == nil,
			"actVersion": string(actOut),
			"dockerOK":   dockerErr == nil,
		}
		if dockerErr != nil {
			msg := strings.TrimSpace(dockerStderr.String())
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				msg = "docker info timed out (daemon may be starting)"
			} else if msg == "" {
				msg = dockerErr.Error()
			}
			resp["dockerError"] = truncateTail(msg, 300)
		}
		writeJSON(w, http.StatusOK, resp)
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
			RepoPath     string     `json:"repoPath"`
			Kind         SecretKind `json:"kind"`
			Key          string     `json:"key"`
			Value        string     `json:"value"`
			WorkflowFile string     `json:"workflowFile"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := UpsertSecret(db, key, body.RepoPath, body.Kind, body.Key, body.Value, body.WorkflowFile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath     string     `json:"repoPath"`
			Kind         SecretKind `json:"kind"`
			Key          string     `json:"key"`
			WorkflowFile string     `json:"workflowFile"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := DeleteSecret(db, body.RepoPath, body.Kind, body.Key, body.WorkflowFile); err != nil {
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
			EventPayload string            `json:"eventPayload"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.EventPayload != "" && !json.Valid([]byte(body.EventPayload)) {
			http.Error(w, "eventPayload is not valid JSON", http.StatusBadRequest)
			return
		}
		runID, err := engine.Enqueue(RunRequest{
			RepoPath:     body.RepoPath,
			WorkflowFile: body.WorkflowFile,
			Event:        body.Event,
			Inputs:       body.Inputs,
			ExtraSecrets: body.ExtraSecrets,
			ExtraVars:    body.ExtraVars,
			EventPayload: body.EventPayload,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"runId": runID})
	})

	mux.HandleFunc("GET /api/event-payload", func(w http.ResponseWriter, r *http.Request) {
		repoPath := r.URL.Query().Get("repoPath")
		workflowFile := r.URL.Query().Get("workflowFile")
		payload, err := GetEventPayload(db, repoPath, workflowFile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"payload": payload})
	})

	mux.HandleFunc("POST /api/event-payload", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RepoPath     string `json:"repoPath"`
			WorkflowFile string `json:"workflowFile"`
			Payload      string `json:"payload"`
		}
		if err := readJSON(r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.Payload != "" && !json.Valid([]byte(body.Payload)) {
			http.Error(w, "payload is not valid JSON", http.StatusBadRequest)
			return
		}
		if err := SaveEventPayload(db, body.RepoPath, body.WorkflowFile, body.Payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
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

// truncateTail keeps the LAST max bytes of s (the tail carries the real
// error in multi-line CLI output), trimmed to a rune boundary.
func truncateTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	for i := 0; i < len(s); i++ {
		if utf8.RuneStart(s[i]) {
			return s[i:]
		}
	}
	return s
}
