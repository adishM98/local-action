package app

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // local-only tool, no cross-origin threat model
}

type wsClient struct {
	send chan string
}

type Hub struct {
	mu      sync.Mutex
	buffers map[int64][]string
	clients map[int64]map[*wsClient]bool
}

func NewHub() *Hub {
	return &Hub{
		buffers: map[int64][]string{},
		clients: map[int64]map[*wsClient]bool{},
	}
}

// Forget releases the buffered log lines for runID. It must only be called
// once a run has reached a terminal status (success/failed/cancelled) — the
// buffer is what allows a client connecting mid-run to replay history, so
// freeing it earlier would drop lines for anyone still watching. The
// run_logs table remains the durable source of truth for replay after this.
func (h *Hub) Forget(runID int64) {
	h.mu.Lock()
	delete(h.buffers, runID)
	h.mu.Unlock()
}

func (h *Hub) Broadcast(runID int64, line string) {
	h.mu.Lock()
	h.buffers[runID] = append(h.buffers[runID], line)
	var recipients []*wsClient
	for c := range h.clients[runID] {
		recipients = append(recipients, c)
	}
	h.mu.Unlock()

	for _, c := range recipients {
		select {
		case c.send <- line:
		default:
		}
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, runID int64) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := &wsClient{send: make(chan string, 256)}

	h.mu.Lock()
	buffered := append([]string(nil), h.buffers[runID]...)
	if h.clients[runID] == nil {
		h.clients[runID] = map[*wsClient]bool{}
	}
	h.clients[runID][client] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients[runID], client)
		h.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for _, line := range buffered {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			return err
		}
	}

	for {
		select {
		case line := <-client.send:
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				return err
			}
		case <-done:
			return nil
		}
	}
}
