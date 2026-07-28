package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// DatatableHandler handles datatable-related HTTP requests.
type DatatableHandler struct {
	datatableService *service.DatatableService
	kusciaClient     *kuscia.Client
}

// NewDatatableHandler creates a new DatatableHandler.
func NewDatatableHandler(datatableService *service.DatatableService, kusciaClient *kuscia.Client) *DatatableHandler {
	return &DatatableHandler{datatableService: datatableService, kusciaClient: kusciaClient}
}

// Register handles datatable registration.
func (h *DatatableHandler) Register(c *gin.Context) {
	var req service.RegisterDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.datatableService.RegisterDatatable(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles datatable list retrieval.
func (h *DatatableHandler) List(c *gin.Context) {
	var req service.ListDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	datatables, err := h.datatableService.ListDatatables(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, datatables)
}

// Detail handles datatable detail retrieval.
func (h *DatatableHandler) Detail(c *gin.Context) {
	var req service.GetDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.datatableService.GetDatatable(c.Request.Context(), &req)
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

// Delete handles datatable deletion.
func (h *DatatableHandler) Delete(c *gin.Context) {
	var req service.DeleteDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.datatableService.DeleteDatatable(c.Request.Context(), &req); err != nil {
		if err == service.ErrDatatableNotFound {
			response.Fail(c, errcode.DatatableNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Grant handles datatable grant via Kuscia DomainData grant API.
func (h *DatatableHandler) Grant(c *gin.Context) {
	var req service.GrantDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Grant domain data access to target node via Kuscia
	if h.kusciaClient != nil {
		grantReq := &kuscia.GrantDomainDataRequest{
			DomainID:     req.NodeID,
			DomainDataID: req.DatatableID,
			GrantDomain:  req.TargetNode,
		}
		if err := h.kusciaClient.GrantDomainData(c.Request.Context(), grantReq); err != nil {
			// Best-effort: Kuscia may be unreachable in dev mode
			response.OK(c, gin.H{"granted": false, "reason": err.Error()})
			return
		}
	}

	response.OK(c, gin.H{"granted": true})
}

// CreateFedTable handles federated table creation.
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
