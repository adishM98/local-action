// Package terminal manages interactive shell sessions (real ptys) backing
// the in-app terminal panel — the user runs `claude`, git, etc. against the
// scanned repo without tabbing out to a separate terminal app.
package terminal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// bufferCap caps the replay buffer per session — enough scrollback to
// reconnect after a refresh without letting a long-lived shell grow it
// unbounded.
const bufferCap = 64 * 1024

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // local-only tool, no cross-origin threat model
}

// Session is one real shell process with its pty. It keeps running
// independent of whether any client is attached — closing the panel just
// detaches; the shell survives until explicitly killed or it exits itself.
type Session struct {
	ID       string
	RepoPath string

	ptmx *os.File
	cmd  *exec.Cmd

	mu      sync.Mutex
	buf     []byte
	clients map[chan []byte]bool
	closed  bool
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}}
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func shellPath() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/zsh"
}

// Create spawns a login shell rooted at repoPath and starts pumping its
// output in the background.
func (m *Manager) Create(repoPath string) (*Session, error) {
	cmd := exec.Command(shellPath(), "-l")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:       newID(),
		RepoPath: repoPath,
		ptmx:     ptmx,
		cmd:      cmd,
		clients:  map[chan []byte]bool{},
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	go s.pump(m)

	return s, nil
}

// pump reads pty output until the shell exits, broadcasting to whoever's
// attached and buffering for replay. Runs once per session for its whole
// lifetime, not per WS connection.
func (s *Session) pump(m *Manager) {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.broadcast(buf[:n])
		}
		if err != nil {
			break
		}
	}

	s.mu.Lock()
	s.closed = true
	clients := make([]chan []byte, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clients = map[chan []byte]bool{}
	s.mu.Unlock()
	for _, c := range clients {
		close(c)
	}

	m.mu.Lock()
	delete(m.sessions, s.ID)
	m.mu.Unlock()
}

func (s *Session) broadcast(p []byte) {
	s.mu.Lock()
	s.buf = append(s.buf, p...)
	if len(s.buf) > bufferCap {
		s.buf = s.buf[len(s.buf)-bufferCap:]
	}
	recipients := make([]chan []byte, 0, len(s.clients))
	for c := range s.clients {
		recipients = append(recipients, c)
	}
	s.mu.Unlock()

	chunk := append([]byte(nil), p...)
	for _, c := range recipients {
		select {
		case c <- chunk:
		default:
		}
	}
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns live session ids for repoPath, in creation order isn't
// preserved (map iteration) — the frontend doesn't depend on tab order
// surviving a refresh exactly, just that the tabs come back.
func (m *Manager) List(repoPath string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := []string{}
	for id, s := range m.sessions {
		if s.RepoPath == repoPath {
			ids = append(ids, id)
		}
	}
	return ids
}

// Kill terminates the shell process. The pump goroutine notices the pty
// read failing and does the actual cleanup (removing it from the manager,
// closing client channels).
func (m *Manager) Kill(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return false
	}
	s.cmd.Process.Kill()
	return true
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// ServeWS attaches a client to the session: binary frames carry raw pty
// bytes both directions, a JSON text frame with type "resize" tells the pty
// about a viewport size change.
func (s *Session) ServeWS(w http.ResponseWriter, r *http.Request) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	ch := make(chan []byte, 256)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	buffered := append([]byte(nil), s.buf...)
	s.clients[ch] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()

	if len(buffered) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, buffered); err != nil {
			return err
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				s.ptmx.Write(data)
			case websocket.TextMessage:
				var msg resizeMsg
				if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
					pty.Setsize(s.ptmx, &pty.Winsize{Rows: msg.Rows, Cols: msg.Cols})
				}
			}
		}
	}()

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return nil
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return err
			}
		case <-done:
			return nil
		}
	}
}
