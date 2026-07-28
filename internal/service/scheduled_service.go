package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Scheduled service errors.
var (
	ErrScheduledNotFound = errors.New("scheduled task not found")
	ErrInvalidCron       = errors.New("invalid cron expression")
)

// ScheduledService handles scheduled task management with a cron engine.
type ScheduledService struct {
	db           *gorm.DB
	cron         *cron.Cron
	log          *zap.Logger
	graphService *GraphService
	entries      map[string]cron.EntryID // scheduleTaskID -> cron entry ID
}

// NewScheduledService creates a new ScheduledService and starts the cron engine.
func NewScheduledService(db *gorm.DB, log *zap.Logger, graphService *GraphService) *ScheduledService {
	c := cron.New(cron.WithSeconds())
	s := &ScheduledService{
		db:           db,
		cron:         c,
		log:          log,
		graphService: graphService,
		entries:      make(map[string]cron.EntryID),
	}
	c.Start()
	return s
}

// Stop gracefully stops the cron engine.
func (s *ScheduledService) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

// --- DTOs ---

// CreateScheduledRequest represents a scheduled task creation request.
type CreateScheduledRequest struct {
	ProjectID string `json:"project_id" binding:"required"`
	GraphID   string `json:"graph_id" binding:"required"`
	Cron      string `json:"cron" binding:"required"`
	StartTime string `json:"start_time"` // format: 2006-01-02 15:04:05
}

// ScheduledVO represents a scheduled task view object.
type ScheduledVO struct {
	ScheduleTaskID string  `json:"schedule_task_id"`
	ProjectID      string  `json:"project_id"`
	GraphID        string  `json:"graph_id"`
	Cron           string  `json:"cron"`
	Status         string  `json:"status"`
	ExpectStart    string  `json:"schedule_task_expect_start_time"`
	StartTime      *string `json:"schedule_task_start_time,omitempty"`
	EndTime        *string `json:"schedule_task_end_time,omitempty"`
	GmtCreate      string  `json:"gmt_create"`
}

// --- Service Methods ---

// Create creates a new scheduled task and registers it with the cron engine.
func (s *ScheduledService) Create(ctx context.Context, req *CreateScheduledRequest) (*ScheduledVO, error) {
	// Validate cron expression
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(req.Cron); err != nil {
		return nil, ErrInvalidCron
	}

	taskID := uuid.New().String()[:8]
	scheduleID := uuid.New().String()[:8]

	expectStart := time.Now().Add(time.Minute)
	if req.StartTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", req.StartTime); err == nil {
			expectStart = t
		}
	}

	task := &model.ProjectScheduleTaskDO{
		ProjectID:               req.ProjectID,
		GraphID:                 req.GraphID,
		ScheduleID:              scheduleID,
		ScheduleTaskID:          taskID,
		Cron:                    req.Cron,
		ScheduleTaskExpectStart: expectStart,
		Status:                  model.ScheduledStatusToBeRun,
	}

	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return nil, err
	}

	// Register with cron engine
	s.registerCron(task)

	return s.toVO(task), nil
}

// List lists all scheduled tasks.
func (s *ScheduledService) List(ctx context.Context, projectID string) ([]ScheduledVO, error) {
	var tasks []model.ProjectScheduleTaskDO
	query := s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{})
	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if err := query.Order("gmt_create DESC").Find(&tasks).Error; err != nil {
		return nil, err
	}

	result := make([]ScheduledVO, 0, len(tasks))
	for i := range tasks {
		result = append(result, *s.toVO(&tasks[i]))
	}
	return result, nil
}

// Delete removes a scheduled task and unregisters it from cron.
func (s *ScheduledService) Delete(ctx context.Context, taskID string) error {
	var task model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_task_id = ?", taskID).First(&task).Error; err != nil {
		return ErrScheduledNotFound
	}

	// Unregister from cron
	if entryID, ok := s.entries[taskID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, taskID)
	}

	return s.db.WithContext(ctx).Delete(&task).Error
}

// Pause pauses a scheduled task.
func (s *ScheduledService) Pause(ctx context.Context, taskID string) error {
	var task model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_task_id = ?", taskID).First(&task).Error; err != nil {
		return ErrScheduledNotFound
	}

	// Remove from cron engine
	if entryID, ok := s.entries[taskID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, taskID)
	}

	task.Status = model.ScheduledStatusPaused
	return s.db.WithContext(ctx).Save(&task).Error
}

