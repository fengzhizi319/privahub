package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// NodeHandler handles node-related HTTP requests (Java NodeController).
type NodeHandler struct {
	nodeService *service.NodeService
}

// NewNodeHandler creates a new NodeHandler.
func NewNodeHandler(nodeService *service.NodeService) *NodeHandler {
	return &NodeHandler{nodeService: nodeService}
}

// Create handles node/create. Returns the server-generated nodeId (string).
func (h *NodeHandler) Create(c *gin.Context) {
	var req service.CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	nodeID, err := h.nodeService.CreateNode(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, nodeID)
}

// Update handles node/update (Java: netAddress).
func (h *NodeHandler) Update(c *gin.Context) {
	var req service.UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.nodeService.UpdateNode(nodeReqCtx(c), &req); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, firstNonEmptyStr(req.NodeID, req.NodeIDAlt))
}

// Get handles node/get.
func (h *NodeHandler) Get(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	node, err := h.nodeService.GetNode(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, node)
}

// List handles node/list: full NodeVO list (Java returns a bare array).
func (h *NodeHandler) List(c *gin.Context) {
	nodes, err := h.nodeService.ListNodes(nodeReqCtx(c))
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, nodes)
}

// Page handles node/page: {list, total}.
func (h *NodeHandler) Page(c *gin.Context) {
	var req service.PageNodeRequest
	_ = c.ShouldBindJSON(&req)
	page, err := h.nodeService.PageNodes(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, page)
}

// Delete handles node/delete.
func (h *NodeHandler) Delete(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.nodeService.DeleteNode(nodeReqCtx(c), req.id()); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// Token handles node/token: the Kuscia deploy token (no refresh).
func (h *NodeHandler) Token(c *gin.Context) { h.token(c, false) }

// NewToken handles node/newToken: refresh from Kuscia's unused deploy tokens.
func (h *NodeHandler) NewToken(c *gin.Context) { h.token(c, true) }

func (h *NodeHandler) token(c *gin.Context, refresh bool) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.nodeService.GetNodeToken(nodeReqCtx(c), req.id(), refresh)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// CreateRoute handles Go-only node/route/create (delegates to NodeRouteService).
func (h *NodeHandler) CreateRoute(c *gin.Context) {
	var req service.CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.nodeService.CreateRoute(nodeReqCtx(c), &req); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// ListRoutes handles Go-only node/route/list.
func (h *NodeHandler) ListRoutes(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	routes, err := h.nodeService.ListRoutes(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, gin.H{"routes": routes})
}

// DeleteRoute handles Go-only node/route/delete (both directions).
func (h *NodeHandler) DeleteRoute(c *gin.Context) {
	var req struct {
		SrcNodeID    string `json:"src_node_id"`
		DstNodeID    string `json:"dst_node_id"`
		SrcNodeIDAlt string `json:"srcNodeId"`
		DstNodeIDAlt string `json:"dstNodeId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	src, dst := firstNonEmptyStr(req.SrcNodeID, req.SrcNodeIDAlt), firstNonEmptyStr(req.DstNodeID, req.DstNodeIDAlt)
	if src == "" || dst == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.nodeService.DeleteRoute(nodeReqCtx(c), src, dst); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// Refresh handles node/refresh.
func (h *NodeHandler) Refresh(c *gin.Context) {
	var req nodeIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	node, err := h.nodeService.RefreshNode(nodeReqCtx(c), req.id())
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, node)
}

// ResultList handles node result list retrieval.
func (h *NodeHandler) ResultList(c *gin.Context) {
	var req service.ListNodeResultRequest
	// Java: @Valid @RequestBody -> a missing / malformed body is a
	// validation error (202011100), not an empty query.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
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

func firstNonEmptyStr(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
