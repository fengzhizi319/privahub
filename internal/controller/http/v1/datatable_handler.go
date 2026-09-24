package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// DatatableHandler handles /api/v1alpha1/datatable/* (Java DatatableController
// plus Go-only register/detail/grant/fed/create).
type DatatableHandler struct {
	datatableService *service.DatatableService
	kusciaClient     *kuscia.Client
}

// NewDatatableHandler creates a new DatatableHandler.
func NewDatatableHandler(datatableService *service.DatatableService, kusciaClient *kuscia.Client) *DatatableHandler {
	return &DatatableHandler{datatableService: datatableService, kusciaClient: kusciaClient}
}

// Register handles Go-only datatable registration into a project.
func (h *DatatableHandler) Register(c *gin.Context) {
	var req service.RegisterDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.datatableService.RegisterDatatable(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.SystemError)
		return
	}
	response.OK(c, vo)
}

// List handles POST /datatable/list.
// Request: {ownerId, pageSize, pageNumber, statusFilter?, datatableNameFilter?, types?[], nodeNamesFilter?[], teeNodeId?}
// Response: {datatableNodeVOList[{datatableVO, nodeId, nodeName}], totalDatatableNums}
func (h *DatatableHandler) List(c *gin.Context) {
	var req service.ListDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datatableService.ListDatatablesByOwnerID(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatatableQueryDatatableFailed)
		return
	}
	response.OK(c, vo)
}

// Detail handles the Go-only project datatable detail.
func (h *DatatableHandler) Detail(c *gin.Context) {
	var req service.ProjectDatatableDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.datatableService.GetProjectDatatable(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrDatatableNotFound {
			response.Fail(c, errcode.DatatableNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}
	response.OK(c, vo)
}

// Delete handles POST /datatable/delete (no project id required).
// Request: {nodeId, datatableId, datasourceType?, datasourceId?, relativeUri?, teeNodeId?, type?}
func (h *DatatableHandler) Delete(c *gin.Context) {
	var req service.DeleteDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if err := h.datatableService.DeleteDatatable(c.Request.Context(), &req); err != nil {
		failDataErr(c, err, errcode.DatatableDeleteDatatableFailed)
		return
	}
	response.OKEmpty(c)
}

// Grant handles the Go-only datatable grant via Kuscia DomainDataGrant. Kuscia
// failures are returned as errors (not OK).
func (h *DatatableHandler) Grant(c *gin.Context) {
	var req service.GrantDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	grantID, err := h.datatableService.GrantDatatable(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatatableCreateDatatableGrantFailed)
		return
	}
	response.OK(c, gin.H{"granted": true, "domaindatagrant_id": grantID})
}

// CreateFedTable handles federated table creation (Go-only).
func (h *DatatableHandler) CreateFedTable(c *gin.Context) {
	var req service.CreateFedTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	vo, err := h.datatableService.CreateFedTable(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}
	response.OK(c, vo)
}

// Create handles POST /datatable/create.
// Request: {ownerId, nodeIds[], datatableName, datasourceId, datasourceName, datasourceType: LOCAL|OSS|ODPS|MYSQL|HTTP,
// desc?, relativeUri, columns[{colName,colType,colComment}], partition?{type,fields[{name,type,comment}]}, nullStrs?[]}
// Response: {dataTableNodeInfos[{nodeId,domainDataId}], failedCreatedNodes{}}
func (h *DatatableHandler) Create(c *gin.Context) {
	var req service.CreateDatatableCompatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datatableService.CreateDatatableCompat(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatatableCreateFailed)
		return
	}
	response.OK(c, vo)
}

// Get handles POST /datatable/get.
// Request: {nodeId, datatableId, datasourceType?, type?, teeNodeId?}
// Response: {datatableVO{...}, nodeId, nodeName}
func (h *DatatableHandler) Get(c *gin.Context) {
	var req service.GetDatatableCompatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	vo, err := h.datatableService.GetDatatableCompat(c.Request.Context(), &req)
	if err != nil {
		failDataErr(c, err, errcode.DatatableQueryDatatableFailed)
		return
	}
	response.OK(c, vo)
}

// PushToTee handles POST /datatable/pushToTee.
// Request: {nodeId, datatableId, teeNodeId?, datasourceId?, relativeUri?}
func (h *DatatableHandler) PushToTee(c *gin.Context) {
	var req service.PushDatatableToTeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if err := h.datatableService.PushDatatableToTee(c.Request.Context(), &req); err != nil {
		failDataErr(c, err, errcode.DatatableCreateTeeJobFailed)
		return
	}
	response.OKEmpty(c)
}
