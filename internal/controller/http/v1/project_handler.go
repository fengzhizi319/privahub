package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// ProjectHandler handles project-related HTTP requests.
type ProjectHandler struct {
	projectService   *service.ProjectService
	datatableService *service.DatatableService
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(projectService *service.ProjectService, datatableService *service.DatatableService) *ProjectHandler {
	return &ProjectHandler{projectService: projectService, datatableService: datatableService}
}

// Create handles project creation.
func (h *ProjectHandler) Create(c *gin.Context) {
	var req service.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ownerID, _ := c.Get("owner_id")
	ownerIDStr, _ := ownerID.(string)

	project, err := h.projectService.CreateProject(c.Request.Context(), &req, ownerIDStr)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, project)
}

// Get handles project detail retrieval.
func (h *ProjectHandler) Get(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	project, err := h.projectService.GetProject(c.Request.Context(), req.ProjectID)
	if err != nil {
		response.Fail(c, errcode.ProjectNotFound)
		return
	}

	response.OK(c, project)
}

// List handles project list retrieval with pagination.
func (h *ProjectHandler) List(c *gin.Context) {
	var req struct {
		Page int    `json:"page"`
		Size int    `json:"size"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body for default pagination
		req.Page = 1
		req.Size = 10
	}

	result, err := h.projectService.ListProjects(c.Request.Context(), req.Page, req.Size, req.Name)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, result)
}

// Update handles project update.
func (h *ProjectHandler) Update(c *gin.Context) {
	var req service.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.UpdateProject(c.Request.Context(), &req); err != nil {
		if err == service.ErrProjectNotFound {
			response.Fail(c, errcode.ProjectNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Delete handles project deletion.
func (h *ProjectHandler) Delete(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.DeleteProject(c.Request.Context(), req.ProjectID); err != nil {
		if err == service.ErrProjectNotFound {
			response.Fail(c, errcode.ProjectNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// AddNode handles adding a node to a project.
func (h *ProjectHandler) AddNode(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
		NodeID    string `json:"node_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.AddNode(c.Request.Context(), req.ProjectID, req.NodeID); err != nil {
		if err == service.ErrProjectNotFound {
			response.Fail(c, errcode.ProjectNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// AddInst handles adding an institution to a project.
func (h *ProjectHandler) AddInst(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
		InstID    string `json:"inst_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.AddInst(c.Request.Context(), req.ProjectID, req.InstID); err != nil {
		if err == service.ErrProjectNotFound {
			response.Fail(c, errcode.ProjectNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Archive handles project archiving.
func (h *ProjectHandler) Archive(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.ArchiveProject(c.Request.Context(), req.ProjectID); err != nil {
		if err == service.ErrProjectNotFound {
			response.Fail(c, errcode.ProjectNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// AddDatatable handles adding a datatable to a project.
func (h *ProjectHandler) AddDatatable(c *gin.Context) {
	var req service.AddDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.AddDatatable(c.Request.Context(), &req); err != nil {
		if err == service.ErrProjectDatatableInvalid {
			response.Fail(c, errcode.ParamError)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// DeleteDatatable handles removing a datatable from a project.
func (h *ProjectHandler) DeleteDatatable(c *gin.Context) {
	var req service.ProjDeleteDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.DeleteDatatable(c.Request.Context(), &req); err != nil {
		if err == service.ErrProjectDatatableInvalid {
			response.Fail(c, errcode.ParamError)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// GetDatatable handles getting a datatable from a project.
func (h *ProjectHandler) GetDatatable(c *gin.Context) {
	var req struct {
		ProjectID   string `json:"projectId"`
		NodeID      string `json:"nodeId"`
		DatatableID string `json:"datatableId"`
		Type        string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Reuse the datatable service compat query so the response matches the
	// frontend DatatableNodeVO contract ({datatableVO, nodeName, nodeId}).
	vo, err := h.datatableService.GetDatatableCompat(c.Request.Context(), &service.GetDatatableCompatRequest{
		NodeID:      req.NodeID,
		DatatableID: req.DatatableID,
		Type:        req.Type,
	})
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// TeeList handles TEE job list retrieval.
func (h *ProjectHandler) TeeList(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// TEE jobs are a subset of jobs - return empty for now
	response.OK(c, []interface{}{})
}

// GetOutTable handles output table retrieval.
func (h *ProjectHandler) GetOutTable(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
		JobID     string `json:"job_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	response.OK(c, gin.H{"project_id": req.ProjectID, "tables": []interface{}{}})
}

// UpdateTableConfig handles table config update.
func (h *ProjectHandler) UpdateTableConfig(c *gin.Context) {
	var req service.AddDatatableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.projectService.UpdateTableConfig(c.Request.Context(), &req); err != nil {
		if err == service.ErrProjectDatatableInvalid {
			response.Fail(c, errcode.ParamError)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// DatasourceList handles datasource list for a project.
func (h *ProjectHandler) DatasourceList(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id" binding:"required"`
		NodeID    string `json:"node_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	response.OK(c, []interface{}{})
}
