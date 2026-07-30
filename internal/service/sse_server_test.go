package service

import (
	"net/http/httptest"
	"testing"
	"time"
)

// mockFlusher implements http.ResponseWriter + http.Flusher for SSE tests.
type mockFlusher struct {
	*httptest.ResponseRecorder
}

func (m *mockFlusher) Flush() {
	m.ResponseRecorder.Flush()
}

func newMockFlusher() *mockFlusher {
	return &mockFlusher{ResponseRecorder: httptest.NewRecorder()}
}

func TestSseServer_OpenAndClose(t *testing.T) {
	s := NewSseServer(nil) // nil logger should not panic
	defer s.Stop()

	w := newMockFlusher()
	session, err := s.Open("node-1", w)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if session.NodeID != "node-1" {
		t.Fatalf("expected node-1, got %s", session.NodeID)
	}

	if s.ActiveConnections() != 1 {
		t.Fatalf("expected 1 active connection, got %d", s.ActiveConnections())
	}

	closed := s.Close("node-1")
	if !closed {
		t.Fatal("expected Close to return true for existing session")
	}
	if s.ActiveConnections() != 0 {
		t.Fatalf("expected 0 active connections after close, got %d", s.ActiveConnections())
	}
}

func TestSseServer_CloseNonExistent(t *testing.T) {
	s := NewSseServer(nil)
	defer s.Stop()

	closed := s.Close("nonexistent")
	if closed {
		t.Fatal("expected Close to return false for non-existent session")
	}
}

func TestSseServer_OpenReplacesExisting(t *testing.T) {
	s := NewSseServer(nil)
	defer s.Stop()

	w1 := newMockFlusher()
	session1, _ := s.Open("node-1", w1)

	w2 := newMockFlusher()
	session2, _ := s.Open("node-1", w2)

	// Old session's Done channel should be closed
	select {
	case <-session1.Done:
		// expected
	default:
		t.Fatal("expected old session Done channel to be closed")
	}

	// New session should be active
	select {
	case <-session2.Done:
		t.Fatal("new session Done should not be closed")
	default:
		// expected
	}

	if s.ActiveConnections() != 1 {
		t.Fatalf("expected 1 connection after re-open, got %d", s.ActiveConnections())
	}
}

func TestSseServer_Send(t *testing.T) {
	s := NewSseServer(nil)
	defer s.Stop()

	w := newMockFlusher()
	s.Open("node-1", w)

	data := &SyncDataDTO{
		TableName:      "users",
		Action:         "INSERT",
		Data:           map[string]string{"id": "1"},
		LastUpdateTime: "2024-01-01 00:00:00",
	}

	ok := s.Send("node-1", data)
	if !ok {
		t.Fatal("expected Send to succeed for existing session")
	}

	body := w.Body.String()
	if body == "" {
		t.Fatal("expected data written to response writer")
	}
	if !contains(body, `"table_name":"users"`) {
		t.Fatalf("expected JSON payload in SSE data, got %q", body)
	}
}

func TestSseServer_SendNonExistent(t *testing.T) {
	s := NewSseServer(nil)
	defer s.Stop()

	data := &SyncDataDTO{TableName: "test"}
	ok := s.Send("nonexistent", data)
	if ok {
		t.Fatal("expected Send to return false for non-existent session")
	}
}

func TestSseServer_Broadcast(t *testing.T) {
	s := NewSseServer(nil)
	defer s.Stop()

	w1 := newMockFlusher()
	w2 := newMockFlusher()
	w3 := newMockFlusher()
	s.Open("node-1", w1)
	s.Open("node-2", w2)
	s.Open("node-3", w3)

	data := &SyncDataDTO{TableName: "projects", Action: "UPDATE"}
	count := s.Broadcast(data)
	if count != 3 {
		t.Fatalf("expected broadcast to 3 nodes, got %d", count)
	}
}

func TestSseServer_Stop(t *testing.T) {
	s := NewSseServer(nil)

	w1 := newMockFlusher()
	w2 := newMockFlusher()
	session1, _ := s.Open("node-1", w1)
	session2, _ := s.Open("node-2", w2)

	s.Stop()

	// All sessions should have Done closed
	select {
	case <-session1.Done:
	default:
		t.Fatal("session1 Done should be closed after Stop")
	}
	select {
	case <-session2.Done:
	default:
		t.Fatal("session2 Done should be closed after Stop")
	}

	if s.ActiveConnections() != 0 {
		t.Fatalf("expected 0 connections after Stop, got %d", s.ActiveConnections())
	}

	// Stop should be idempotent
	s.Stop()
}

func TestSseServer_Ping(t *testing.T) {
	s := NewSseServer(nil)
	defer s.Stop()

	w := newMockFlusher()
	s.Open("node-1", w)

	s.Ping()

	body := w.Body.String()
	if !contains(body, ": ping") {
		t.Fatalf("expected ping comment in SSE output, got %q", body)
	}
}

func TestSseServer_HeartbeatStopsOnStop(t *testing.T) {
	s := NewSseServer(nil)

	// Verify Stop doesn't hang (heartbeat goroutine exits)
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete in time — goroutine leak")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