// Resume resumes a paused scheduled task.
func (s *ScheduledService) Resume(ctx context.Context, taskID string) error {
	var task model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_task_id = ?", taskID).First(&task).Error; err != nil {
		return ErrScheduledNotFound
	}

	task.Status = model.ScheduledStatusToBeRun
	if err := s.db.WithContext(ctx).Save(&task).Error; err != nil {
		return err
	}

	s.registerCron(&task)
	return nil
}

// Offline marks a scheduled task as offline (permanently stopped).
func (s *ScheduledService) Offline(ctx context.Context, taskID string) error {
	var task model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_task_id = ?", taskID).First(&task).Error; err != nil {
		return ErrScheduledNotFound
	}

	if entryID, ok := s.entries[taskID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, taskID)
	}

	task.Status = model.ScheduledStatusOffline
	return s.db.WithContext(ctx).Save(&task).Error
}

// registerCron adds a task to the cron engine.
func (s *ScheduledService) registerCron(task *model.ProjectScheduleTaskDO) {
	taskID := task.ScheduleTaskID
	graphID := task.GraphID
	projectID := task.ProjectID
	db := s.db
	log := s.log

	entryID, err := s.cron.AddFunc(task.Cron, func() {
		log.Info("Scheduled task triggered",
			zap.String("task_id", taskID),
			zap.String("graph_id", graphID),
			zap.String("project_id", projectID),
		)

		ctx := context.Background()
		now := time.Now()

		// Update status to RUNNING
		db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
			Where("schedule_task_id = ?", taskID).
			Updates(map[string]interface{}{
				"status":                   model.ScheduledStatusRunning,
				"schedule_task_start_time": now,
			})

		// Trigger graph execution via GraphService.StartGraph
		if s.graphService != nil {
			_, startErr := s.graphService.StartGraph(ctx, &StartGraphRequest{
				ProjectID: projectID,
				GraphID:   graphID,
			})
			if startErr != nil {
				s.log.Error("Scheduled graph execution failed",
					zap.String("task_id", taskID),
					zap.Error(startErr),
				)
				db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
					Where("schedule_task_id = ?", taskID).
					Updates(map[string]interface{}{
						"status":                 model.ScheduledStatusFailed,
						"schedule_task_end_time": time.Now(),
					})
				return
			}
		}

		// Mark as SUCCESS after execution
		db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
			Where("schedule_task_id = ?", taskID).
			Updates(map[string]interface{}{
				"status":                 model.ScheduledStatusSuccess,
				"schedule_task_end_time": time.Now(),
			})
	})

	if err != nil {
		s.log.Error("Failed to register cron job",
			zap.String("task_id", taskID),
			zap.String("cron", task.Cron),
			zap.Error(err),
		)
		return
	}

	s.entries[taskID] = entryID
}

func (s *ScheduledService) toVO(task *model.ProjectScheduleTaskDO) *ScheduledVO {
	vo := &ScheduledVO{
		ScheduleTaskID: task.ScheduleTaskID,
		ProjectID:      task.ProjectID,
		GraphID:        task.GraphID,
		Cron:           task.Cron,
		Status:         task.Status,
		ExpectStart:    task.ScheduleTaskExpectStart.Format("2006-01-02 15:04:05"),
		GmtCreate:      task.GmtCreate.Format("2006-01-02 15:04:05"),
	}
	if task.ScheduleTaskStartTime != nil {
		st := task.ScheduleTaskStartTime.Format("2006-01-02 15:04:05")
		vo.StartTime = &st
	}
	if task.ScheduleTaskEndTime != nil {
		et := task.ScheduleTaskEndTime.Format("2006-01-02 15:04:05")
		vo.EndTime = &et
	}
	return vo
}

// --- Compat (camelCase) DTOs aligned with the frontend contract ---

// CronCompat represents the structured cron config sent by the frontend.
type CronCompat struct {
	StartTime     string `json:"startTime"`
	EndTime       string `json:"endTime"`
	ScheduleCycle string `json:"scheduleCycle"`
	ScheduleDate  string `json:"scheduleDate"`
	ScheduleTime  string `json:"scheduleTime"`
}

