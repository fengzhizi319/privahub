package v1

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// ModelHandler handles model export / management / serving requests
// (Java ModelExportController + ModelManagementController).
type ModelHandler struct {
	modelService *service.ModelService
}

// NewModelHandler creates a new ModelHandler. The Kuscia client argument is
// kept for wiring compatibility; Kuscia calls go through the service.
func NewModelHandler(modelService *service.ModelService, _ *kuscia.Client) *ModelHandler {
	return &ModelHandler{modelService: modelService}
}

// requireField is Java @NotBlank (whitespace-only values are blank too).
func requireField(c *gin.Context, v, name string) bool {
	if strings.TrimSpace(v) == "" {
		response.FailWithMsg(c, errcode.ParamError, name+" must not be blank")
		return false
	}
	return true
}

// --- ModelExportController ---

// Pack handles POST /model/pack.
func (h *ModelHandler) Pack(c *gin.Context) {
	var req service.ModelExportPackageRequest
	if !bind(c, &req) || !requireField(c, req.ProjectID, "projectId") || !middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	resp, err := h.modelService.ExportModel(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, errcode.ModelExportFailed)
		return
	}
	response.OK(c, resp)
}

// PackStatus handles POST /model/status.
func (h *ModelHandler) PackStatus(c *gin.Context) {
	var req service.ModelExportStatusRequest
	if !bind(c, &req) || !requireField(c, req.JobID, "jobId") || !requireField(c, req.ProjectID, "projectId") ||
		!middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	dto, err := h.modelService.QueryModelExport(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, errcode.ModelExportFailed)
		return
	}
	response.OK(c, dto)
}

// ModelPartyPath handles POST /model/modelPartyPath.
func (h *ModelHandler) ModelPartyPath(c *gin.Context) {
	var req service.ModelPartyPathRequest
	// Java ModelPartyPathRequest: projectId, graphNodeId, graphNodeOutPutId are @NotBlank.
	if !bind(c, &req) || !requireField(c, req.ProjectID, "projectId") || !requireField(c, req.GraphNodeID, "graphNodeId") ||
		!requireField(c, req.GraphNodeOutPutID, "graphNodeOutPutId") || !middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	out, err := h.modelService.ModelPartyPath(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, errcode.ModelExportFailed)
		return
	}
	response.OK(c, out)
}

// --- ModelManagementController ---

type modelIDRequest struct {
	ModelID   string `json:"modelId"`
	ProjectID string `json:"projectId"`
	NodeID    string `json:"nodeId"`
}

type servingIDRequest struct {
	ServingID string `json:"servingId"`
}

// Page handles POST /model/page.
func (h *ModelHandler) Page(c *gin.Context) {
	var req service.QueryModelPageRequest
	if !bind(c, &req) || !requireField(c, req.ProjectID, "projectId") || !middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	vo, err := h.modelService.ModelPackPage(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// Detail handles POST /model/detail.
func (h *ModelHandler) Detail(c *gin.Context) {
	var req modelIDRequest
	if !bind(c, &req) || !requireField(c, req.ModelID, "modelId") || !requireField(c, req.ProjectID, "projectId") ||
		!middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	vo, err := h.modelService.ModelPackDetail(operatorCtx(c), req.ModelID, req.ProjectID)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// Info handles POST /model/info.
func (h *ModelHandler) Info(c *gin.Context) {
	var req modelIDRequest
	if !bind(c, &req) || !requireField(c, req.ModelID, "modelId") || !requireField(c, req.ProjectID, "projectId") ||
		!middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	vo, err := h.modelService.ModelPackInfo(operatorCtx(c), req.ModelID, req.ProjectID)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// CreateServing handles POST /model/serving/create.
func (h *ModelHandler) CreateServing(c *gin.Context) {
	var req service.CreateModelServingRequest
	if !bind(c, &req) || !requireField(c, req.ModelID, "modelId") || !requireField(c, req.ProjectID, "projectId") {
		return
	}
	if msg := service.ValidateServingPartyConfigs(req.PartyConfigs); msg != "" {
		response.FailWithMsg(c, errcode.ParamError, msg)
		return
	}
	if !middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	if err := h.modelService.CreateModelServing(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// requireOwnerProject checks project access for a model/serving resolved by
// id (Java has no DataResource on these endpoints; unknown ids fall through
// to the service's not-found error).
func requireOwnerProject(c *gin.Context, projectID string) bool {
	if projectID == "" {
		return true
	}
	return middleware.RequireProjectAccess(c, projectID)
}

// ServingDetail handles POST /model/serving/detail.
func (h *ModelHandler) ServingDetail(c *gin.Context) {
	var req servingIDRequest
	if !bind(c, &req) || !requireField(c, req.ServingID, "servingId") ||
		!requireOwnerProject(c, h.modelService.ServingProjectID(c.Request.Context(), req.ServingID)) {
		return
	}
	vo, err := h.modelService.QueryModelServingDetail(operatorCtx(c), req.ServingID)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// DeleteServing handles POST /model/serving/delete.
func (h *ModelHandler) DeleteServing(c *gin.Context) {
	var req servingIDRequest
	if !bind(c, &req) || !requireField(c, req.ServingID, "servingId") ||
		!requireOwnerProject(c, h.modelService.ServingProjectID(c.Request.Context(), req.ServingID)) {
		return
	}
	if err := h.modelService.DeleteModelServing(operatorCtx(c), req.ServingID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// Discard handles POST /model/discard.
func (h *ModelHandler) Discard(c *gin.Context) {
	var req modelIDRequest
	if !bind(c, &req) || !requireField(c, req.ModelID, "modelId") ||
		!requireOwnerProject(c, h.modelService.ModelProjectID(c.Request.Context(), req.ModelID)) {
		return
	}
	if err := h.modelService.DiscardModelPack(operatorCtx(c), req.ModelID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// Delete handles POST /model/delete.
func (h *ModelHandler) Delete(c *gin.Context) {
	var req modelIDRequest
	if !bind(c, &req) || !requireField(c, req.ModelID, "modelId") ||
		!requireOwnerProject(c, h.modelService.ModelProjectID(c.Request.Context(), req.ModelID)) {
		return
	}
	if err := h.modelService.DeleteModelPack(operatorCtx(c), req.NodeID, req.ModelID); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}
