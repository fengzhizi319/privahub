package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// NodeHandler handles node-related HTTP requests.
type NodeHandler struct {
	nodeService *service.NodeService
}

// NewNodeHandler creates a new NodeHandler.
func NewNodeHandler(nodeService *service.NodeService) *NodeHandler {
	return &NodeHandler{nodeService: nodeService}
}

// Create handles node creation.
func (h *NodeHandler) Create(c *gin.Context) {
	var req service.CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	node, err := h.nodeService.CreateNode(c.Request.Context(), &req)
	if err != nil {
		switch err {
		case service.ErrNodeAlreadyExists:
			response.FailWithMsg(c, errcode.AlreadyExists, "node already exists")
		default:
			response.Fail(c, errcode.SystemError)
		}
		return
	}

	response.OK(c, node)
}

// Update handles node update.
func (h *NodeHandler) Update(c *gin.Context) {
	var req service.UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.nodeService.UpdateNode(c.Request.Context(), &req); err != nil {
		if err == service.ErrNodeNotFound {
			response.FailWithMsg(c, errcode.NotFound, "node not found")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Get handles node detail retrieval.
func (h *NodeHandler) Get(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	node, err := h.nodeService.GetNode(c.Request.Context(), req.NodeID)
	if err != nil {
		response.FailWithMsg(c, errcode.NotFound, "node not found")
		return
	}

	response.OK(c, node)
}

// List handles node list retrieval.
func (h *NodeHandler) List(c *gin.Context) {
	nodes, err := h.nodeService.ListNodes(c.Request.Context())
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"nodes": nodes})
}

// Delete handles node deletion.
func (h *NodeHandler) Delete(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.nodeService.DeleteNode(c.Request.Context(), req.NodeID); err != nil {
		if err == service.ErrNodeNotFound {
			response.FailWithMsg(c, errcode.NotFound, "node not found")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Token handles node deployment token generation.
func (h *NodeHandler) Token(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	token, err := h.nodeService.GenerateToken(c.Request.Context(), req.NodeID)
	if err != nil {
		if err == service.ErrNodeNotFound {
			response.FailWithMsg(c, errcode.NotFound, "node not found")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"token": token})
}

// CreateRoute handles route creation.
func (h *NodeHandler) CreateRoute(c *gin.Context) {
	var req service.CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.nodeService.CreateRoute(c.Request.Context(), &req); err != nil {
		if err == service.ErrRouteAlreadyExists {
			response.FailWithMsg(c, errcode.AlreadyExists, "route already exists")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// ListRoutes handles route list retrieval.
func (h *NodeHandler) ListRoutes(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	routes, err := h.nodeService.ListRoutes(c.Request.Context(), req.NodeID)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"routes": routes})
}

// DeleteRoute handles route deletion.
func (h *NodeHandler) DeleteRoute(c *gin.Context) {
	var req struct {
		SrcNodeID string `json:"src_node_id" binding:"required"`
		DstNodeID string `json:"dst_node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.nodeService.DeleteRoute(c.Request.Context(), req.SrcNodeID, req.DstNodeID); err != nil {
		if err == service.ErrRouteNotFound {
			response.FailWithMsg(c, errcode.NotFound, "route not found")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Refresh handles node status refresh.
func (h *NodeHandler) Refresh(c *gin.Context) {
	var req struct {
		NodeID    string `json:"nodeId"`
		NodeIDAlt string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.NodeID = ""
	}
	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = req.NodeIDAlt
	}

	node, err := h.nodeService.RefreshNode(c.Request.Context(), nodeID)
	if err != nil {
		if err == service.ErrNodeNotFound {
			response.FailWithMsg(c, errcode.NotFound, "node not found")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, node)
}
