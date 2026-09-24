package service

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
	if !contains(body, `"tableName":"users"`) || !contains(body, "id:users\n") {
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

// TestSseServer_ConcurrentWrites exercises Send/Ping/Broadcast/Open/Close
// concurrently (run with -race): every write goes through the session lock.
func TestSseServer_ConcurrentWrites(t *testing.T) {
	s := NewSseServerWithInterval(nil, time.Millisecond)
	defer s.Stop()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		node := fmt.Sprintf("node-%d", i)
		s.Open(node, newMockFlusher())
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.Send(node, &SyncData{TableName: "project", Action: "update", Data: map[string]int{"j": j}})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.Ping()
				s.Broadcast(&SyncData{TableName: "node"})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.Open(node, newMockFlusher())
				s.IsSubscribed(node, "project")
				if j%10 == 0 {
					s.Close(node)
				}
			}
		}()
	}
	wg.Wait()
}

func TestParseSubscriptions(t *testing.T) {
	raw := `[{"tableName":"org.secretflow.secretpad.persistence.entity.ProjectDO","lastUpdateTime":"2024-01-01 00:00:00"},{"table_name":"node"}]`
	for _, in := range []string{raw, url.QueryEscape(raw)} {
		subs, err := ParseSubscriptions(in)
		if err != nil || len(subs) != 2 || subs[1].TableName != "node" || subs[0].LastUpdateTime != "2024-01-01 00:00:00" {
			t.Fatalf("parse %q: %+v %v", in, subs, err)
		}
	}
	if _, err := ParseSubscriptions("{bad"); err == nil {
		t.Fatal("expected error")
	}
}

// TestSseServer_HTTPStreamSurvivesServerTimeouts checks that HandleSseSync
// lifts http.Server Read/WriteTimeout, filters subscriptions and requires
// kuscia-origin-source.
func TestSseServer_HTTPStreamSurvivesServerTimeouts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewSseServerWithInterval(nil, 50*time.Millisecond)
	defer s.Stop()
	s.SetTableFilter(func(n string) (string, bool) { return n, n != "user_accounts" })
	opened := make(chan []SyncData, 1)
	s.SetOpenHook(func(node string, subs []SyncData) { opened <- subs })
	r := gin.New()
	r.GET("/sync", s.HandleSseSync)
	srv := httptest.NewUnstartedServer(r)
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Config.ReadTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sync")
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 256)
	n, _ := resp.Body.Read(b)
	resp.Body.Close()
	if !strings.Contains(string(b[:n]), "202011109") {
		t.Fatalf("missing origin must fail with SSE_ERROR, got %s", b[:n])
	}

	p := url.QueryEscape(`[{"tableName":"project","lastUpdateTime":"0"},{"tableName":"user_accounts"}]`)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/sync?p="+p, nil)
	req.Header.Set("kuscia-origin-source", "edge1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	select {
	case subs := <-opened:
		if len(subs) != 1 || subs[0].TableName != "project" {
			t.Fatalf("subscriptions %+v", subs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("open hook not called")
	}
	sc := bufio.NewScanner(resp.Body)
	lines := make(chan string, 100)
	go func() {
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	time.Sleep(300 * time.Millisecond) // well past both server timeouts
	if !s.Send("edge1", &SyncData{TableName: "project", Action: "create", Data: map[string]string{"project_id": "p1"}}) {
		t.Fatal("send after server timeouts failed")
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("stream closed")
			}
			if strings.HasPrefix(l, "data:") && strings.Contains(l, `"p1"`) {
				return
			}
		case <-deadline:
			t.Fatal("data event not received")
		}
	}
}
