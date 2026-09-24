package v1

import (
	"errors"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// ScheduledHandler serves /api/v1alpha1/scheduled/* (Java ScheduledController).
type ScheduledHandler struct {
	scheduledService *service.ScheduledService
}

// NewScheduledHandler creates a new ScheduledHandler.
func NewScheduledHandler(scheduledService *service.ScheduledService) *ScheduledHandler {
	return &ScheduledHandler{scheduledService: scheduledService}
}

// failScheduled maps service errors to Java-compatible error responses.
func failScheduled(c *gin.Context, err error) {
	var se *service.ScheduledError
	if errors.As(err, &se) {
		switch {
		case se.Msg == "":
			response.Fail(c, se.Code)
		case se.Code.HasPlaceholder():
			response.FailArgs(c, se.Code, se.Msg)
		default:
			response.FailWithMsg(c, se.Code, response.Message(c, se.Code)+": "+se.Msg)
		}
		return
	}
	response.FailWithMsg(c, errcode.SystemError, err.Error())
}

func bindScheduled(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		response.FailWithMsg(c, errcode.ParamError, err.Error())
		return false
	}
	return true
}

func requireFields(c *gin.Context, fields map[string]string) bool {
	for name, v := range fields {
		if v == "" {
			response.FailWithMsg(c, errcode.ParamError, name+" must not be blank")
			return false
		}
	}
	return true
}

// GetID handles POST /scheduled/id: generates a new schedule id.
func (h *ScheduledHandler) GetID(c *gin.Context) {
	var req service.ScheduledIdRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"projectId": req.ProjectID, "graphId": req.GraphID}) {
		return
	}
	id, err := h.scheduledService.BuildSchedulerID(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, id)
}

// OnceSuccess handles POST /scheduled/graph/once/success.
func (h *ScheduledHandler) OnceSuccess(c *gin.Context) {
	var req service.ScheduledGraphOnceSuccessRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"projectId": req.ProjectID, "graphId": req.GraphID}) {
		return
	}
	ok, err := h.scheduledService.OnceSuccess(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, ok)
}

// GraphCreate handles POST /scheduled/graph/create.
func (h *ScheduledHandler) GraphCreate(c *gin.Context) {
	var req service.ScheduledGraphCreateRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"projectId": req.ProjectID, "graphId": req.GraphID}) {
		return
	}
	if err := h.scheduledService.CreateScheduler(c.Request.Context(), &req); err != nil {
		failScheduled(c, err)
		return
	}
	response.OKEmpty(c)
}

// Page handles POST /scheduled/page.
func (h *ScheduledHandler) Page(c *gin.Context) {
	var req service.PageScheduledRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"projectId": req.ProjectID}) {
		return
	}
	page, err := h.scheduledService.QueryPage(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, page)
}

// Offline handles POST /scheduled/offline.
func (h *ScheduledHandler) Offline(c *gin.Context) {
	var req service.ScheduledOfflineRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"scheduleId": req.ScheduleID}) {
		return
	}
	if err := h.scheduledService.Offline(c.Request.Context(), &req); err != nil {
		failScheduled(c, err)
		return
	}
	response.OKEmpty(c)
}

// Del handles POST /scheduled/del.
func (h *ScheduledHandler) Del(c *gin.Context) {
	var req service.ScheduledDelRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"scheduleId": req.ScheduleID}) {
		return
	}
	if err := h.scheduledService.Del(c.Request.Context(), &req); err != nil {
		failScheduled(c, err)
		return
	}
	response.OKEmpty(c)
}

// Info handles POST /scheduled/info.
func (h *ScheduledHandler) Info(c *gin.Context) {
	var req service.ScheduledInfoRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"scheduleId": req.ScheduleID}) {
		return
	}
	vo, err := h.scheduledService.Info(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, vo)
}

// TaskPage handles POST /scheduled/task/page.
func (h *ScheduledHandler) TaskPage(c *gin.Context) {
	var req service.TaskPageScheduledRequest
	if !bindScheduled(c, &req) {
		return
	}
	page, err := h.scheduledService.TaskPage(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, page)
}

// TaskStop handles POST /scheduled/task/stop.
func (h *ScheduledHandler) TaskStop(c *gin.Context) {
	var req service.TaskStopScheduledRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"scheduleId": req.ScheduleID, "scheduleTaskId": req.ScheduleTaskID}) {
		return
	}
	if err := h.scheduledService.TaskStop(c.Request.Context(), &req); err != nil {
		failScheduled(c, err)
		return
	}
	response.OKEmpty(c)
}

// TaskReRun handles POST /scheduled/task/rerun.
func (h *ScheduledHandler) TaskReRun(c *gin.Context) {
	var req service.TaskReRunScheduledRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"scheduleId": req.ScheduleID, "scheduleTaskId": req.ScheduleTaskID}) {
		return
	}
	if req.Type != "" && req.Type != service.ScheduledRestartTypeAll && req.Type != service.ScheduledRestartTypeFailed {
		response.FailWithMsg(c, errcode.ParamError, "type must be one of 0, 1")
		return
	}
	if err := h.scheduledService.TaskRerun(c.Request.Context(), &req); err != nil {
		failScheduled(c, err)
		return
	}
	response.OKEmpty(c)
}

// TaskInfo handles POST /scheduled/task/info.
func (h *ScheduledHandler) TaskInfo(c *gin.Context) {
	var req service.TaskInfoScheduledRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"scheduleId": req.ScheduleID, "scheduleTaskId": req.ScheduleTaskID}) {
		return
	}
	vo, err := h.scheduledService.TaskInfo(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, vo)
}

// JobList handles POST /scheduled/job/list. Java requires scheduleTaskId (one
// run); Go also accepts it blank to list the jobs of every run of the
// schedule's graph (the periodic-task detail page of the whole schedule).
func (h *ScheduledHandler) JobList(c *gin.Context) {
	var req service.ScheduleListProjectJobRequest
	if !bindScheduled(c, &req) || !requireFields(c, map[string]string{"projectId": req.ProjectID, "graphId": req.GraphID}) {
		return
	}
	page, err := h.scheduledService.ListProjectJob(c.Request.Context(), &req)
	if err != nil {
		failScheduled(c, err)
		return
	}
	response.OK(c, page)
}
