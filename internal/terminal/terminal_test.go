package terminal

import (
	"os"
	"strings"
	"testing"
	"time"
)

func waitForOutput(t *testing.T, s *Session, contains string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		buf := string(s.buf)
		s.mu.Unlock()
		if strings.Contains(buf, contains) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for output containing %q", contains)
}

func TestManager_CreateListKill(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()

	s, err := m.Create(dir)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.ID == "" {
		t.Fatal("expected non-empty session id")
	}

	ids := m.List(dir)
	if len(ids) != 1 || ids[0] != s.ID {
		t.Fatalf("List(%q) = %v, want [%s]", dir, ids, s.ID)
	}

	if got, ok := m.Get(s.ID); !ok || got != s {
		t.Fatalf("Get(%q) = %v, %v", s.ID, got, ok)
	}

	if !m.Kill(s.ID) {
		t.Fatal("Kill on live session returned false")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := m.Get(s.ID); !ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := m.Get(s.ID); ok {
		t.Fatal("session still present after Kill")
	}

	if m.Kill("nonexistent") {
		t.Fatal("Kill on unknown id returned true")
	}
}

func TestManager_ListScopedByRepoPath(t *testing.T) {
	m := NewManager()
	a, err := m.Create(t.TempDir())
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	defer m.Kill(a.ID)

	if ids := m.List("/some/other/repo"); len(ids) != 0 {
		t.Fatalf("List for unrelated repo = %v, want empty", ids)
	}
}

func TestSession_BufferCapped(t *testing.T) {
	m := NewManager()
	s, err := m.Create(t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer m.Kill(s.ID)

	s.broadcast(make([]byte, bufferCap+1000))
	s.mu.Lock()
	n := len(s.buf)
	s.mu.Unlock()
	if n != bufferCap {
		t.Fatalf("buffer len = %d, want %d", n, bufferCap)
	}
}

func TestSession_ShellRuns(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()
	s, err := m.Create(dir)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer m.Kill(s.ID)

	s.ptmx.Write([]byte("echo hello-terminal\r"))
	waitForOutput(t, s, "hello-terminal")
}

func TestManager_CreateEmptyRepoPathDefaultsToHome(t *testing.T) {
	m := NewManager()
	s, err := m.Create("")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer m.Kill(s.ID)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	s.ptmx.Write([]byte("pwd\r"))
	waitForOutput(t, s, home)
}
