package v1

import (
	"strings"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// MessageHandler is Java MessageController (/api/v1alpha1/message/*).
type MessageHandler struct {
	svc *votehandler.Service
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(svc *votehandler.Service) *MessageHandler {
	return &MessageHandler{svc: svc}
}

// List is POST message/list (@DataResource ownerId NODE_ID).
func (h *MessageHandler) List(c *gin.Context) {
	var req votehandler.MessageListRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.OwnerID) == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "ownerId must not be blank")
		return
	}
	if !middleware.RequireNodeAccess(c, req.OwnerID) {
		return
	}
	vo, err := h.svc.List(nodeReqCtx(c), &req)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// Detail is POST message/detail (@DataResource ownerId NODE_ID).
func (h *MessageHandler) Detail(c *gin.Context) {
	var req struct {
		OwnerID     string `json:"ownerId"`
		VoteID      string `json:"voteId"`
		IsInitiator *bool  `json:"isInitiator"`
		VoteType    string `json:"voteType"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.SystemValidationError, "invalid request body")
		return
	}
	// Java MessageDetailRequest: @NotBlank ownerId/voteId, @NotNull isInitiator,
	// @NotBlank @OneOfType voteType.
	msg := votehandler.ValidateVoteType(req.VoteType)
	switch {
	case strings.TrimSpace(req.OwnerID) == "":
		msg = "ownerId must not be blank"
	case strings.TrimSpace(req.VoteID) == "":
		msg = "voteId must not be blank"
	case req.IsInitiator == nil:
		msg = "isInitiator must not be null"
	}
	if msg != "" {
		response.FailWithMsg(c, errcode.SystemValidationError, msg)
		return
	}
	if !middleware.RequireNodeAccess(c, req.OwnerID) {
		return
	}
	vo, err := h.svc.Detail(nodeReqCtx(c), *req.IsInitiator, req.OwnerID, req.VoteID, req.VoteType)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}

// Reply is POST message/reply (@DataResource voteParticipantId NODE_ID).
func (h *MessageHandler) Reply(c *gin.Context) {
	var req struct {
		Action            string `json:"action"`
		Reason            string `json:"reason"`
		VoteID            string `json:"voteId"`
		VoteParticipantID string `json:"voteParticipantId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.SystemValidationError, "invalid request body")
		return
	}
	// Java VoteReplyRequest: @OneOfType action, @Size(max=50) reason, @NotBlank ids.
	if msg := votehandler.ValidateReply(req.Action, req.Reason, req.VoteID, req.VoteParticipantID); msg != "" {
		response.FailWithMsg(c, errcode.SystemValidationError, msg)
		return
	}
	if !middleware.RequireNodeAccess(c, req.VoteParticipantID) {
		return
	}
	if err := h.svc.Reply(nodeReqCtx(c), req.Action, req.Reason, req.VoteParticipantID, req.VoteID); err != nil {
		failNodeErr(c, err)
		return
	}
	response.OKEmpty(c)
}

// Pending is POST message/pending: number of invites waiting for ownerId.
func (h *MessageHandler) Pending(c *gin.Context) {
	var req struct {
		OwnerID string `json:"ownerId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.OwnerID == "" {
		response.FailWithMsg(c, errcode.SystemValidationError, "ownerId must not be blank")
		return
	}
	if !middleware.RequireNodeAccess(c, req.OwnerID) {
		return
	}
	n, err := h.svc.PendingCount(nodeReqCtx(c), req.OwnerID)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, n)
}
