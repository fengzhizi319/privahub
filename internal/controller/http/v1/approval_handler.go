package v1

import (
	"strings"

	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/service/votehandler"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// ApprovalHandler is Java ApprovalController (/api/v1alpha1/approval/*).
type ApprovalHandler struct {
	svc *votehandler.Service
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(svc *votehandler.Service) *ApprovalHandler {
	return &ApprovalHandler{svc: svc}
}

// Create is POST approval/create {initiatorId, voteType, voteConfig}
// (@DataResource initiatorId NODE_ID). Data is empty like Java; the new vote
// id is returned as a Go extension field.
func (h *ApprovalHandler) Create(c *gin.Context) {
	var req struct {
		InitiatorID string                  `json:"initiatorId"`
		VoteType    string                  `json:"voteType"`
		VoteConfig  *votehandler.VoteConfig `json:"voteConfig"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.SystemValidationError, "invalid request body")
		return
	}
	// Java CreateApprovalRequest: @NotBlank initiatorId, @OneOfType voteType
	// and the @Valid per-type voteConfig.
	msg := ""
	if strings.TrimSpace(req.InitiatorID) == "" {
		msg = "initiatorId must not be blank"
	} else if msg = votehandler.ValidateVoteType(req.VoteType); msg == "" {
		msg = votehandler.ValidateVoteConfig(req.VoteType, req.VoteConfig)
	}
	if msg != "" {
		response.FailWithMsg(c, errcode.SystemValidationError, msg)
		return
	}
	if !middleware.RequireNodeAccess(c, req.InitiatorID) {
		return
	}
	voteID, err := h.svc.CreateApproval(nodeReqCtx(c), req.InitiatorID, req.VoteType, req.VoteConfig)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, gin.H{"voteId": voteID})
}

// PullStatus is POST approval/pull/status {projectID, jobID, taskID?, resourceID, resourceType}.
// Java PullStatusRequest also requires taskID; Go resolves an empty taskID
// from the project result (the result list rows carry no task id).
func (h *ApprovalHandler) PullStatus(c *gin.Context) {
	var req struct {
		ProjectID    string `json:"projectID"`
		JobID        string `json:"jobID"`
		TaskID       string `json:"taskID"`
		ResourceType string `json:"resourceType"`
		ResourceID   string `json:"resourceID"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMsg(c, errcode.SystemValidationError, "invalid request body")
		return
	}
	if msg := votehandler.ValidatePullStatus(req.ProjectID, req.JobID, req.ResourceID, req.ResourceType); msg != "" {
		response.FailWithMsg(c, errcode.SystemValidationError, msg)
		return
	}
	vo, err := h.svc.PullStatus(nodeReqCtx(c), req.ProjectID, req.JobID, req.TaskID, req.ResourceID, req.ResourceType)
	if err != nil {
		failNodeErr(c, err)
		return
	}
	response.OK(c, vo)
}
