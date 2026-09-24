package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// SsePingMsg is the heartbeat event id (Java SseSession.SSE_PING_MSG).
const SsePingMsg = "ping"

// sseWriteTimeout bounds a single SSE write so a stuck client cannot block
// the dispatcher forever.
const sseWriteTimeout = 30 * time.Second

// SyncDataDTO is kept as an alias of SyncData for existing callers.
type SyncDataDTO = SyncData

// SseSession represents a single SSE client connection (Java SseSession
// sessionMap + sessionTableMap entry).
type SseSession struct {
	NodeID string
	Writer http.ResponseWriter
	// Flusher is kept for backward compatibility; writes go through write().
	Flusher http.Flusher
	Done    chan struct{}
	// Tables are the subscribed tables with the client's last update time.
	Tables []SyncData

	rc        *http.ResponseController
	writeMu   sync.Mutex // serializes every write+flush on this stream
	closeOnce sync.Once
	mu        sync.Mutex // guards LastActive
	// LastActive is the time of the last successful write.
	LastActive time.Time
}

// write writes one SSE frame under the per-session write lock and flushes it.
func (s *SseSession) write(frame string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.Done:
		return fmt.Errorf("sse session %s closed", s.NodeID)
	default:
	}
	if s.rc != nil {
		_ = s.rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
	}
	if _, err := s.Writer.Write([]byte(frame)); err != nil {
		return err
	}
	if s.rc != nil {
		if err := s.rc.Flush(); err != nil {
			return err
		}
	} else if s.Flusher != nil {
		s.Flusher.Flush()
	}
	s.mu.Lock()
	s.LastActive = time.Now()
	s.mu.Unlock()
	return nil
}

// close marks the session done. It takes the write lock so no write is in
// flight once it returns (the handler must not return while a write runs).
func (s *SseSession) close() {
	s.writeMu.Lock()
	s.closeOnce.Do(func() { close(s.Done) })
	s.writeMu.Unlock()
}

// Subscribed reports whether the session subscribed to tableName.
func (s *SseSession) Subscribed(tableName string) bool {
	for _, t := range s.Tables {
		if t.TableName == tableName {
			return true
		}
	}
	return false
}

// SseOpenHook runs after a session opened (Java
// JpaSyncDataService.syncByLastUpdateTime); it is called in its own goroutine.
type SseOpenHook func(nodeID string, subscriptions []SyncData)

// SseServer manages SSE connections for center-edge data synchronization
// (Java SseServerImpl + CenterDataSyncController).
type SseServer struct {
	mu       sync.RWMutex
	sessions map[string]*SseSession
	log      *zap.Logger
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once

	hookMu sync.RWMutex
	onOpen SseOpenHook
	// tableFilter canonicalizes/filters subscribed table names (nil: keep).
	tableFilter func(string) (string, bool)
}

// NewSseServer creates a new SSE server with a 15s heartbeat.
func NewSseServer(log *zap.Logger) *SseServer {
	return NewSseServerWithInterval(log, 15*time.Second)
}

// NewSseServerWithInterval creates a new SSE server with the given heartbeat period.
func NewSseServerWithInterval(log *zap.Logger, ping time.Duration) *SseServer {
	if log == nil {
		log = zap.NewNop()
	}
	if ping <= 0 {
		ping = 15 * time.Second
	}
	s := &SseServer{
		sessions: make(map[string]*SseSession),
		log:      log,
		interval: ping,
		stopCh:   make(chan struct{}),
	}
	go s.heartbeatLoop()
	return s
}

// SetOpenHook installs the hook run after every session open.
func (s *SseServer) SetOpenHook(h SseOpenHook) {
	s.hookMu.Lock()
	s.onOpen = h
	s.hookMu.Unlock()
}

// SetTableFilter installs the subscription table canonicalizer: it maps a
// requested table name (Go table or Java class name) to the Go table name and
// reports whether the table may be synchronized.
func (s *SseServer) SetTableFilter(f func(string) (string, bool)) {
	s.hookMu.Lock()
	s.tableFilter = f
	s.hookMu.Unlock()
}

