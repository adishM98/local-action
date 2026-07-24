package app

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
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
	branch, sha := gitInfo(req.RepoPath)
	runID, err := CreateRun(e.db, Run{
		RepoPath:     req.RepoPath,
		WorkflowFile: req.WorkflowFile,
		Event:        req.Event,
		Inputs:       encodeInputs(req.Inputs),
		Status:       StatusQueued,
		CreatedAt:    time.Now().Unix(),
		Branch:       branch,
		CommitSHA:    sha,
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

	secretFile, varFile, envFile, eventPayloadFile, cleanup, err := e.writeTempFiles(req)
	if err != nil {
		e.failEarly(runID, started, fmt.Errorf("preparing secrets/vars for act: %w", err))
		return
	}
	defer cleanup()

	argv := BuildArgv(req, secretFile, varFile, envFile, eventPayloadFile)
	cmd := exec.Command(e.actBin, argv...)
	cmd.Dir = req.RepoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.failEarly(runID, started, fmt.Errorf("opening act stdout: %w", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.failEarly(runID, started, fmt.Errorf("opening act stderr: %w", err))
		return
	}

	if err := cmd.Start(); err != nil {
		e.failEarly(runID, started, fmt.Errorf("starting act (bin=%q): %w", e.actBin, err))
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
			if err := AppendRunLog(e.db, runID, n, text); err != nil {
				log.Printf("run %d: append log line %d: %v", runID, n, err)
			}
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
	if err := UpdateRunStatus(e.db, runID, status, &started, &finished); err != nil {
		log.Printf("run %d: update status to %s: %v", runID, status, err)
	}
	if e.onFinish != nil {
		e.onFinish(runID)
	}
}

// failEarly records why a run never got to the point of streaming act's own
// output, so the log viewer shows a reason instead of an empty pane. Without
// this, a missing act binary or a broken temp-file write left StatusFailed
// with zero log lines and no way to tell what went wrong.
func (e *Engine) failEarly(runID int64, started int64, cause error) {
	message := "local-action: " + cause.Error()
	if err := AppendRunLog(e.db, runID, 1, message); err != nil {
		log.Printf("run %d: append early-failure log: %v", runID, err)
	}
	if e.onLine != nil {
		e.onLine(runID, message)
	}
	e.finish(runID, StatusFailed, started)
}

func (e *Engine) writeTempFiles(req RunRequest) (secretFile, varFile, envFile, eventPayloadFile string, cleanup func(), err error) {
	secrets, err := SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, KindSecret)
	if err != nil {
		return "", "", "", "", nil, err
	}
	vars, err := SecretsForRun(e.db, e.key, req.RepoPath, req.WorkflowFile, KindVar)
	if err != nil {
		return "", "", "", "", nil, err
	}

	sf, err := writeDotenvTemp("act-secrets-*.env", secrets, req.ExtraSecrets)
	if err != nil {
		return "", "", "", "", nil, err
	}
	vf, err := writeDotenvTemp("act-vars-*.env", vars, req.ExtraVars)
	if err != nil {
		os.Remove(sf)
		return "", "", "", "", nil, err
	}
	// Explicitly empty --env-file so act never falls back to auto-loading
	// a ".env" from its working directory (the target repo itself).
	ef, err := writeDotenvTemp("act-empty-*.env", nil, nil)
	if err != nil {
		os.Remove(sf)
		os.Remove(vf)
		return "", "", "", "", nil, err
	}
	var pf string
	if req.EventPayload != "" {
		pf, err = writeTempFile("act-event-*.json", req.EventPayload)
		if err != nil {
			os.Remove(sf)
			os.Remove(vf)
			os.Remove(ef)
			return "", "", "", "", nil, err
		}
	}
	cleanup = func() {
		os.Remove(sf)
		os.Remove(vf)
		os.Remove(ef)
		if pf != "" {
			os.Remove(pf)
		}
	}
	return sf, vf, ef, pf, cleanup, nil
}

func writeTempFile(pattern, content string) (string, error) {
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
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	success = true
	return name, nil
}

func writeDotenvTemp(pattern string, values, extra map[string]string) (string, error) {
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
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(f, "%s=%s\n", k, values[k])
	}
	for k, v := range extra {
		fmt.Fprintf(f, "%s=%s\n", k, v)
	}
	success = true
	return name, nil
}
