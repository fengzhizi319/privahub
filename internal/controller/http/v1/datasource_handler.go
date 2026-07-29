package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// DatasourceHandler handles datasource-related HTTP requests.
type DatasourceHandler struct {
	datasourceService *service.DatasourceService
}

// NewDatasourceHandler creates a new DatasourceHandler.
func NewDatasourceHandler(datasourceService *service.DatasourceService) *DatasourceHandler {
	return &DatasourceHandler{datasourceService: datasourceService}
}

// Create handles datasource creation.
func (h *DatasourceHandler) Create(c *gin.Context) {
	var req service.CreateDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.datasourceService.CreateDatasource(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles datasource list retrieval.
func (h *DatasourceHandler) List(c *gin.Context) {
	var req service.DatasourceListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.DatasourceListRequest{}
	}

	vo, err := h.datasourceService.ListDatasources(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// Detail handles datasource detail retrieval.
func (h *DatasourceHandler) Detail(c *gin.Context) {
	var req service.DatasourceDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.DatasourceDetailRequest{}
	}

	vo, err := h.datasourceService.GetDatasourceDetail(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrDatasourceNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// Delete handles datasource deletion.
func (h *DatasourceHandler) Delete(c *gin.Context) {
	var req service.DeleteDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.DeleteDatasourceRequest{}
	}

	if err := h.datasourceService.DeleteDatasource(c.Request.Context(), &req); err != nil {
		if err == service.ErrDatasourceNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Test handles datasource connection test.
func (h *DatasourceHandler) Test(c *gin.Context) {
	var req service.TestDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.datasourceService.TestDatasource(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// Nodes handles datasource-nodes retrieval.
func (h *DatasourceHandler) Nodes(c *gin.Context) {
	var req service.DatasourceNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.DatasourceNodesRequest{}
	}

	vo, err := h.datasourceService.GetDatasourceNodes(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}