// ScheduledGraphCreateRequest represents a scheduled graph creation request (frontend contract).
type ScheduledGraphCreateRequest struct {
	ScheduleID   string     `json:"scheduleId"`
	ScheduleDesc string     `json:"scheduleDesc"`
	Cron         CronCompat `json:"cron"`
	ProjectID    string     `json:"projectId" binding:"required"`
	GraphID      string     `json:"graphId" binding:"required"`
	Nodes        []string   `json:"nodes"`
}

// ScheduledIdRequest represents a schedule-id query request.
type ScheduledIdRequest struct {
	ProjectID string `json:"projectId" binding:"required"`
	GraphID   string `json:"graphId" binding:"required"`
}

// ScheduledInfoRequest represents a schedule-info query request.
type ScheduledInfoRequest struct {
	ScheduleID string `json:"scheduleId" binding:"required"`
}

// TaskPageScheduledRequest represents a scheduled task page request.
type TaskPageScheduledRequest struct {
	ScheduleID string `json:"scheduleId" binding:"required"`
	Page       int    `json:"page"`
	Size       int    `json:"size"`
}

// TaskPageScheduledCompatVO represents a scheduled task page entry (frontend contract).
type TaskPageScheduledCompatVO struct {
	ScheduleTaskID              string `json:"scheduleTaskId"`
	ScheduleTaskExpectStartTime string `json:"scheduleTaskExpectStartTime"`
	ScheduleTaskStartTime       string `json:"scheduleTaskStartTime,omitempty"`
	ScheduleTaskEndTime         string `json:"scheduleTaskEndTime,omitempty"`
	ScheduleTaskStatus          string `json:"scheduleTaskStatus"`
	AllReRun                    bool   `json:"allReRun"`
}

// TaskReRunScheduledRequest represents a scheduled task rerun request.
type TaskReRunScheduledRequest struct {
	ScheduleID     string `json:"scheduleId" binding:"required"`
	ScheduleTaskID string `json:"scheduleTaskId" binding:"required"`
	Type           string `json:"type"`
}

// TaskStopScheduledRequest represents a scheduled task stop request.
type TaskStopScheduledRequest struct {
	ScheduleID     string `json:"scheduleId" binding:"required"`
	ScheduleTaskID string `json:"scheduleTaskId" binding:"required"`
}

// ScheduledGraphOnceSuccessRequest represents a once-success query request.
type ScheduledGraphOnceSuccessRequest struct {
	ProjectID string `json:"projectId" binding:"required"`
	GraphID   string `json:"graphId" binding:"required"`
}

// ScheduleListProjectJobRequest represents a scheduled job list request.
type ScheduleListProjectJobRequest struct {
	ProjectID      string `json:"projectId" binding:"required"`
	GraphID        string `json:"graphId" binding:"required"`
	ScheduleTaskID string `json:"scheduleTaskId"`
	PageNum        int    `json:"pageNum"`
	PageSize       int    `json:"pageSize"`
}

// TaskInfoScheduledRequest represents a scheduled task info request.
type TaskInfoScheduledRequest struct {
	ScheduleID     string `json:"scheduleId" binding:"required"`
	ScheduleTaskID string `json:"scheduleTaskId" binding:"required"`
}

// ProjectJobCompatVO represents a project job view object (frontend contract).
type ProjectJobCompatVO struct {
	JobID       string `json:"jobId,omitempty"`
	Status      string `json:"status,omitempty"`
	ErrMsg      string `json:"errMsg,omitempty"`
	GmtCreate   string `json:"gmtCreate,omitempty"`
	GmtModified string `json:"gmtModified,omitempty"`
	GmtFinished string `json:"gmtFinished,omitempty"`
	Finished    bool   `json:"finished"`
}

// --- Compat Service Methods ---

