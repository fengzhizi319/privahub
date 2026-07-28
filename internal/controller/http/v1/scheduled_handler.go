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

	response.OK(c, tasks)
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
