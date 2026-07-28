package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// JobHandler handles job-related HTTP requests.
type JobHandler struct {
	jobService *service.JobService
}

// NewJobHandler creates a new JobHandler.
func NewJobHandler(jobService *service.JobService) *JobHandler {
	return &JobHandler{jobService: jobService}
}

// Create handles job creation.
func (h *JobHandler) Create(c *gin.Context) {
	var req service.CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.CreateJobRequest{}
	}

	vo, err := h.jobService.CreateJob(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles job list retrieval.
func (h *JobHandler) List(c *gin.Context) {
	var req service.JobListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.JobListRequest{}
	}

	result, err := h.jobService.ListJobs(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, result)
}

// Detail handles job detail retrieval.
func (h *JobHandler) Detail(c *gin.Context) {
	var req service.GetJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.GetJobRequest{}
	}

	vo, err := h.jobService.GetJob(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrJobNotFound {
			response.Fail(c, errcode.JobNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// Stop handles job stop.
func (h *JobHandler) Stop(c *gin.Context) {
	var req service.StopJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.StopJobRequest{}
	}

	if err := h.jobService.StopJob(c.Request.Context(), &req); err != nil {
		if err == service.ErrJobNotFound {
			response.Fail(c, errcode.JobNotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// TaskLogs handles task log retrieval.
func (h *JobHandler) TaskLogs(c *gin.Context) {
	var req service.GetTaskLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = service.GetTaskLogRequest{}
	}

	vo, err := h.jobService.GetTaskLogs(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}
