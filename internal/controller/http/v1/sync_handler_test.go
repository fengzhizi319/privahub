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
	// The sending node bob belongs to institution instB.
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", InstID: "instB"})
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

// TestSyncHandler_DataSyncOwnership: a peer may only write rows it owns
// (reverse of the P2P producer filter); rejected writes change nothing and
// answer a generic AUTH_FAILED.
func TestSyncHandler_DataSyncOwnership(t *testing.T) {
	db, r := newSyncHandlerEnv(t)
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", InstID: "instB"})
	db.Create(&model.NodeDO{NodeID: "bob2", Name: "Bob2", ControlNodeID: "bob2", InstID: "instB"})
	db.Create(&model.ProjectGraphDO{ProjectID: "p", GraphID: "victim", Name: "orig", OwnerID: "instC"})
	db.Create(&model.ProjectGraphDO{ProjectID: "p", GraphID: "bobs", Name: "orig", OwnerID: "instB"})
	denied := func(body string) {
		t.Helper()
		w := syncPostJSON(r, "/api/v1alpha1/data/sync", body)
		if !strings.Contains(w.Body.String(), "202011602") {
			t.Errorf("expected AUTH_FAILED for %s, got %s", body, w.Body.String())
		}
	}
	// Foreign owner in the payload.
	denied(`{"tableName":"project_graph","action":"create","data":{"project_id":"p","graph_id":"new","name":"x","owner_id":"instC"}}`)
	// Claiming ownership of an existing foreign row (overwrite by business key).
	denied(`{"tableName":"project_graph","action":"update","data":{"project_id":"p","graph_id":"victim","name":"pwned","owner_id":"instB"}}`)
	// Deleting a foreign row.
	denied(`{"tableName":"project_graph","action":"delete","data":{"project_id":"p","graph_id":"victim"}}`)
	// Datatable of another institution's node.
	denied(`{"tableName":"project_datatable","action":"create","data":{"project_id":"p","node_id":"carol","datatable_id":"d"}}`)
	// Graph node of a graph bob does not own.
	denied(`{"tableName":"project_graph_node","action":"create","data":{"project_id":"p","graph_id":"victim","graph_node_id":"n1"}}`)
	var g model.ProjectGraphDO
	db.Where("graph_id = ?", "victim").First(&g)
	if g.Name != "orig" || g.OwnerID != "instC" {
		t.Fatalf("victim graph modified: %+v", g)
	}
	// Own rows are accepted: own graph, datatable of another node of instB.
	for _, ok := range []string{
		`{"tableName":"project_graph","action":"update","data":{"project_id":"p","graph_id":"bobs","name":"new","owner_id":"instB"}}`,
		`{"tableName":"project_datatable","action":"create","data":{"project_id":"p","node_id":"bob2","datatable_id":"d"}}`,
		`{"tableName":"project_graph_node","action":"create","data":{"project_id":"p","graph_id":"bobs","graph_node_id":"n1"}}`,
	} {
		if w := syncPostJSON(r, "/api/v1alpha1/data/sync", ok); !strings.Contains(w.Body.String(), `"code":0`) {
			t.Errorf("own row rejected: %s -> %s", ok, w.Body.String())
		}
	}
	// Missing origin is rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/data/sync", strings.NewReader(
		`{"tableName":"project_graph","action":"update","data":{"project_id":"p","graph_id":"bobs","name":"n","owner_id":"instB"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "202011602") {
		t.Errorf("missing origin must be rejected: %s", w.Body.String())
	}
}
