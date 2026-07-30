package v1

import (
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NodeRouteHandler handles node route HTTP requests.
type NodeRouteHandler struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
}

// NewNodeRouteHandler creates a new NodeRouteHandler.
func NewNodeRouteHandler(db *gorm.DB, kusciaClient *kuscia.Client) *NodeRouteHandler {
	return &NodeRouteHandler{db: db, kusciaClient: kusciaClient}
}

// NodeRouterVO represents a node route view object.
type NodeRouterVO struct {
	RouteID       string `json:"route_id"`
	SrcNodeID     string `json:"src_node_id"`
	DstNodeID     string `json:"dst_node_id"`
	SrcNetAddress string `json:"src_net_address"`
	DstNetAddress string `json:"dst_net_address"`
	GmtCreate     string `json:"gmt_create"`
}

// Page handles node route pagination.
func (h *NodeRouteHandler) Page(c *gin.Context) {
	var req struct {
		Page   int    `json:"page"`
		Size   int    `json:"size"`
		NodeID string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Page = 1
		req.Size = 10
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 10
	}

	query := h.db.WithContext(c.Request.Context()).Model(&model.NodeRouteDO{})
	if req.NodeID != "" {
		query = query.Where("src_node_id = ? OR dst_node_id = ?", req.NodeID, req.NodeID)
	}

	// Bug62 fix: check Count and Find errors instead of silently ignoring them.
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	var routes []model.NodeRouteDO
	if err := query.Order("gmt_create DESC").Offset((req.Page - 1) * req.Size).Limit(req.Size).Find(&routes).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	result := make([]NodeRouterVO, 0, len(routes))
	for _, r := range routes {
		result = append(result, NodeRouterVO{
			RouteID:       r.RouteID,
			SrcNodeID:     r.SrcNodeID,
			DstNodeID:     r.DstNodeID,
			SrcNetAddress: r.SrcNetAddress,
			DstNetAddress: r.DstNetAddress,
			GmtCreate:     r.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}

	response.OK(c, gin.H{
		"data":  result,
		"total": total,
		"page":  req.Page,
		"size":  req.Size,
	})
}

// Get handles node route detail retrieval.
func (h *NodeRouteHandler) Get(c *gin.Context) {
	var req struct {
		RouterID string `json:"router_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	var route model.NodeRouteDO
	if err := h.db.WithContext(c.Request.Context()).Where("route_id = ?", req.RouterID).First(&route).Error; err != nil {
		response.Fail(c, errcode.RouteNotFound)
		return
	}

	response.OK(c, NodeRouterVO{
		RouteID:       route.RouteID,
		SrcNodeID:     route.SrcNodeID,
		DstNodeID:     route.DstNodeID,
		SrcNetAddress: route.SrcNetAddress,
		DstNetAddress: route.DstNetAddress,
		GmtCreate:     route.GmtCreate.Format("2006-01-02 15:04:05"),
	})
}

// Update handles node route update.
func (h *NodeRouteHandler) Update(c *gin.Context) {
	var req struct {
		RouterID      string `json:"router_id" binding:"required"`
		DstNetAddress string `json:"dst_net_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	var route model.NodeRouteDO
	if err := h.db.WithContext(c.Request.Context()).Where("route_id = ?", req.RouterID).First(&route).Error; err != nil {
		response.Fail(c, errcode.RouteNotFound)
		return
	}

	if req.DstNetAddress != "" {
		route.DstNetAddress = req.DstNetAddress
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&route).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, req.RouterID)
}

// ListNode handles listing available nodes for route creation.
func (h *NodeRouteHandler) ListNode(c *gin.Context) {
	var nodes []model.NodeDO
	// Bug63 fix: check the DB error instead of silently ignoring it.
	if err := h.db.WithContext(c.Request.Context()).Find(&nodes).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	result := make([]gin.H, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, gin.H{
			"node_id": n.NodeID,
			"name":    n.Name,
			"type":    n.Type,
		})
	}

	response.OK(c, result)
}

// Refresh handles node route status refresh via Kuscia DomainRoute query.
func (h *NodeRouteHandler) Refresh(c *gin.Context) {
	var req struct {
		RouterID string `json:"router_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	var route model.NodeRouteDO
	if err := h.db.WithContext(c.Request.Context()).Where("route_id = ?", req.RouterID).First(&route).Error; err != nil {
		response.Fail(c, errcode.RouteNotFound)
		return
	}

	// Query Kuscia DomainRoute for actual route status
	status := "Ready"
	if h.kusciaClient != nil {
		resp, err := h.kusciaClient.QueryDomainRoute(c.Request.Context(), route.SrcNodeID, route.DstNodeID)
		if err != nil {
			status = "NotReady"
		} else {
			status = resp.Data.Status
		}
	}

	response.OK(c, gin.H{
		"route_id":        route.RouteID,
		"src_node_id":     route.SrcNodeID,
		"dst_node_id":     route.DstNodeID,
		"src_net_address": route.SrcNetAddress,
		"dst_net_address": route.DstNetAddress,
		"status":          status,
		"gmt_create":      route.GmtCreate.Format("2006-01-02 15:04:05"),
	})
}

// Delete handles node route deletion both locally and in Kuscia.
func (h *NodeRouteHandler) Delete(c *gin.Context) {
	var req struct {
		RouterID string `json:"router_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Find route first to get src/dst for Kuscia deletion
	var route model.NodeRouteDO
	if err := h.db.WithContext(c.Request.Context()).Where("route_id = ?", req.RouterID).First(&route).Error; err != nil {
		response.Fail(c, errcode.RouteNotFound)
		return
	}

	// Delete in Kuscia (best-effort)
	if h.kusciaClient != nil {
		_ = h.kusciaClient.DeleteDomainRoute(c.Request.Context(), route.SrcNodeID, route.DstNodeID)
	}

	// Delete locally
	// Bug64 fix: check the Delete error instead of silently ignoring it.
	if err := h.db.WithContext(c.Request.Context()).Delete(&route).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}
	response.OKEmpty(c)
}
