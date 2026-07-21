package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHub_ReplayThenLiveBroadcast(t *testing.T) {
	hub := NewHub()

	hub.Broadcast(42, "buffered line 1")
	hub.Broadcast(42, "buffered line 2")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, 42)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read buffered 1: %v", err)
	}
	if string(msg1) != "buffered line 1" {
		t.Fatalf("expected buffered line 1, got %q", msg1)
	}

	_, msg2, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read buffered 2: %v", err)
	}
	if string(msg2) != "buffered line 2" {
		t.Fatalf("expected buffered line 2, got %q", msg2)
	}

	time.Sleep(50 * time.Millisecond) // let the client register before broadcasting live
	hub.Broadcast(42, "live line")

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg3, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read live: %v", err)
	}
	if string(msg3) != "live line" {
		t.Fatalf("expected live line, got %q", msg3)
	}
}
