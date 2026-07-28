package v1

import (
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MessageHandler handles message center HTTP requests.
type MessageHandler struct {
	db *gorm.DB
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(db *gorm.DB) *MessageHandler {
	return &MessageHandler{db: db}
}

// MessageVO represents a message view object.
type MessageVO struct {
	VoteID      string `json:"vote_id"`
	Type        string `json:"type"`
	Initiator   string `json:"initiator"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Action      string `json:"action,omitempty"`
	GmtCreate   string `json:"gmt_create"`
}

// List handles message list retrieval.
func (h *MessageHandler) List(c *gin.Context) {
	var req struct {
		Page int    `json:"page"`
		Size int    `json:"size"`
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Page = 1
		req.Size = 10
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 10
	}

	query := h.db.WithContext(c.Request.Context()).Model(&model.VoteRequestDO{})
	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}

	var total int64
	query.Count(&total)

	var votes []model.VoteRequestDO
	query.Order("gmt_create DESC").Offset((req.Page - 1) * req.Size).Limit(req.Size).Find(&votes)

	result := make([]MessageVO, 0, len(votes))
	for _, v := range votes {
		statusStr := "PENDING"
		if v.Status == 1 {
			statusStr = "APPROVED"
		} else if v.Status == 2 {
			statusStr = "REJECTED"
		}
		result = append(result, MessageVO{
			VoteID:      v.VoteID,
			Type:        v.Type,
			Initiator:   v.Initiator,
			Description: v.Description,
			Status:      statusStr,
			GmtCreate:   v.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}

	response.OK(c, gin.H{
		"messages": result,
		"list":     result,
		"total":    total,
		"page":     req.Page,
		"size":     req.Size,
	})
}

// Detail handles message detail retrieval.
func (h *MessageHandler) Detail(c *gin.Context) {
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
			"reason": inv.Reason,
		})
	}

	statusStr := "PENDING"
	if vote.Status == 1 {
		statusStr = "APPROVED"
	} else if vote.Status == 2 {
		statusStr = "REJECTED"
	}

	response.OK(c, gin.H{
		"vote_id":     vote.VoteID,
		"type":        vote.Type,
		"initiator":   vote.Initiator,
		"voters":      vote.Voters,
		"description": vote.Description,
		"status":      statusStr,
		"request_msg": vote.RequestMsg,
		"invites":     inviteVOs,
		"gmt_create":  vote.GmtCreate.Format("2006-01-02 15:04:05"),
	})
}

// Reply handles message reply (vote action).
func (h *MessageHandler) Reply(c *gin.Context) {
	var req struct {
		VoteID string `json:"vote_id" binding:"required"`
		Voter  string `json:"voter" binding:"required"`
		Action string `json:"action" binding:"required"` // AGREE or REJECT
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if req.Action != "AGREE" && req.Action != "REJECT" {
		response.Fail(c, errcode.ParamError)
		return
	}

	// Update the vote invite
	result := h.db.WithContext(c.Request.Context()).
		Model(&model.VoteInviteDO{}).
		Where("vote_id = ? AND vote_participant_id = ?", req.VoteID, req.Voter).
		Updates(map[string]interface{}{
			"action": req.Action,
			"reason": req.Reason,
		})

	if result.RowsAffected == 0 {
		response.Fail(c, errcode.NotFound)
		return
	}

	// Check if all voters have responded - update vote status
	var pendingCount int64
	h.db.Model(&model.VoteInviteDO{}).
		Where("vote_id = ? AND action = ?", req.VoteID, "REVIEWING").
		Count(&pendingCount)

	if pendingCount == 0 {
		// All voted - check if any rejected
		var rejectCount int64
		h.db.Model(&model.VoteInviteDO{}).
			Where("vote_id = ? AND action = ?", req.VoteID, "REJECT").
			Count(&rejectCount)

		newStatus := int8(1) // APPROVED
		if rejectCount > 0 {
			newStatus = 2 // REJECTED
		}
		h.db.Model(&model.VoteRequestDO{}).
			Where("vote_id = ?", req.VoteID).
			Update("status", newStatus)
	}

	response.OKEmpty(c)
}

// Pending handles pending message count retrieval.
func (h *MessageHandler) Pending(c *gin.Context) {
	var req struct {
		Voter string `json:"voter"`
	}
	c.ShouldBindJSON(&req)

	var count int64
	query := h.db.WithContext(c.Request.Context()).Model(&model.VoteInviteDO{}).Where("action = ?", "REVIEWING")
	if req.Voter != "" {
		query = query.Where("vote_participant_id = ?", req.Voter)
	}
	query.Count(&count)

	response.OK(c, gin.H{"pending_count": count})
}
