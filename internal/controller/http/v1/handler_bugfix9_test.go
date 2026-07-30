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
				RouteID string `json:"route_id"`
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
			NodeID string `json:"node_id"`
			Name   string `json:"name"`
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

// --- Bug 65: ApprovalHandler.PullStatus error propagation ---

func TestApprovalHandler_PullStatus_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewApprovalHandler(db)
	r := setupTestRouter()
	r.POST("/pullstatus", h.PullStatus)

	db.Create(&model.VoteRequestDO{
		VoteID:    "vote-1",
		Initiator: "alice",
		Voters:    "bob,carol",
		Type:      "PROJECT_CREATE",
		Status:    0,
	})
	db.Create(&model.VoteInviteDO{
		VoteID:            "vote-1",
		Initiator:         "alice",
		VoteParticipantID: "bob",
		Action:            "AGREE",
	})
	db.Create(&model.VoteInviteDO{
		VoteID:            "vote-1",
		Initiator:         "alice",
		VoteParticipantID: "carol",
		Action:            "REVIEWING",
	})

	w := postJSON(r, "/pullstatus", map[string]interface{}{"vote_id": "vote-1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			VoteID  string `json:"vote_id"`
			Status  string `json:"status"`
			Invites []struct {
				Voter  string `json:"voter"`
				Action string `json:"action"`
			} `json:"invites"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.VoteID != "vote-1" {
		t.Fatalf("expected vote-1, got %s", resp.Data.VoteID)
	}
	if resp.Data.Status != "PENDING" {
		t.Fatalf("expected PENDING, got %s", resp.Data.Status)
	}
	if len(resp.Data.Invites) != 2 {
		t.Fatalf("expected 2 invites, got %d", len(resp.Data.Invites))
	}
}

// --- Bug 66-68: MessageHandler error propagation ---

func TestMessageHandler_List_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/list", h.List)

	db.Create(&model.VoteRequestDO{
		VoteID:      "vote-1",
		Initiator:   "alice",
		Type:        "PROJECT_CREATE",
		Status:      0,
		Description: "Test vote",
	})
	db.Create(&model.VoteRequestDO{
		VoteID:      "vote-2",
		Initiator:   "bob",
		Type:        "NODE_CREATE",
		Status:      1,
		Description: "Another vote",
	})

	w := postJSON(r, "/list", map[string]interface{}{"page": 1, "size": 10})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			Total int64 `json:"total"`
			List  []struct {
				VoteID string `json:"vote_id"`
				Status string `json:"status"`
			} `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Total != 2 {
		t.Fatalf("expected total 2, got %d", resp.Data.Total)
	}
	if len(resp.Data.List) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(resp.Data.List))
	}
}

func TestMessageHandler_Detail_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/detail", h.Detail)

	db.Create(&model.VoteRequestDO{
		VoteID:      "vote-1",
		Initiator:   "alice",
		Voters:      "bob,carol",
		Type:        "PROJECT_CREATE",
		Status:      1,
		Description: "Test vote",
	})
	db.Create(&model.VoteInviteDO{
		VoteID:            "vote-1",
		VoteParticipantID: "bob",
		Action:            "AGREE",
		Reason:            "Looks good",
	})

	w := postJSON(r, "/detail", map[string]interface{}{"vote_id": "vote-1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data struct {
			VoteID  string `json:"vote_id"`
			Status  string `json:"status"`
			Invites []struct {
				Voter  string `json:"voter"`
				Action string `json:"action"`
				Reason string `json:"reason"`
			} `json:"invites"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Status != "APPROVED" {
		t.Fatalf("expected APPROVED, got %s", resp.Data.Status)
	}
	if len(resp.Data.Invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(resp.Data.Invites))
	}
	if resp.Data.Invites[0].Reason != "Looks good" {
		t.Fatalf("expected reason 'Looks good', got %s", resp.Data.Invites[0].Reason)
	}
}

func TestMessageHandler_Pending_Success(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewMessageHandler(db)
	r := setupTestRouter()
	r.POST("/pending", h.Pending)

	db.Create(&model.VoteInviteDO{VoteID: "vote-1", VoteParticipantID: "bob", Action: "REVIEWING"})
	db.Create(&model.VoteInviteDO{VoteID: "vote-2", VoteParticipantID: "bob", Action: "AGREE"})
	db.Create(&model.VoteInviteDO{VoteID: "vote-3", VoteParticipantID: "carol", Action: "REVIEWING"})

	// All pending
	w := postJSON(r, "/pending", map[string]interface{}{})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data int64 `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data != 2 {
		t.Fatalf("expected 2 pending, got %d", resp.Data)
	}

	// Filter by voter
	w = postJSON(r, "/pending", map[string]interface{}{"voter": "bob"})
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data != 1 {
		t.Fatalf("expected 1 pending for bob, got %d", resp.Data)
	}
}
