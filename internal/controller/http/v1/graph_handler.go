package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// GraphHandler handles DAG graph HTTP requests.
type GraphHandler struct {
	graphService *service.GraphService
}

// NewGraphHandler creates a new GraphHandler.
func NewGraphHandler(graphService *service.GraphService) *GraphHandler {
	return &GraphHandler{graphService: graphService}
}

// Create handles graph creation.
func (h *GraphHandler) Create(c *gin.Context) {
	var req service.CreateGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.graphService.CreateGraph(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles graph list retrieval.
func (h *GraphHandler) List(c *gin.Context) {
	var req service.ListGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.ListGraphRequest{}
	}

	graphs, err := h.graphService.ListGraph(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, graphs)
}

// Detail handles graph detail retrieval.
func (h *GraphHandler) Detail(c *gin.Context) {
	var req service.GetGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.GetGraphRequest{}
	}

	detail, err := h.graphService.GetGraphDetail(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, detail)
}

// Delete handles graph deletion.
func (h *GraphHandler) Delete(c *gin.Context) {
	var req service.DeleteGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.DeleteGraphRequest{}
	}

	if err := h.graphService.DeleteGraph(c.Request.Context(), &req); err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// FullUpdate handles full graph update (nodes + edges).
func (h *GraphHandler) FullUpdate(c *gin.Context) {
	var req service.FullUpdateGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.graphService.FullUpdateGraph(c.Request.Context(), &req); err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// UpdateMeta handles graph meta (name) update.
func (h *GraphHandler) UpdateMeta(c *gin.Context) {
	var req service.UpdateGraphMetaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.graphService.UpdateGraphMeta(c.Request.Context(), &req); err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// UpdateNode handles single graph node update.
func (h *GraphHandler) UpdateNode(c *gin.Context) {
	var req service.UpdateGraphNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.graphService.UpdateGraphNode(c.Request.Context(), &req); err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Start handles graph execution start.
func (h *GraphHandler) Start(c *gin.Context) {
	var req service.StartGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.graphService.StartGraph(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// Stop handles graph execution stop.
func (h *GraphHandler) Stop(c *gin.Context) {
	var req service.StopGraphRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.graphService.StopGraph(c.Request.Context(), &req); err != nil {
		if err == service.ErrJobNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// NodeStatus handles graph node status retrieval.
func (h *GraphHandler) NodeStatus(c *gin.Context) {
	var req service.ListGraphNodeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.graphService.ListNodeStatus(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// NodeOutput handles graph node output retrieval.
func (h *GraphHandler) NodeOutput(c *gin.Context) {
	var req service.GraphNodeOutputRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.graphService.GetNodeOutput(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// NodeLogs handles graph node logs retrieval.
func (h *GraphHandler) NodeLogs(c *gin.Context) {
	var req service.GraphNodeLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.graphService.GetNodeLogs(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// NodeMaxIndex handles node max index refresh.
func (h *GraphHandler) NodeMaxIndex(c *gin.Context) {
	var req service.RefreshNodeMaxIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	idx, err := h.graphService.RefreshNodeMaxIndex(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrGraphNotFound {
			response.Fail(c, errcode.GraphNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"max_index": idx})
}
