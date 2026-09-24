package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/gin-gonic/gin"
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

// --- Bug 47: MiscHandler.ListInstNodes error handling ---

func TestInstHandler_ListInstNodes_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	ns := service.NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), nil, db)
	h := NewInstHandler(db, service.NewInstService(db, ns, t.TempDir(), "", nil))
	r := setupTestRouter()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), &auth.UserContext{Name: "u", OwnerID: "inst1"}))
	})
	r.POST("/inst/node/list", h.ListInstNodes)

	// Seed inst + nodes (only nodes of the login inst are listed)
	db.Create(&model.InstDO{InstID: "inst1", Name: "Inst 1"})
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice", Type: "normal", InstID: "inst1"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", Type: "normal", InstID: "inst1"})
	db.Create(&model.NodeDO{NodeID: "carol", Name: "Carol", ControlNodeID: "carol", Type: "normal", InstID: "other"})

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
