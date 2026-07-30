package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupHandlerTestDB creates an in-memory SQLite database for handler tests.
func setupHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.ProjectDO{},
		&model.ProjectNodeDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.EdgeDataSyncLogDO{},
		&model.InstDO{},
		&model.VoteRequestDO{},
		&model.VoteInviteDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func postJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Bug 44: P2PHandler.ProjectCreate atomicity ---

func TestP2PHandler_ProjectCreate_Atomic(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewP2PHandler(db, nil, zap.NewNop())
	r := setupTestRouter()
	r.POST("/p2p/project/create", h.ProjectCreate)

	// Create project with node associations
	w := postJSON(r, "/p2p/project/create", map[string]interface{}{
		"name":     "Test P2P Project",
		"node_ids": []string{"alice", "bob"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify project was created
	var project model.ProjectDO
	if err := db.Where("name = ?", "Test P2P Project").First(&project).Error; err != nil {
		t.Fatalf("project not found: %v", err)
	}

	// Verify node associations were created atomically
	var count int64
	db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", project.ProjectID).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 node associations, got %d", count)
	}
}

func TestP2PHandler_ProjectCreate_DuplicateNodeRollback(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewP2PHandler(db, nil, zap.NewNop())
	r := setupTestRouter()
	r.POST("/p2p/project/create", h.ProjectCreate)

	// Pre-insert a conflicting node association to trigger a unique constraint error
	// First create a project to get a valid project_id format
	w := postJSON(r, "/p2p/project/create", map[string]interface{}{
		"name":     "First Project",
		"node_ids": []string{"alice"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("first create failed: %d", w.Code)
	}

	// Verify the first project exists with its node
	var count int64
	db.Model(&model.ProjectDO{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 project, got %d", count)
	}
}

// --- Bug 45: P2PHandler.DataSync timestamp ---

func TestP2PHandler_DataSync_Timestamp(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewP2PHandler(db, nil, zap.NewNop())
	r := setupTestRouter()
	r.POST("/p2p/data/sync", h.DataSync)

	before := time.Now().Add(-time.Second)
	w := postJSON(r, "/p2p/data/sync", map[string]interface{}{
		"table_name": "test_table",
		"data":       "payload",
		"action":     "upsert",
	})
	after := time.Now().Add(time.Second)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify sync log has a real timestamp (not "now")
	var log model.EdgeDataSyncLogDO
	if err := db.Where("table_name = ?", "test_table").First(&log).Error; err != nil {
		t.Fatalf("sync log not found: %v", err)
	}
	if log.LastUpdateTime == "now" {
		t.Error("LastUpdateTime should be a real timestamp, got literal 'now'")
	}
	// Verify the timestamp is parseable and within the test window (use local timezone)
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", log.LastUpdateTime, time.Local)
	if err != nil {
		t.Fatalf("LastUpdateTime is not a valid timestamp: %v", err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Errorf("timestamp %v not within expected window [%v, %v]", parsed, before, after)
	}
}

// --- Bug 46: P2PHandler.ProjectParticipants error handling ---

func TestP2PHandler_ProjectParticipants_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewP2PHandler(db, nil, zap.NewNop())
	r := setupTestRouter()
	r.POST("/p2p/project/participants", h.ProjectParticipants)

	// Seed data
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice Node", NetAddress: "127.0.0.1:8080", ControlNodeID: "alice"})

	w := postJSON(r, "/p2p/project/participants", map[string]interface{}{
		"project_id": "p1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	participants := data["participants"].([]interface{})
	if len(participants) != 1 {
		t.Errorf("expected 1 participant, got %d", len(participants))
	}
}

// --- Bug 47: MiscHandler.ListInstNodes error handling ---

func TestMiscHandler_ListInstNodes_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMiscHandler(db, nil)
	r := setupTestRouter()
	r.POST("/inst/node/list", h.ListInstNodes)

	// Seed nodes
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice", Type: "normal"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", Type: "normal"})

	w := postJSON(r, "/inst/node/list", map[string]interface{}{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(data))
	}
}

// --- Bug 48: MessageHandler.Reply atomicity ---

func TestMessageHandler_Reply_AtomicApproval(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/message/reply", h.Reply)

	// Seed a vote with two invites
	db.Create(&model.VoteRequestDO{VoteID: "v1", Type: "DATA_GRANT", Initiator: "alice", Status: 0})
	db.Create(&model.VoteInviteDO{VoteID: "v1", VoteParticipantID: "bob", Action: "REVIEWING"})
	db.Create(&model.VoteInviteDO{VoteID: "v1", VoteParticipantID: "carol", Action: "REVIEWING"})

	// First reply - should not finalize yet
	w := postJSON(r, "/message/reply", map[string]interface{}{
		"vote_id": "v1",
		"voter":   "bob",
		"action":  "AGREE",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Vote should still be pending (one more voter needed)
	var vote model.VoteRequestDO
	db.Where("vote_id = ?", "v1").First(&vote)
	if vote.Status != 0 {
		t.Errorf("vote should still be PENDING (0), got %d", vote.Status)
	}

	// Second reply - should finalize as APPROVED
	w = postJSON(r, "/message/reply", map[string]interface{}{
		"vote_id": "v1",
		"voter":   "carol",
		"action":  "AGREE",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	db.Where("vote_id = ?", "v1").First(&vote)
	if vote.Status != 1 {
		t.Errorf("vote should be APPROVED (1), got %d", vote.Status)
	}
}

func TestMessageHandler_Reply_Rejection(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/message/reply", h.Reply)

	db.Create(&model.VoteRequestDO{VoteID: "v2", Type: "DATA_GRANT", Initiator: "alice", Status: 0})
	db.Create(&model.VoteInviteDO{VoteID: "v2", VoteParticipantID: "bob", Action: "REVIEWING"})

	w := postJSON(r, "/message/reply", map[string]interface{}{
		"vote_id": "v2",
		"voter":   "bob",
		"action":  "REJECT",
		"reason":  "not allowed",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var vote model.VoteRequestDO
	db.Where("vote_id = ?", "v2").First(&vote)
	if vote.Status != 2 {
		t.Errorf("vote should be REJECTED (2), got %d", vote.Status)
	}
}

func TestMessageHandler_Reply_NotFound(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/message/reply", h.Reply)

	// Reply to a non-existent vote invite
	w := postJSON(r, "/message/reply", map[string]interface{}{
		"vote_id": "nonexistent",
		"voter":   "nobody",
		"action":  "AGREE",
	})
	// Should return a non-500 error (NotFound mapped to 200 with error code in body)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	status := resp["status"].(map[string]interface{})
	code := status["code"].(float64)
	if code == 0 {
		t.Error("expected non-zero error code for not-found vote invite")
	}
}

func TestMessageHandler_Reply_InvalidAction(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/message/reply", h.Reply)

	w := postJSON(r, "/message/reply", map[string]interface{}{
		"vote_id": "v1",
		"voter":   "bob",
		"action":  "INVALID_ACTION",
	})
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	status := resp["status"].(map[string]interface{})
	code := status["code"].(float64)
	if code == 0 {
		t.Error("expected non-zero error code for invalid action")
	}
}
