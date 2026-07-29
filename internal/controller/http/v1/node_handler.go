package v1

import (
	"time"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
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

	response.OK(c, gin.H{"list": nodes, "nodes": nodes, "total": len(nodes)})
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

	response.OK(c, gin.H{
		"token":                token,
		"token_status":         "Available",
		"last_transition_time": time.Now().Format("2006-01-02 15:04:05"),
	})
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

// ResultList handles node result list retrieval.
func (h *NodeHandler) ResultList(c *gin.Context) {
	var req service.ListNodeResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.ListNodeResultRequest{}
	}

	vo, err := h.nodeService.ListNodeResults(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// ResultDetail handles node result detail retrieval.
func (h *NodeHandler) ResultDetail(c *gin.Context) {
	var req service.GetNodeResultDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.nodeService.GetNodeResultDetail(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}
