package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// VoteHandler handles vote-related HTTP requests.
type VoteHandler struct {
	voteService *service.VoteService
}

// NewVoteHandler creates a new VoteHandler.
func NewVoteHandler(voteService *service.VoteService) *VoteHandler {
	return &VoteHandler{voteService: voteService}
}

// Create handles vote creation.
func (h *VoteHandler) Create(c *gin.Context) {
	var req service.CreateVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.voteService.CreateVote(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles vote list retrieval.
func (h *VoteHandler) List(c *gin.Context) {
	var req service.ListVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body for listing all votes
		req = service.ListVoteRequest{}
	}

	votes, err := h.voteService.ListVotes(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, votes)
}

// Detail handles vote detail retrieval.
func (h *VoteHandler) Detail(c *gin.Context) {
	var req service.GetVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vote, invites, err := h.voteService.GetVoteDetail(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrVoteNotFound {
			response.Fail(c, errcode.VoteNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{
		"vote":    vote,
		"invites": invites,
	})
}

// Reply handles vote reply (AGREE/REJECT).
func (h *VoteHandler) Reply(c *gin.Context) {
	var req service.ReplyVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.voteService.ReplyVote(c.Request.Context(), &req); err != nil {
		if err == service.ErrVoteNotFound {
			response.Fail(c, errcode.VoteNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}
