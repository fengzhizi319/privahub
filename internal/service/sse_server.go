package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SyncDataDTO represents a data synchronization message between center and edge.
type SyncDataDTO struct {
	TableName      string      `json:"table_name"`
	Action         string      `json:"action"` // INSERT, UPDATE, DELETE
	Data           interface{} `json:"data"`
	LastUpdateTime string      `json:"last_update_time"`
}

// SseSession represents a single SSE client connection.
type SseSession struct {
	NodeID     string
	Writer     http.ResponseWriter
	Flusher    http.Flusher
	Done       chan struct{}
	mu         sync.Mutex // guards LastActive against concurrent writes
	LastActive time.Time
}

// SseServer manages SSE connections for center-edge data synchronization.
type SseServer struct {
	mu       sync.RWMutex
	sessions map[string]*SseSession
	log      *zap.Logger
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewSseServer creates a new SSE server with heartbeat.
func NewSseServer(log *zap.Logger) *SseServer {
	if log == nil {
		log = zap.NewNop()
	}
	s := &SseServer{
		sessions: make(map[string]*SseSession),
		log:      log,
		interval: 15 * time.Second,
		stopCh:   make(chan struct{}),
	}
	go s.heartbeatLoop()
	return s
}

// Stop gracefully shuts down the SSE server, closing all sessions and
// stopping the heartbeat goroutine. Safe to call multiple times.
func (s *SseServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.mu.Lock()
	for id, session := range s.sessions {
		close(session.Done)
		delete(s.sessions, id)
	}
	s.mu.Unlock()
}

// Open registers a new SSE client connection for a node.
func (s *SseServer) Open(nodeID string, w http.ResponseWriter) (*SseSession, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	// Close existing session for this node if any
	s.Close(nodeID)

	session := &SseSession{
		NodeID:     nodeID,
		Writer:     w,
		Flusher:    flusher,
		Done:       make(chan struct{}),
		LastActive: time.Now(),
	}

	s.mu.Lock()
	s.sessions[nodeID] = session
	s.mu.Unlock()

	s.log.Info("SSE session opened", zap.String("node_id", nodeID))
	return session, nil
}

// Send pushes a sync data message to a specific node.
func (s *SseServer) Send(nodeID string, data *SyncDataDTO) bool {
	s.mu.RLock()
	session, exists := s.sessions[nodeID]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	payload, err := json.Marshal(data)
	if err != nil {
		s.log.Error("SSE marshal error", zap.Error(err))
		return false
	}

	_, err = fmt.Fprintf(session.Writer, "data: %s\n\n", payload)
	if err != nil {
		s.log.Warn("SSE write failed, closing session", zap.String("node_id", nodeID), zap.Error(err))
		s.Close(nodeID)
		return false
	}
	session.Flusher.Flush()
	session.mu.Lock()
	session.LastActive = time.Now()
	session.mu.Unlock()
	return true
}

// Broadcast sends a sync data message to all connected nodes.
func (s *SseServer) Broadcast(data *SyncDataDTO) int {
	s.mu.RLock()
	nodeIDs := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		nodeIDs = append(nodeIDs, id)
	}
	s.mu.RUnlock()

	count := 0
	for _, nodeID := range nodeIDs {
		if s.Send(nodeID, data) {
			count++
		}
	}
	return count
}

// Close terminates an SSE session for a node.
func (s *SseServer) Close(nodeID string) bool {
	s.mu.Lock()
	session, exists := s.sessions[nodeID]
	if exists {
		delete(s.sessions, nodeID)
	}
	s.mu.Unlock()

	if exists {
		close(session.Done)
		s.log.Info("SSE session closed", zap.String("node_id", nodeID))
	}
	return exists
}

// Ping sends heartbeat comments to all connected clients.
func (s *SseServer) Ping() {
	s.mu.RLock()
	sessions := make([]*SseSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.RUnlock()

	for _, session := range sessions {
		_, err := fmt.Fprintf(session.Writer, ": ping\n\n")
		if err != nil {
			s.Close(session.NodeID)
			continue
		}
		session.Flusher.Flush()
		session.mu.Lock()
		session.LastActive = time.Now()
		session.mu.Unlock()
	}
}

// ActiveConnections returns the number of active SSE connections.
func (s *SseServer) ActiveConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// heartbeatLoop periodically sends ping to keep connections alive.
// It terminates when the server's stop channel is closed.
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

// HandleSseSync is the HTTP handler for SSE sync endpoint.
// Edge nodes connect to this endpoint to receive real-time data changes.
func (s *SseServer) HandleSseSync(c *gin.Context) {
	nodeID := c.GetHeader("kuscia-origin-source")
	if nodeID == "" {
		nodeID = c.Query("node_id")
	}
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": gin.H{"code": 202011501, "msg": "node_id is required"},
		})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	session, err := s.Open(nodeID, c.Writer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": gin.H{"code": 202011500, "msg": "streaming not supported"},
		})
		return
	}

	// Send initial connection event (use json.Marshal to prevent JSON injection)
	connPayload, _ := json.Marshal(map[string]string{"node_id": nodeID})
	fmt.Fprintf(session.Writer, "event: connected\ndata: %s\n\n", connPayload)
	session.Flusher.Flush()

	// Block until client disconnects
	<-c.Request.Context().Done()
	s.Close(nodeID)
}
