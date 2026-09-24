package v1

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// --- Bug 62: NodeRouteHandler.Page error propagation ---

func TestNodeRouteHandler_Page_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewNodeRouteHandler(db, nil)
	r := setupTestRouter()
	r.POST("/page", h.Page)

	// Seed routes
	db.Create(&model.NodeRouteDO{
		RouteID:       "route-1",
		SrcNodeID:     "alice",
		DstNodeID:     "bob",
		SrcNetAddress: "alice:8080",
		DstNetAddress: "bob:8080",
	})
	db.Create(&model.NodeRouteDO{
		RouteID:       "route-2",
		SrcNodeID:     "bob",
		DstNodeID:     "carol",
		SrcNetAddress: "bob:8080",
		DstNetAddress: "carol:8080",
	})

	w := postJSON(r, "/page", map[string]interface{}{"page": 1, "size": 10})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
		Data struct {
			Total int64 `json:"total"`
			Data  []struct {
				RouteID string `json:"routeId"`
			} `json:"data"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Data.Total)
	}
	if len(resp.Data.Data) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(resp.Data.Data))
	}
}

func TestNodeRouteHandler_Page_WithNodeFilter(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewNodeRouteHandler(db, nil)
	r := setupTestRouter()
	r.POST("/page", h.Page)

	db.Create(&model.NodeRouteDO{RouteID: "route-1", SrcNodeID: "alice", DstNodeID: "bob"})
	db.Create(&model.NodeRouteDO{RouteID: "route-2", SrcNodeID: "bob", DstNodeID: "carol"})
	db.Create(&model.NodeRouteDO{RouteID: "route-3", SrcNodeID: "carol", DstNodeID: "dave"})

	w := postJSON(r, "/page", map[string]interface{}{"page": 1, "size": 10, "node_id": "bob"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// bob appears in route-1 (dst) and route-2 (src)
	if resp.Data.Total != 2 {
		t.Fatalf("expected total 2 for bob filter, got %d", resp.Data.Total)
	}
}

// --- Bug 63: NodeRouteHandler.ListNode error propagation ---

func TestNodeRouteHandler_ListNode_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewNodeRouteHandler(db, nil)
	r := setupTestRouter()
	r.POST("/listnode", h.ListNode)

	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice Node", Type: "normal"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "Bob Node", Type: "normal"})

	w := postJSON(r, "/listnode", map[string]interface{}{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []struct {
			NodeID string `json:"nodeId"`
			Name   string `json:"nodeName"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(resp.Data))
	}
}

// --- Bug 64: NodeRouteHandler.Delete error propagation ---

func TestNodeRouteHandler_Delete_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewNodeRouteHandler(db, nil)
	r := setupTestRouter()
	r.POST("/delete", h.Delete)

	db.Create(&model.NodeRouteDO{RouteID: "route-1", SrcNodeID: "alice", DstNodeID: "bob"})

	w := postJSON(r, "/delete", map[string]interface{}{"router_id": "route-1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify route was deleted
	var count int64
	db.Model(&model.NodeRouteDO{}).Where("route_id = ?", "route-1").Count(&count)
	if count != 0 {
		t.Fatalf("expected route to be deleted, but found %d", count)
	}
}

func TestNodeRouteHandler_Delete_NotFound(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewNodeRouteHandler(db, nil)
	r := setupTestRouter()
	r.POST("/delete", h.Delete)

	w := postJSON(r, "/delete", map[string]interface{}{"router_id": "nonexistent"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Should return error code for not found
	if resp.Status.Code == 0 {
		t.Fatal("expected error code for nonexistent route")
	}
}