// Stop gracefully shuts down the SSE server, closing all sessions and
// stopping the heartbeat goroutine. Safe to call multiple times.
func (s *SseServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.mu.Lock()
	for id, session := range s.sessions {
		session.close()
		delete(s.sessions, id)
	}
	s.mu.Unlock()
}

// Open registers a new SSE client connection for a node, replacing (and
// closing) any previous session of that node.
func (s *SseServer) Open(nodeID string, w http.ResponseWriter, subscriptions ...SyncData) (*SseSession, error) {
	if strings.TrimSpace(nodeID) == "" {
		return nil, fmt.Errorf("unknown client")
	}
	flusher, _ := w.(http.Flusher)
	rc := http.NewResponseController(w)
	if flusher == nil {
		// ResponseController may still reach a Flusher through Unwrap.
		if err := rc.Flush(); err != nil {
			return nil, fmt.Errorf("streaming not supported")
		}
	}

	session := &SseSession{
		NodeID:     nodeID,
		Writer:     w,
		Flusher:    flusher,
		Done:       make(chan struct{}),
		Tables:     subscriptions,
		rc:         rc,
		LastActive: time.Now(),
	}

	s.mu.Lock()
	old := s.sessions[nodeID]
	s.sessions[nodeID] = session
	s.mu.Unlock()
	if old != nil {
		old.close()
	}

	s.log.Info("SSE session opened", zap.String("node_id", nodeID), zap.Int("tables", len(subscriptions)))
	return session, nil
}

// Session returns the active session of a node.
func (s *SseServer) Session(nodeID string) (*SseSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[nodeID]
	return sess, ok
}

// NodeIDs returns the ids of all connected nodes.
func (s *SseServer) NodeIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		out = append(out, id)
	}
	return out
}

// Subscriptions returns the subscribed tables of a connected node.
func (s *SseServer) Subscriptions(nodeID string) []SyncData {
	sess, ok := s.Session(nodeID)
	if !ok {
		return nil
	}
	return sess.Tables
}

// IsSubscribed reports whether nodeID is connected and subscribed to tableName.
func (s *SseServer) IsSubscribed(nodeID, tableName string) bool {
	sess, ok := s.Session(nodeID)
	return ok && sess.Subscribed(tableName)
}

// sseFrame renders a data event: id=<tableName>, data=<json> (Java
// SseSession.send).
func sseFrame(data *SyncData) (string, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	id := strings.NewReplacer("\n", "", "\r", "").Replace(data.TableName)
	return fmt.Sprintf("id:%s\ndata:%s\n\n", id, payload), nil
}

// SendE pushes a sync data message to a specific node and returns the error.
// A write failure closes the session.
func (s *SseServer) SendE(nodeID string, data *SyncData) error {
	sess, ok := s.Session(nodeID)
	if !ok {
		return fmt.Errorf("sse client %s not found", nodeID)
	}
	frame, err := sseFrame(data)
	if err != nil {
		return err
	}
	if err := sess.write(frame); err != nil {
		s.log.Warn("SSE write failed, closing session", zap.String("node_id", nodeID), zap.Error(err))
		s.closeSession(sess)
		return err
	}
	return nil
}

// Send pushes a sync data message to a specific node.
func (s *SseServer) Send(nodeID string, data *SyncData) bool {
	return s.SendE(nodeID, data) == nil
}

// Broadcast sends a sync data message to all connected nodes.
func (s *SseServer) Broadcast(data *SyncData) int {
	count := 0
	for _, nodeID := range s.NodeIDs() {
		if s.Send(nodeID, data) {
			count++
		}
	}
	return count
}

// closeSession removes sess only if it is still the registered session of
// its node (a newer session of the same node is kept).
func (s *SseServer) closeSession(sess *SseSession) {
	s.mu.Lock()
	if cur, ok := s.sessions[sess.NodeID]; ok && cur == sess {
		delete(s.sessions, sess.NodeID)
	}
	s.mu.Unlock()
	sess.close()
}

