package v1

import (
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// errVoteInviteNotFound is a sentinel error used within the Reply transaction
// to distinguish "no matching invite row" from real DB failures.
var errVoteInviteNotFound = errors.New("vote invite not found")

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

	// Bug66 fix: check Count and Find errors instead of silently ignoring them.
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	var votes []model.VoteRequestDO
	if err := query.Order("gmt_create DESC").Offset((req.Page - 1) * req.Size).Limit(req.Size).Find(&votes).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

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
	// Bug67 fix: check the DB error instead of silently ignoring it.
	if err := h.db.WithContext(c.Request.Context()).Where("vote_id = ?", req.VoteID).Find(&invites).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

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

	// Bug48 fix: wrap the vote reply + status finalization in a transaction
	// to prevent inconsistent state if the process crashes mid-way.
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Update the vote invite
		result := tx.Model(&model.VoteInviteDO{}).
			Where("vote_id = ? AND vote_participant_id = ?", req.VoteID, req.Voter).
			Updates(map[string]interface{}{
				"action": req.Action,
				"reason": req.Reason,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errVoteInviteNotFound
		}

		// Check if all voters have responded
		var pendingCount int64
		if err := tx.Model(&model.VoteInviteDO{}).
			Where("vote_id = ? AND action = ?", req.VoteID, "REVIEWING").
			Count(&pendingCount).Error; err != nil {
			return err
		}

		if pendingCount == 0 {
			// All voted - check if any rejected
			var rejectCount int64
			if err := tx.Model(&model.VoteInviteDO{}).
				Where("vote_id = ? AND action = ?", req.VoteID, "REJECT").
				Count(&rejectCount).Error; err != nil {
				return err
			}

			newStatus := int8(1) // APPROVED
			if rejectCount > 0 {
				newStatus = 2 // REJECTED
			}
			if err := tx.Model(&model.VoteRequestDO{}).
				Where("vote_id = ?", req.VoteID).
				Update("status", newStatus).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if err == errVoteInviteNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
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
	// Bug68 fix: check the Count error instead of silently ignoring it.
	if err := query.Count(&count).Error; err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	// Frontend reads the response data as a bare number (Number(unwrap(data))).
	response.OK(c, count)
}
