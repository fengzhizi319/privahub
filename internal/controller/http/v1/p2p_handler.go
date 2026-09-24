package v1

import (
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// P2PHandler is Java P2PProjectController (/p2p/project/*) +
// P2pNodeController (/p2p/node/*).
type P2PHandler struct {
	db  *gorm.DB
	svc *votehandler.P2PService
	log *zap.Logger
}

// NewP2PHandler creates a new P2PHandler.
func NewP2PHandler(db *gorm.DB, svc *votehandler.P2PService, log *zap.Logger) *P2PHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &P2PHandler{db: db, svc: svc, log: log}
}

// ProjectCreate is POST p2p/project/create → {projectId}.
func (h *P2PHandler) ProjectCreate(c *gin.Context) {
	var req votehandler.P2PCreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "name is required")
		return
	}
	id, err := h.svc.CreateProject(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, gin.H{"projectId": id})
}

// ProjectList is POST p2p/project/list → ProjectVO[].
func (h *P2PHandler) ProjectList(c *gin.Context) {
	list, err := h.svc.ListProjects(nodeReqCtx(c))
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, list)
}

// ProjectUpdate is POST p2p/project/update {projectId, name, description}
// (@DataResource projectId PROJECT_ID; owner only).
func (h *P2PHandler) ProjectUpdate(c *gin.Context) {
	var req struct {
		ProjectID   string `json:"projectId"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "projectId must not be blank")
		return
	}
	if !middleware.RequireProjectAccess(c, req.ProjectID) {
		return
	}
	if err := h.svc.UpdateProject(nodeReqCtx(c), req.ProjectID, req.Name, req.Description); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// ProjectArchive is POST p2p/project/archive {projectId} (owner, REVIEWING only).
func (h *P2PHandler) ProjectArchive(c *gin.Context) {
	var req struct {
		ProjectID string `json:"projectId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProjectID == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "projectId must not be blank")
		return
	}
	if err := h.svc.ArchiveProject(nodeReqCtx(c), req.ProjectID); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// ProjectParticipants is POST p2p/project/participants {voteId}.
func (h *P2PHandler) ProjectParticipants(c *gin.Context) {
	var req struct {
		VoteID string `json:"voteId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.VoteID == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "voteId must not be blank")
		return
	}
	vo, err := h.svc.Participants(nodeReqCtx(c), req.VoteID)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// NodeCreate is POST p2p/node/create (Java P2pCreateNodeRequest) → nodeId.
func (h *P2PHandler) NodeCreate(c *gin.Context) {
	var req votehandler.P2PCreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.SystemValidationError, err.Error())
		return
	}
	id, err := h.svc.CreateNode(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, id)
}

// NodeDelete is POST p2p/node/delete {routerId}.
func (h *P2PHandler) NodeDelete(c *gin.Context) {
	var req routerIDBody
	if err := c.ShouldBindJSON(&req); err != nil || req.id() == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "routerId must not be blank")
		return
	}
	if err := h.svc.DeleteNodeRoute(nodeReqCtx(c), req.id()); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}