// Close terminates the SSE session of a node.
func (s *SseServer) Close(nodeID string) bool {
	s.mu.Lock()
	session, exists := s.sessions[nodeID]
	if exists {
		delete(s.sessions, nodeID)
	}
	s.mu.Unlock()

	if exists {
		session.close()
		s.log.Info("SSE session closed", zap.String("node_id", nodeID))
	}
	return exists
}

// Ping sends a heartbeat to all connected clients (Java SseSession.ping:
// id=ping, comment=ping, data=ping). Failing sessions are closed.
func (s *SseServer) Ping() {
	s.mu.RLock()
	sessions := make([]*SseSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.RUnlock()

	frame := "id:" + SsePingMsg + "\n: " + SsePingMsg + "\ndata:" + SsePingMsg + "\n\n"
	for _, session := range sessions {
		if err := session.write(frame); err != nil {
			s.log.Warn("SSE ping failed, closing session", zap.String("node_id", session.NodeID), zap.Error(err))
			s.closeSession(session)
		}
	}
}

// ActiveConnections returns the number of active SSE connections.
func (s *SseServer) ActiveConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// heartbeatLoop periodically sends ping to keep connections alive.
func (s *SseServer) heartbeatLoop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.Ping()
		}
	}
}

// ParseSubscriptions decodes the `p` query parameter: a (possibly
// URL-encoded) JSON list of {tableName, lastUpdateTime}.
func ParseSubscriptions(p string) ([]SyncData, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil, nil
	}
	var subs []SyncData
	if err := json.Unmarshal([]byte(p), &subs); err == nil {
		return subs, nil
	}
	dec, err := url.QueryUnescape(p)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(dec), &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

func (s *SseServer) filterSubscriptions(subs []SyncData) []SyncData {
	s.hookMu.RLock()
	f := s.tableFilter
	s.hookMu.RUnlock()
	out := make([]SyncData, 0, len(subs))
	seen := map[string]bool{}
	for _, sub := range subs {
		name := strings.TrimSpace(sub.TableName)
		if f != nil {
			canon, ok := f(name)
			if !ok {
				s.log.Warn("SSE subscription ignored: table not synchronized", zap.String("table", name))
				continue
			}
			name = canon
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		lut := strings.TrimSpace(sub.LastUpdateTime)
		if lut == "" {
			lut = "0"
		}
		out = append(out, SyncData{TableName: name, LastUpdateTime: lut})
	}
	return out
}

// HandleSseSync is GET /sync (Java CenterDataSyncController.sync): the edge
// identified by kuscia-origin-source subscribes with ?p=[{tableName,
// lastUpdateTime}], receives the incremental data since lastUpdateTime (open
// hook), then every change of those tables (dispatcher) and a ping every 15s.
//
// The http.Server read/write timeouts are lifted for this stream through
// http.ResponseController; each write gets its own 30s deadline.
func (s *SseServer) HandleSseSync(c *gin.Context) {
	nodeID := strings.TrimSpace(c.GetHeader("kuscia-origin-source"))
	if nodeID == "" {
		response.FailArgs(c, errcode.SystemSseError, "Unknown client")
		return
	}
	subs, err := ParseSubscriptions(c.Query("p"))
	if err != nil {
		response.FailArgs(c, errcode.SystemSseError, "invalid subscription parameter p")
		return
	}
	subs = s.filterSubscriptions(subs)

	rc := http.NewResponseController(c.Writer)
	// Lift the server-level deadlines: the stream lives until the client leaves.
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	session, err := s.Open(nodeID, c.Writer, subs...)
	if err != nil {
		response.FailArgs(c, errcode.SystemSseError, err.Error())
		return
	}
	// Commit headers immediately so the client sees the stream open.
	_ = session.write(": connected\n\n")

	s.hookMu.RLock()
	hook := s.onOpen
	s.hookMu.RUnlock()
	if hook != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.Error("SSE open hook panic", zap.Any("panic", r))
				}
			}()
			hook(nodeID, subs)
		}()
	}

	select {
	case <-c.Request.Context().Done():
	case <-session.Done:
	case <-s.stopCh:
	}
	s.closeSession(session)
	s.log.Info("SSE session finished", zap.String("node_id", nodeID))
}
