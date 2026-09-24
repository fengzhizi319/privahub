package v1

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// GraphHandler handles DAG graph HTTP requests (Java GraphController).
type GraphHandler struct {
	graphService *service.GraphService
}

// NewGraphHandler creates a new GraphHandler.
func NewGraphHandler(graphService *service.GraphService) *GraphHandler {
	return &GraphHandler{graphService: graphService}
}

// operatorCtx returns the request context carrying the current user identity
// (Java UserContext) for the graph / project / job services.
func operatorCtx(c *gin.Context) context.Context {
	return service.WithOperator(c.Request.Context(), service.Operator{
		UserID:    c.GetString("user_id"),
		OwnerID:   c.GetString("owner_id"),
		OwnerType: c.GetString("owner_type"),
	})
}

// failService renders a service error: PadError carries its Java error code,
// legacy sentinels are mapped, everything else becomes fallback.
func failService(c *gin.Context, err error, fallback *errcode.ErrorCode) {
	if pe, ok := service.AsPadError(err); ok {
		response.FailArgs(c, pe.Code, pe.Args...)
		return
	}
	switch {
	case errors.Is(err, service.ErrGraphNotFound):
		response.Fail(c, errcode.GraphNotExists)
	case errors.Is(err, service.ErrJobNotFound):
		response.Fail(c, errcode.ProjectJobNotExists)
	case errors.Is(err, service.ErrProjectNotFound):
		response.Fail(c, errcode.ProjectNotExists)
	case errors.Is(err, service.ErrProjectDatatableInvalid):
		response.Fail(c, errcode.ParamError)
	default:
		if fallback == nil {
			fallback = errcode.SystemError
		}
		response.FailWithMsg(c, fallback, err.Error())
	}
}

// bind decodes the JSON body; an empty body leaves req untouched.
func bind(c *gin.Context, req any) bool {
	if c.Request.ContentLength == 0 {
		return true
	}
	if err := c.ShouldBindJSON(req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return false
	}
	return true
}

// Create handles graph creation.
func (h *GraphHandler) Create(c *gin.Context) {
	var req service.CreateGraphRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.graphService.CreateGraph(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// List handles graph list retrieval.
func (h *GraphHandler) List(c *gin.Context) {
	var req service.ListGraphRequest
	if !bind(c, &req) {
		return
	}
	graphs, err := h.graphService.ListGraph(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, graphs)
}

// Detail handles graph detail retrieval.
func (h *GraphHandler) Detail(c *gin.Context) {
	var req service.GetGraphRequest
	if !bind(c, &req) {
		return
	}
	detail, err := h.graphService.GetGraphDetail(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, detail)
}

// Delete handles graph deletion.
func (h *GraphHandler) Delete(c *gin.Context) {
	var req service.DeleteGraphRequest
	if !bind(c, &req) {
		return
	}
	if err := h.graphService.DeleteGraph(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// FullUpdate handles full graph update (nodes + edges).
func (h *GraphHandler) FullUpdate(c *gin.Context) {
	var req service.FullUpdateGraphRequest
	if !bind(c, &req) {
		return
	}
	if err := h.graphService.FullUpdateGraph(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// UpdateMeta handles graph meta (name) update.
func (h *GraphHandler) UpdateMeta(c *gin.Context) {
	var req service.UpdateGraphMetaRequest
	if !bind(c, &req) {
		return
	}
	if err := h.graphService.UpdateGraphMeta(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// UpdateNode handles single graph node update.
func (h *GraphHandler) UpdateNode(c *gin.Context) {
	var req service.UpdateGraphNodeRequest
	if !bind(c, &req) {
		return
	}
	if err := h.graphService.UpdateGraphNode(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// Start handles graph execution start.
func (h *GraphHandler) Start(c *gin.Context) {
	var req service.StartGraphRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.graphService.StartGraph(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, errcode.ProjectJobCreateError)
		return
	}
	response.OK(c, vo)
}

// Stop handles graph execution stop.
func (h *GraphHandler) Stop(c *gin.Context) {
	var req service.StopGraphRequest
	if !bind(c, &req) {
		return
	}
	if err := h.graphService.StopGraph(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// NodeStatus handles graph node status retrieval.
func (h *GraphHandler) NodeStatus(c *gin.Context) {
	var req service.ListGraphNodeStatusRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.graphService.ListNodeStatus(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// NodeOutput handles graph node output retrieval.
func (h *GraphHandler) NodeOutput(c *gin.Context) {
	var req service.GraphNodeOutputRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.graphService.GetNodeOutput(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// NodeLogs handles graph node logs retrieval.
func (h *GraphHandler) NodeLogs(c *gin.Context) {
	var req service.GraphNodeLogsRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.graphService.GetNodeLogs(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// NodeMaxIndex handles node max index refresh.
func (h *GraphHandler) NodeMaxIndex(c *gin.Context) {
	var req service.RefreshNodeMaxIndexRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.graphService.RefreshNodeMaxIndex(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}
