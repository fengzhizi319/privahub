package v1

import (
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// JobHandler handles project job HTTP requests (Java ProjectController job part).
type JobHandler struct {
	jobService   *service.JobService
	graphService *service.GraphService
}

// NewJobHandler creates a new JobHandler.
func NewJobHandler(jobService *service.JobService) *JobHandler {
	return &JobHandler{jobService: jobService}
}

// WithGraphService wires the graph service used by the legacy job/create route.
func (h *JobHandler) WithGraphService(g *service.GraphService) *JobHandler {
	h.graphService = g
	return h
}

// Create is the Go-only legacy project/job/create route: it runs the graph
// through the Java job chain (graph/start semantics) instead of creating an
// empty hard-coded job.
func (h *JobHandler) Create(c *gin.Context) {
	var req service.StartGraphRequest
	if !bind(c, &req) {
		return
	}
	if h.graphService == nil {
		response.Fail(c, errcode.ProjectJobCreateError)
		return
	}
	vo, err := h.graphService.StartGraph(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, errcode.ProjectJobCreateError)
		return
	}
	response.OK(c, vo)
}

// List handles job list retrieval.
func (h *JobHandler) List(c *gin.Context) {
	var req service.JobListRequest
	if !bind(c, &req) {
		return
	}
	result, err := h.jobService.ListJobs(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, result)
}

// Detail handles job detail retrieval (project/job/get).
func (h *JobHandler) Detail(c *gin.Context) {
	var req service.GetJobRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.jobService.GetJob(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// Stop handles job stop.
func (h *JobHandler) Stop(c *gin.Context) {
	var req service.StopJobRequest
	if !bind(c, &req) {
		return
	}
	if err := h.jobService.StopJob(operatorCtx(c), &req); err != nil {
		failService(c, err, nil)
		return
	}
	response.OKEmpty(c)
}

// TaskLogs handles task log retrieval.
func (h *JobHandler) TaskLogs(c *gin.Context) {
	var req service.GetTaskLogRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.jobService.GetTaskLogs(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}

// TaskOutput handles task output retrieval.
func (h *JobHandler) TaskOutput(c *gin.Context) {
	var req service.GetTaskOutputRequest
	if !bind(c, &req) {
		return
	}
	vo, err := h.jobService.GetTaskOutput(operatorCtx(c), &req)
	if err != nil {
		failService(c, err, nil)
		return
	}
	response.OK(c, vo)
}
