package service

import (
	"context"
	"errors"
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
