package v1

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"gorm.io/gorm"
)

// ApprovalHandler handles approval HTTP requests.
type ApprovalHandler struct {
	db *gorm.DB
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(db *gorm.DB) *ApprovalHandler {
	return &ApprovalHandler{db: db}
}

// Create handles approval creation.
func (h *ApprovalHandler) Create(c *gin.Context) {
	var req struct {
		Initiator   string   `json:"initiator" binding:"required"`
		Voters      []string `json:"voters" binding:"required"`
		Type        string   `json:"type" binding:"required"`
		Description string   `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	voteID := uuid.New().String()[:8]
	vote := &model.VoteRequestDO{
		VoteID:            voteID,
		Initiator:         req.Initiator,
		Voters:            strings.Join(req.Voters, ","),
		Type:              req.Type,
		VoteCounter:       req.Initiator,
		Executors:         strings.Join(req.Voters, ","),
		ApprovedThreshold: len(req.Voters),
		Status:            0, // 0 = PENDING
		ExecuteStatus:     "COMMITTED",
		Description:       req.Description,
	}

	if err := h.db.WithContext(c.Request.Context()).Create(vote).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	// Create vote invites for each voter
	for _, voter := range req.Voters {
		invite := &model.VoteInviteDO{
			VoteID:            voteID,
			Initiator:         req.Initiator,
			VoteParticipantID: voter,
			Type:              req.Type,
			Action:            "REVIEWING",
			Description:       req.Description,
		}
		h.db.Create(invite)
	}

	response.OK(c, gin.H{"vote_id": voteID})
}

// PullStatus handles approval status polling.
func (h *ApprovalHandler) PullStatus(c *gin.Context) {
	var req struct {
		VoteID string `json:"vote_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	var vote model.VoteRequestDO
	if err := h.db.WithContext(c.Request.Context()).Where("vote_id = ?", req.VoteID).First(&vote).Error; err != nil {
		response.Fail(c, errcode.NotFound)
		return
	}

	var invites []model.VoteInviteDO
	h.db.Where("vote_id = ?", req.VoteID).Find(&invites)

	inviteVOs := make([]gin.H, 0, len(invites))
	for _, inv := range invites {
		inviteVOs = append(inviteVOs, gin.H{
			"voter":  inv.VoteParticipantID,
			"action": inv.Action,
		})
	}

	statusStr := "PENDING"
	if vote.Status == 1 {
		statusStr = "APPROVED"
	} else if vote.Status == 2 {
		statusStr = "REJECTED"
	}

	response.OK(c, gin.H{
		"vote_id": vote.VoteID,
		"status":  statusStr,
		"invites": inviteVOs,
	})
}