// buildCronExpression converts the structured frontend cron config into a 6-field cron expression.
func buildCronExpression(c CronCompat) string {
	hour, minute := 0, 0
	if c.ScheduleTime != "" {
		parts := strings.Split(c.ScheduleTime, ":")
		if len(parts) >= 2 {
			if h, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				hour = h
			}
			if m, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				minute = m
			}
		}
	}

	date := strings.TrimSpace(c.ScheduleDate)
	if date == "" {
		date = "*"
	}

	switch strings.ToUpper(c.ScheduleCycle) {
	case "HOUR":
		return fmt.Sprintf("0 %d * * * *", minute)
	case "WEEK":
		return fmt.Sprintf("0 %d %d * * %s", minute, hour, date)
	case "MONTH":
		return fmt.Sprintf("0 %d %d %s * *", minute, hour, date)
	default: // DAY or unspecified
		return fmt.Sprintf("0 %d %d * * *", minute, hour)
	}
}

// CreateScheduledGraph creates a schedule from a graph using a structured cron config.
func (s *ScheduledService) CreateScheduledGraph(ctx context.Context, req *ScheduledGraphCreateRequest) error {
	cronExpr := buildCronExpression(req.Cron)

	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(cronExpr); err != nil {
		return ErrInvalidCron
	}

	scheduleID := req.ScheduleID
	if scheduleID == "" {
		scheduleID = uuid.New().String()[:8]
	}
	taskID := uuid.New().String()[:8]

	task := &model.ProjectScheduleTaskDO{
		ProjectID:               req.ProjectID,
		GraphID:                 req.GraphID,
		ScheduleID:              scheduleID,
		ScheduleTaskID:          taskID,
		Cron:                    cronExpr,
		ScheduleTaskExpectStart: time.Now().Add(time.Minute),
		Status:                  model.ScheduledStatusToBeRun,
	}

	if err := s.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}

	s.registerCron(task)
	return nil
}

// GetScheduledID returns the latest schedule id for a project+graph.
func (s *ScheduledService) GetScheduledID(ctx context.Context, req *ScheduledIdRequest) (string, error) {
	var task model.ProjectScheduleTaskDO
	err := s.db.WithContext(ctx).
		Where("project_id = ? AND graph_id = ?", req.ProjectID, req.GraphID).
		Order("gmt_create DESC").
		First(&task).Error
	if err != nil {
		return "", nil // degrade: no schedule yet
	}
	return task.ScheduleID, nil
}

// GetScheduledInfo returns job info for the latest task of a schedule.
func (s *ScheduledService) GetScheduledInfo(ctx context.Context, req *ScheduledInfoRequest) (*ProjectJobCompatVO, error) {
	var task model.ProjectScheduleTaskDO
	err := s.db.WithContext(ctx).
		Where("schedule_id = ?", req.ScheduleID).
		Order("gmt_create DESC").
		First(&task).Error
	if err != nil {
		return &ProjectJobCompatVO{}, nil
	}
	return s.taskToJobVO(&task), nil
}

// GetScheduledTaskPage lists task executions for a schedule, paginated.
func (s *ScheduledService) GetScheduledTaskPage(ctx context.Context, req *TaskPageScheduledRequest) ([]TaskPageScheduledCompatVO, error) {
	size := req.Size
	if size <= 0 {
		size = 100
	}
	page := req.Page
	if page < 1 {
		page = 1
	}

	var tasks []model.ProjectScheduleTaskDO
	err := s.db.WithContext(ctx).
		Where("schedule_id = ?", req.ScheduleID).
		Order("gmt_create DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}

	result := make([]TaskPageScheduledCompatVO, 0, len(tasks))
	for i := range tasks {
		result = append(result, s.taskToPageVO(&tasks[i]))
	}
	return result, nil
}

// ReRunScheduledTask re-triggers a scheduled task execution.
func (s *ScheduledService) ReRunScheduledTask(ctx context.Context, req *TaskReRunScheduledRequest) error {
	var task model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_task_id = ?", req.ScheduleTaskID).First(&task).Error; err != nil {
		return ErrScheduledNotFound
	}

	now := time.Now()
	task.Status = model.ScheduledStatusRunning
	task.ScheduleTaskStartTime = &now
	task.ScheduleTaskEndTime = nil
	if err := s.db.WithContext(ctx).Save(&task).Error; err != nil {
		return err
	}

	if s.graphService != nil {
		if _, err := s.graphService.StartGraph(ctx, &StartGraphRequest{ProjectID: task.ProjectID, GraphID: task.GraphID}); err != nil {
			end := time.Now()
			task.Status = model.ScheduledStatusFailed
			task.ScheduleTaskEndTime = &end
			_ = s.db.WithContext(ctx).Save(&task).Error
			return nil // degrade: request accepted
		}
	}

	end := time.Now()
	task.Status = model.ScheduledStatusSuccess
	task.ScheduleTaskEndTime = &end
	_ = s.db.WithContext(ctx).Save(&task).Error
	return nil
}

