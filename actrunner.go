package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type LineHandler func(runID int64, line string)

// FinishHandler is invoked exactly once per run, after the run has reached a
// terminal status (success/failed/cancelled) and all its logs have been
// persisted. It's used to release resources scoped to the lifetime of a
// single run, e.g. the WebSocket hub's per-run log buffer.
type FinishHandler func(runID int64)

type Engine struct {
	db       *sql.DB
	key      []byte
	actBin   string
	onLine   LineHandler
	onFinish FinishHandler
	queue    chan queuedRun
	mu       sync.Mutex
	running  map[int64]*exec.Cmd
}

type queuedRun struct {
	runID int64
	req   RunRequest
}

func NewEngine(db *sql.DB, key []byte, actBin string, onLine LineHandler, onFinish FinishHandler) *Engine {
	e := &Engine{
		db:       db,
		key:      key,
		actBin:   actBin,
		onLine:   onLine,
		onFinish: onFinish,
		queue:    make(chan queuedRun, 100),
		running:  map[int64]*exec.Cmd{},
	}
	go e.worker()
	return e
}

func encodeInputs(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (e *Engine) Enqueue(req RunRequest) (int64, error) {
	runID, err := CreateRun(e.db, Run{
		RepoPath:     req.RepoPath,
		WorkflowFile: req.WorkflowFile,
		Event:        req.Event,
		Inputs:       encodeInputs(req.Inputs),
		Status:       StatusQueued,
		CreatedAt:    time.Now().Unix(),
	})
	if err != nil {
		return 0, err
	}
	e.queue <- queuedRun{runID: runID, req: req}
	return runID, nil
}

func (e *Engine) Cancel(runID int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	cmd, ok := e.running[runID]
	if !ok || cmd.Process == nil {
		return false
	}
	return cmd.Process.Kill() == nil
}

func (e *Engine) worker() {
	for qr := range e.queue {
		e.runOne(qr.runID, qr.req)
	}
}

func (e *Engine) runOne(runID int64, req RunRequest) {
	started := time.Now().Unix()

	secretFile, varFile, cleanup, err := e.writeTempFiles(req)
	if err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}
	defer cleanup()

	argv := BuildArgv(req, secretFile, varFile)
	cmd := exec.Command(e.actBin, argv...)
	cmd.Dir = req.RepoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}

	if err := cmd.Start(); err != nil {
		e.finish(runID, StatusFailed, started)
		return
	}

	// Register the process in e.running before announcing StatusRunning so
	// that any caller which observes StatusRunning via GetRun is guaranteed
	// Cancel() will find the process already tracked here. Doing this the
	// other way around (status first, map second) races: a caller can read
	// "running" from the DB and call Cancel() in the window before the map
	// entry exists, causing Cancel() to spuriously return false.
	e.mu.Lock()
	e.running[runID] = cmd
	e.mu.Unlock()

	UpdateRunStatus(e.db, runID, StatusRunning, &started, nil)

	var lineMu sync.Mutex
	lineNo := 0
	var wg sync.WaitGroup
	stream := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lineMu.Lock()
			lineNo++
			n := lineNo
			lineMu.Unlock()
			text := scanner.Text()
			AppendRunLog(e.db, runID, n, text)
			if e.onLine != nil {
				e.onLine(runID, text)
			}
		}
	}
	wg.Add(2)
	go stream(stdout)
	go stream(stderr)
	wg.Wait()

	waitErr := cmd.Wait()

	e.mu.Lock()
	delete(e.running, runID)
	e.mu.Unlock()

	status := StatusSuccess
	if waitErr != nil {
		status = StatusFailed
		if cmd.ProcessState != nil && strings.Contains(cmd.ProcessState.String(), "signal: killed") {
			status = StatusCancelled
		}
	}
	e.finish(runID, status, started)
}

func (e *Engine) finish(runID int64, status RunStatus, started int64) {
	finished := time.Now().Unix()
	UpdateRunStatus(e.db, runID, status, &started, &finished)
	if e.onFinish != nil {
		e.onFinish(runID)
	}
}

func (e *Engine) writeTempFiles(req RunRequest) (secretFile, varFile string, cleanup func(), err error) {
	secrets, err := ListSecrets(e.db, req.RepoPath, KindSecret)
	if err != nil {
		return "", "", nil, err
	}
	vars, err := ListSecrets(e.db, req.RepoPath, KindVar)
	if err != nil {
		return "", "", nil, err
	}

	sf, err := e.writeDotenvTemp("act-secrets-*.env", req.RepoPath, secrets, req.ExtraSecrets)
	if err != nil {
		return "", "", nil, err
	}
	vf, err := e.writeDotenvTemp("act-vars-*.env", req.RepoPath, vars, req.ExtraVars)
	if err != nil {
		os.Remove(sf)
		return "", "", nil, err
	}
	cleanup = func() {
		os.Remove(sf)
		os.Remove(vf)
	}
	return sf, vf, cleanup, nil
}

func (e *Engine) writeDotenvTemp(pattern, repoPath string, entries []SecretEntry, extra map[string]string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	success := false
	defer func() {
		f.Close()
		if !success {
			os.Remove(name)
		}
	}()
	if err := f.Chmod(0600); err != nil {
		return "", err
	}
	for _, entry := range entries {
		val, err := GetSecretValue(e.db, e.key, repoPath, entry.Kind, entry.Key)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(f, "%s=%s\n", entry.Key, val)
	}
	for k, v := range extra {
		fmt.Fprintf(f, "%s=%s\n", k, v)
	}
	success = true
	return name, nil
}
