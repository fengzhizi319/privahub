package v1

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	datasync "github.com/fengzhizi319/privahub/internal/service/sync"
)

func newSyncHandlerEnv(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:synch_%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := migration.AutoMigrate(db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	reg := datasync.DefaultRegistry()
	tables, _ := reg.ResolveTables(datasync.DefaultP2PTables)
	h := NewSyncHandler(datasync.NewVoteSyncService(db, reg, nil), datasync.NewConsumer(db, reg, tables, "alice", nil), nil)
	r := gin.New()
	r.POST("/api/v1alpha1/vote_sync/create", h.VoteSyncCreate)
	r.POST("/api/v1alpha1/data/sync", h.DataSync)
	return db, r
}

func syncPostJSON(r http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("kuscia-origin-source", "bob")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSyncHandler_VoteSyncCreate(t *testing.T) {
	db, r := newSyncHandlerEnv(t)
	w := syncPostJSON(r, "/api/v1alpha1/vote_sync/create", `{"dbSyncRequests":[
		{"syncDataType":"PROJECT","projectNodesInfo":{"projectId":"p1","name":"n","ownerId":"alice"}},
		{"syncDataType":"NODE_ROUTE","projectNodesInfo":{"routeId":"r","srcNodeId":"alice","dstNodeId":"bob"}}]}`)
	if !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatalf("resp %s", w.Body.String())
	}
	var n int64
	db.Model(&model.ProjectDO{}).Count(&n)
	if n != 1 {
		t.Fatalf("project rows %d", n)
	}
	for _, bad := range []string{`{"dbSyncRequests":[]}`, `{`, `{"dbSyncRequests":[{"syncDataType":"USER","projectNodesInfo":{}}]}`} {
		w = syncPostJSON(r, "/api/v1alpha1/vote_sync/create", bad)
		if !strings.Contains(w.Body.String(), "202011100") {
			t.Fatalf("%s -> %s", bad, w.Body.String())
		}
	}
}

func TestSyncHandler_DataSync(t *testing.T) {
	db, r := newSyncHandlerEnv(t)
	w := syncPostJSON(r, "/api/v1alpha1/data/sync", `{"tableName":"org.secretflow.secretpad.persistence.entity.ProjectGraphDO","action":"create","data":{"upk":{"projectId":"p","graphId":"g"},"name":"graph","ownerId":"instB"}}`)
	if !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatalf("resp %s", w.Body.String())
	}
	var g model.ProjectGraphDO
	if db.Where("project_id = ? AND graph_id = ?", "p", "g").First(&g).Error != nil || g.Name != "graph" {
		t.Fatalf("graph not stored: %+v", g)
	}
	w = syncPostJSON(r, "/api/v1alpha1/data/sync", `{"tableName":"project_graph","action":"remove","data":{"project_id":"p","graph_id":"g"}}`)
	var n int64
	db.Model(&model.ProjectGraphDO{}).Count(&n)
	if !strings.Contains(w.Body.String(), `"code":0`) || n != 0 {
		t.Fatalf("delete: %s n=%d", w.Body.String(), n)
	}
	w = syncPostJSON(r, "/api/v1alpha1/data/sync", `{"tableName":"user_accounts","action":"create","data":{"name":"x"}}`)
	if !strings.Contains(w.Body.String(), "202011100") {
		t.Fatalf("user_accounts must be rejected: %s", w.Body.String())
	}
}
