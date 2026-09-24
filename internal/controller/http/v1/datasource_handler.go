package v1

import (
	"errors"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// failDataErr maps datasource/datatable/data service errors (BizError carrying
// a Java error code) to the response envelope.
func failDataErr(c *gin.Context, err error, fallback *errcode.ErrorCode) {
	if be, ok := service.AsBizError(err); ok && be.Code != nil {
		if be.Msg == "" {
			response.Fail(c, be.Code)
			return
		}
		response.FailWithMsg(c, be.Code, be.Msg)
		return
	}
	var ec *errcode.ErrorCode
	if errors.As(err, &ec) {
		response.Fail(c, ec)
		return
	}
	if fallback == nil {
		fallback = errcode.SystemError
	}
	response.FailWithMsg(c, fallback, err.Error())
}

// DatasourceHandler handles /api/v1alpha1/datasource/* (Java DataSourceController).
type DatasourceHandler struct {
	datasourceService *service.DatasourceService
}

// NewDatasourceHandler creates a new DatasourceHandler.
func NewDatasourceHandler(datasourceService *service.DatasourceService) *DatasourceHandler {
	return &DatasourceHandler{datasourceService: datasourceService}
}

// Create handles POST /datasource/create.
// Request: {ownerId, nodeIds[], type: OSS|ODPS|MYSQL, name, dataSourceInfo{...}}
// Response: {datasourceId, failedCreatedNodes{nodeId: reason}}
func (h *DatasourceHandler) Create(c *gin.Context) {
	var req service.CreateDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datasourceService.CreateDatasource(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatasourceDataSourceCreateFail)
		return
	}
	response.OK(c, vo)
}

// List handles POST /datasource/list.
// Request: {ownerId, name?, status?, types?[], page, size}
// Response: {infos[{datasourceId,name,type,nodes[{nodeId,nodeName,status}],relatedDatas[]}], pageSize, pageNum, total, totalPage}
func (h *DatasourceHandler) List(c *gin.Context) {
	var req service.DatasourceListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datasourceService.ListDatasources(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatasourceQueryDatasourceFailed)
		return
	}
	response.OK(c, vo)
}

// Detail handles POST /datasource/detail.
// Request: {ownerId, datasourceId, type}
// Response: {nodes[], datasourceId, name, type, status, info{...desensitized}}
func (h *DatasourceHandler) Detail(c *gin.Context) {
	var req service.DatasourceDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datasourceService.GetDatasourceDetail(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatasourceQueryDatasourceFailed)
		return
	}
	response.OK(c, vo)
}

// Delete handles POST /datasource/delete.
// Request: {ownerId, datasourceId, type: OSS|ODPS|MYSQL}
func (h *DatasourceHandler) Delete(c *gin.Context) {
	var req service.DeleteDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if err := h.datasourceService.DeleteDatasource(c.Request.Context(), &req); err != nil {
		failDataErr(c, err, errcode.DatasourceDataSourceDeleteFail)
		return
	}
	response.OKEmpty(c)
}

// Test handles POST /datasource/test (Go-only): real per-type connectivity check.
// Request: {type: OSS|ODPS|MYSQL, dataSourceInfo{...}}
// Response: {success, message}
func (h *DatasourceHandler) Test(c *gin.Context) {
	var req service.TestDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datasourceService.TestDatasource(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	response.OK(c, vo)
}

// Nodes handles POST /datasource/nodes.
// Request: {ownerId, datasourceId}
// Response: {nodes[{nodeId,nodeName,status}]}
func (h *DatasourceHandler) Nodes(c *gin.Context) {
	var req service.DatasourceNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datasourceService.GetDatasourceNodes(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatasourceQueryDatasourceFailed)
		return
	}
	response.OK(c, vo)
}
