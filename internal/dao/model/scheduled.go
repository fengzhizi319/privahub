package model

import "time"

// ProjectScheduleTaskDO represents a scheduled task entity.
type ProjectScheduleTaskDO struct {
	BaseDO
	ProjectID               string     `gorm:"type:varchar(64);not null;index" json:"project_id"`
	GraphID                 string     `gorm:"type:varchar(64);not null" json:"graph_id"`
	ScheduleJobID           string     `gorm:"type:varchar(64)" json:"schedule_job_id"`
	ScheduleID              string     `gorm:"type:varchar(64);index" json:"schedule_id"`
	ScheduleTaskID          string     `gorm:"uniqueIndex;type:varchar(64);not null" json:"schedule_task_id"`
	Cron                    string     `gorm:"type:varchar(128);not null" json:"cron"`
	ScheduleTaskExpectStart time.Time  `gorm:"not null" json:"schedule_task_expect_start_time"`
	ScheduleTaskStartTime   *time.Time `json:"schedule_task_start_time,omitempty"`
	ScheduleTaskEndTime     *time.Time `json:"schedule_task_end_time,omitempty"`
	Status                  string     `gorm:"type:varchar(32);not null;default:'TO_BE_RUN'" json:"status"`
}

// TableName returns the table name for GORM.
func (ProjectScheduleTaskDO) TableName() string { return "project_schedule_task" }

// Scheduled status constants.
const (
	ScheduledStatusToBeRun = "TO_BE_RUN"
	ScheduledStatusRunning = "RUNNING"
	ScheduledStatusSuccess = "SUCCESS"
	ScheduledStatusFailed  = "FAILED"
	ScheduledStatusPaused  = "PAUSED"
	ScheduledStatusOffline = "OFFLINE"
)
