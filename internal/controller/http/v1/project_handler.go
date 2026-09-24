package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// ProjectHandler handles project-related HTTP requests (Java ProjectController).
type ProjectHandler struct {
	projectService   *service.ProjectService
	datatableService *service.DatatableService
	nodeService      *service.NodeService
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(projectService *service.ProjectService, datatableService *service.DatatableService, nodeService *service.NodeService) *ProjectHandler {
	return &ProjectHandler{projectService: projectService, datatableService: datatableService, nodeService: nodeService}
}

type projectIDRequest struct {
	ProjectID string `json:"projectId"`
}

// Create handles project creation.
func (h *ProjectHandler) Create(c *gin.Context) {
	var req service.CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	project, err := h.projectService.CreateProject(operatorCtx(c), &req, c.GetString("owner_id"))
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, project)
}

// Get handles project detail retrieval.
func (h *ProjectHandler) Get(c *gin.Context) {
	var req projectIDRequest
	if !bind(c, &req) {
		return
	}
	project, err := h.projectService.GetProject(operatorCtx(c), req.ProjectID)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, project)
}

// List returns every project (Java listProject); page / size (Go extension)
// switch to the paginated {projects,total,page,size} shape.
func (h *ProjectHandler) List(c *gin.Context) {
	var req service.ProjectListRequest
	if !bind(c, &req) {
		return
	}
	if req.Page > 0 || req.Size > 0 {
		result, err := h.projectService.ListProjects(operatorCtx(c), req.Page, req.Size, req.Name)
		if err != nil {
			failService(c, err, nil)
			return
		}
		response.OK(c, result)
		return
	}
	result, err := h.projectService.ListAllProjects(operatorCtx(c), req.Name)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, result)
}

// Update handles project update.
func (h *ProjectHandler) Update(c *gin.Context) {
	var req service.UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return
	}
	if err := h.projectService.UpdateProject(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// Delete handles project deletion.
func (h *ProjectHandler) Delete(c *gin.Context) {
	var req projectIDRequest
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.DeleteProject(operatorCtx(c), req.ProjectID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// AddNode handles adding a node to a project.
func (h *ProjectHandler) AddNode(c *gin.Context) {
	var req struct {
		ProjectID string `json:"projectId"`
		NodeID    string `json:"nodeId"`
	}
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.AddNode(operatorCtx(c), req.ProjectID, req.NodeID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// AddInst handles adding an institution to a project.
func (h *ProjectHandler) AddInst(c *gin.Context) {
	var req struct {
		ProjectID string `json:"projectId"`
		InstID    string `json:"instId"`
	}
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.AddInst(operatorCtx(c), req.ProjectID, req.InstID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// Archive handles project archiving.
func (h *ProjectHandler) Archive(c *gin.Context) {
	var req projectIDRequest
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.ArchiveProject(operatorCtx(c), req.ProjectID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// AddDatatable handles adding a datatable to a project.
func (h *ProjectHandler) AddDatatable(c *gin.Context) {
	var req service.AddDatatableRequest
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.AddDatatable(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// DeleteDatatable handles removing a datatable from a project.
func (h *ProjectHandler) DeleteDatatable(c *gin.Context) {
	var req service.ProjDeleteDatatableRequest
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.DeleteDatatable(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// GetDatatable returns the project level column config of a datatable.
func (h *ProjectHandler) GetDatatable(c *gin.Context) {
	var req service.ProjGetDatatableRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.projectService.GetDatatable(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// TeeList handles TEE-capable node list retrieval.
func (h *ProjectHandler) TeeList(c *gin.Context) {
	nodes, err := h.nodeService.ListTeeNodes(c.Request.Context())
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, nodes)
}

// GetOutTable handles output table retrieval.
func (h *ProjectHandler) GetOutTable(c *gin.Context) {
	var req struct {
		ProjectID string `json:"projectId"`
		GraphID   string `json:"graphId"`
	}
	if !bind(c, &req) {
		return
	}
	vo, err := h.projectService.GetProjectOutTable(operatorCtx(c), req.ProjectID, req.GraphID)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// UpdateTableConfig handles table config update.
func (h *ProjectHandler) UpdateTableConfig(c *gin.Context) {
	var req service.AddDatatableRequest
	if !bind(c, &req) {
		return
	}
	if err := h.projectService.UpdateTableConfig(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// DatasourceList handles datasource list for a project, aggregated per node.
func (h *ProjectHandler) DatasourceList(c *gin.Context) {
	var req projectIDRequest
	if !bind(c, &req) {
		return
	}
	result, err := h.projectService.ListProjectDatasources(operatorCtx(c), req.ProjectID)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, result)
}
