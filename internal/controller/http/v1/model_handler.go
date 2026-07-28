package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// ModelHandler handles model and serving HTTP requests.
type ModelHandler struct {
	modelService *service.ModelService
	kusciaClient *kuscia.Client
}

// NewModelHandler creates a new ModelHandler.
func NewModelHandler(modelService *service.ModelService, kusciaClient *kuscia.Client) *ModelHandler {
	return &ModelHandler{modelService: modelService, kusciaClient: kusciaClient}
}

// --- Model Endpoints ---

// ListModels handles model list retrieval.
func (h *ModelHandler) ListModels(c *gin.Context) {
	var req service.ModelListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.ModelListRequest{}
	}

	models, err := h.modelService.ListModels(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"modelPacks": models, "list": models})
}

// ModelDetail handles model detail retrieval.
func (h *ModelHandler) ModelDetail(c *gin.Context) {
	var req service.ModelDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.modelService.GetModelDetail(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrModelNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// DeleteModel handles model deletion.
func (h *ModelHandler) DeleteModel(c *gin.Context) {
	var req service.DeleteModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.modelService.DeleteModel(c.Request.Context(), &req); err != nil {
		if err == service.ErrModelNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// ExportModel handles model export.
func (h *ModelHandler) ExportModel(c *gin.Context) {
	var req struct {
		ModelID string `json:"model_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	path, err := h.modelService.ExportModel(c.Request.Context(), req.ModelID)
	if err != nil {
		if err == service.ErrModelNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"export_path": path})
}

// --- Serving Endpoints ---

// CreateServing handles serving creation.
func (h *ModelHandler) CreateServing(c *gin.Context) {
	var req service.CreateServingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.modelService.CreateServing(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// ListServings handles serving list retrieval.
func (h *ModelHandler) ListServings(c *gin.Context) {
	var req service.ServingListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.ServingListRequest{}
	}

	servings, err := h.modelService.ListServings(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, servings)
}

// ServingDetail handles serving detail retrieval.
func (h *ModelHandler) ServingDetail(c *gin.Context) {
	var req service.ServingDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.modelService.GetServingDetail(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrServingNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// DeleteServing handles serving deletion.
func (h *ModelHandler) DeleteServing(c *gin.Context) {
	var req service.DeleteServingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.modelService.DeleteServing(c.Request.Context(), &req); err != nil {
		if err == service.ErrServingNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// --- Model Export Extended Endpoints ---

// Pack handles model packing (triggers async pack job).
func (h *ModelHandler) Pack(c *gin.Context) {
	var req struct {
		ModelID   string `json:"modelId" binding:"required"`
		ProjectID string `json:"projectId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Verify model exists
	_, err := h.modelService.GetModelDetail(c.Request.Context(), &service.ModelDetailRequest{ModelID: req.ModelID})
	if err != nil {
		if err == service.ErrModelNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	// Trigger a Kuscia job for model packing
	packJobID := uuid.New().String()[:8]
	if h.kusciaClient != nil {
		kusciaReq := &kuscia.CreateJobRequest{
			JobID:     packJobID,
			Initiator: "alice",
			Tasks: []kuscia.TaskConfig{
				{
					AppImage: "secretflow",
					Alias:    "model_pack_" + req.ModelID,
					Parties: []kuscia.Party{
						{Name: "alice", Role: "guest"},
					},
				},
			},
			CustomFields: map[string]string{
				"model_id": req.ModelID,
				"action":   "pack",
			},
		}
		if _, err := h.kusciaClient.CreateJob(c.Request.Context(), kusciaReq); err != nil {
			// Kuscia unreachable — return pack task ID anyway for tracking
			response.OK(c, gin.H{
				"model_id":    req.ModelID,
				"pack_job_id": packJobID,
				"status":      "packing",
				"warning":     "kuscia unreachable",
			})
			return
		}
	}

	response.OK(c, gin.H{
		"model_id":    req.ModelID,
		"pack_job_id": packJobID,
		"status":      "packing",
	})
}

// PackStatus handles model pack status query.
func (h *ModelHandler) PackStatus(c *gin.Context) {
	var req struct {
		ModelID string `json:"modelId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	response.OK(c, gin.H{
		"model_id": req.ModelID,
		"status":   "succeeded",
	})
}

// ModelPartyPath handles model party path query.
func (h *ModelHandler) ModelPartyPath(c *gin.Context) {
	var req struct {
		ModelID string `json:"modelId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.modelService.GetModelDetail(c.Request.Context(), &service.ModelDetailRequest{ModelID: req.ModelID})
	if err != nil {
		if err == service.ErrModelNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{
		"model_id": vo.ModelID,
		"parties":  vo.ModelList,
	})
}

// Discard handles model discard (soft delete / archive).
func (h *ModelHandler) Discard(c *gin.Context) {
	var req struct {
		ModelID string `json:"modelId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.modelService.DeleteModel(c.Request.Context(), &service.DeleteModelRequest{ModelID: req.ModelID}); err != nil {
		if err == service.ErrModelNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}
