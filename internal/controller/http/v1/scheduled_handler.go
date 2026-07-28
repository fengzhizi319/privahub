package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// ScheduledHandler handles scheduled task HTTP requests.
type ScheduledHandler struct {
	scheduledService *service.ScheduledService
}

// NewScheduledHandler creates a new ScheduledHandler.
func NewScheduledHandler(scheduledService *service.ScheduledService) *ScheduledHandler {
	return &ScheduledHandler{scheduledService: scheduledService}
}

// Create handles scheduled task creation.
func (h *ScheduledHandler) Create(c *gin.Context) {
	var req service.CreateScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.scheduledService.Create(c.Request.Context(), &req)
	if err != nil {
		if err == service.ErrInvalidCron {
			response.FailWithMsg(c, errcode.ParamError, "invalid cron expression")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// List handles scheduled task list retrieval.
func (h *ScheduledHandler) List(c *gin.Context) {
	var req struct {
		ProjectID string `json:"project_id"`
	}
	_ = c.ShouldBindJSON(&req)

	tasks, err := h.scheduledService.List(c.Request.Context(), req.ProjectID)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"list": tasks})
}

// Delete handles scheduled task deletion.
func (h *ScheduledHandler) Delete(c *gin.Context) {
	var req struct {
		ScheduleTaskID string `json:"schedule_task_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.Delete(c.Request.Context(), req.ScheduleTaskID); err != nil {
		if err == service.ErrScheduledNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Pause handles scheduled task pause.
func (h *ScheduledHandler) Pause(c *gin.Context) {
	var req struct {
		ScheduleTaskID string `json:"schedule_task_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.Pause(c.Request.Context(), req.ScheduleTaskID); err != nil {
		if err == service.ErrScheduledNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Resume handles scheduled task resume.
func (h *ScheduledHandler) Resume(c *gin.Context) {
	var req struct {
		ScheduleTaskID string `json:"schedule_task_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.Resume(c.Request.Context(), req.ScheduleTaskID); err != nil {
		if err == service.ErrScheduledNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// Offline handles scheduled task offline.
func (h *ScheduledHandler) Offline(c *gin.Context) {
	var req struct {
		ScheduleTaskID string `json:"schedule_task_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.Offline(c.Request.Context(), req.ScheduleTaskID); err != nil {
		if err == service.ErrScheduledNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// GraphCreate handles scheduled graph creation (frontend contract).
func (h *ScheduledHandler) GraphCreate(c *gin.Context) {
	var req service.ScheduledGraphCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.CreateScheduledGraph(c.Request.Context(), &req); err != nil {
		if err == service.ErrInvalidCron {
			response.FailWithMsg(c, errcode.ParamError, "invalid cron config")
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// GetID handles schedule id retrieval.
func (h *ScheduledHandler) GetID(c *gin.Context) {
	var req service.ScheduledIdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	id, err := h.scheduledService.GetScheduledID(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, id)
}

// Info handles schedule info retrieval.
func (h *ScheduledHandler) Info(c *gin.Context) {
	var req service.ScheduledInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.scheduledService.GetScheduledInfo(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}

// TaskPage handles scheduled task page retrieval.
func (h *ScheduledHandler) TaskPage(c *gin.Context) {
	var req service.TaskPageScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	list, err := h.scheduledService.GetScheduledTaskPage(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"list": list})
}

// TaskReRun handles scheduled task rerun.
func (h *ScheduledHandler) TaskReRun(c *gin.Context) {
	var req service.TaskReRunScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.ReRunScheduledTask(c.Request.Context(), &req); err != nil {
		if err == service.ErrScheduledNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// TaskStop handles scheduled task stop.
func (h *ScheduledHandler) TaskStop(c *gin.Context) {
	var req service.TaskStopScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	if err := h.scheduledService.StopScheduledTask(c.Request.Context(), &req); err != nil {
		if err == service.ErrScheduledNotFound {
			response.Fail(c, errcode.NotFound)
			return
		}
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OKEmpty(c)
}

// OnceSuccess handles once-success query.
func (h *ScheduledHandler) OnceSuccess(c *gin.Context) {
	var req service.ScheduledGraphOnceSuccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	ok, err := h.scheduledService.GetScheduledOnceSuccess(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, ok)
}

// JobList handles scheduled job list retrieval.
func (h *ScheduledHandler) JobList(c *gin.Context) {
	var req service.ScheduleListProjectJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	jobs, err := h.scheduledService.GetScheduledJobs(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, gin.H{"list": jobs})
}

// TaskInfo handles scheduled task info retrieval.
func (h *ScheduledHandler) TaskInfo(c *gin.Context) {
	var req service.TaskInfoScheduledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}

	vo, err := h.scheduledService.GetScheduledTaskInfo(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, errcode.SystemError)
		return
	}

	response.OK(c, vo)
}