// StopScheduledTask stops a scheduled task execution.
func (s *ScheduledService) StopScheduledTask(ctx context.Context, req *TaskStopScheduledRequest) error {
	var task model.ProjectScheduleTaskDO
	if err := s.db.WithContext(ctx).Where("schedule_task_id = ?", req.ScheduleTaskID).First(&task).Error; err != nil {
		return ErrScheduledNotFound
	}

	if entryID, ok := s.entries[task.ScheduleTaskID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, task.ScheduleTaskID)
	}

	task.Status = model.ScheduledStatusOffline
	return s.db.WithContext(ctx).Save(&task).Error
}

// GetScheduledOnceSuccess reports whether a graph has at least one successful run.
func (s *ScheduledService) GetScheduledOnceSuccess(ctx context.Context, req *ScheduledGraphOnceSuccessRequest) (bool, error) {
	var count int64
	_ = s.db.WithContext(ctx).Model(&model.ProjectScheduleTaskDO{}).
		Where("project_id = ? AND graph_id = ? AND status = ?", req.ProjectID, req.GraphID, model.ScheduledStatusSuccess).
		Count(&count).Error
	return count > 0, nil
}

// GetScheduledJobs lists jobs for a scheduled task (degrades to empty).
func (s *ScheduledService) GetScheduledJobs(ctx context.Context, req *ScheduleListProjectJobRequest) ([]ProjectJobCompatVO, error) {
	// Individual jobs are not tracked per schedule task in this implementation.
	return make([]ProjectJobCompatVO, 0), nil
}

// GetScheduledTaskInfo returns job info for a specific scheduled task execution.
func (s *ScheduledService) GetScheduledTaskInfo(ctx context.Context, req *TaskInfoScheduledRequest) (*ProjectJobCompatVO, error) {
	var task model.ProjectScheduleTaskDO
	err := s.db.WithContext(ctx).
		Where("schedule_id = ? AND schedule_task_id = ?", req.ScheduleID, req.ScheduleTaskID).
		First(&task).Error
	if err != nil {
		err = s.db.WithContext(ctx).Where("schedule_task_id = ?", req.ScheduleTaskID).First(&task).Error
		if err != nil {
			return &ProjectJobCompatVO{}, nil
		}
	}
	return s.taskToJobVO(&task), nil
}

func (s *ScheduledService) taskToJobVO(task *model.ProjectScheduleTaskDO) *ProjectJobCompatVO {
	vo := &ProjectJobCompatVO{
		JobID:     task.ScheduleTaskID,
		Status:    task.Status,
		GmtCreate: task.GmtCreate.Format("2006-01-02 15:04:05"),
	}
	vo.Finished = task.Status == model.ScheduledStatusSuccess ||
		task.Status == model.ScheduledStatusFailed ||
		task.Status == model.ScheduledStatusOffline
	if task.ScheduleTaskEndTime != nil {
		vo.GmtFinished = task.ScheduleTaskEndTime.Format("2006-01-02 15:04:05")
	}
	return vo
}

func (s *ScheduledService) taskToPageVO(task *model.ProjectScheduleTaskDO) TaskPageScheduledCompatVO {
	vo := TaskPageScheduledCompatVO{
		ScheduleTaskID:              task.ScheduleTaskID,
		ScheduleTaskExpectStartTime: task.ScheduleTaskExpectStart.Format("2006-01-02 15:04:05"),
		ScheduleTaskStatus:          task.Status,
	}
	if task.ScheduleTaskStartTime != nil {
		vo.ScheduleTaskStartTime = task.ScheduleTaskStartTime.Format("2006-01-02 15:04:05")
	}
	if task.ScheduleTaskEndTime != nil {
		vo.ScheduleTaskEndTime = task.ScheduleTaskEndTime.Format("2006-01-02 15:04:05")
	}
	return vo
}
