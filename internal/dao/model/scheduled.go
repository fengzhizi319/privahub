package model

import "time"

// ProjectScheduleDO mirrors Java ProjectScheduleDO (table project_schedule):
// one periodic schedule created from a graph.
//
// JSON columns keep the Java converters' semantics:
//   - Cron:      JSON array of Quartz style expressions
//   - Request:   JSON of the original scheduled/graph/create request
//   - GraphInfo: JSON snapshot of the graph (+ nodes) at creation time
//   - JobInfo:   JSON snapshot of the graph's latest ProjectJob (+ tasks)
type ProjectScheduleDO struct {
	BaseDO
	ScheduleID   string    `gorm:"uniqueIndex:upk_project_schedule_id;type:varchar(64);not null" json:"schedule_id"`
	ScheduleDesc string    `gorm:"type:varchar(256)" json:"schedule_desc"`
	Cron         string    `gorm:"type:text" json:"cron"`
	Request      string    `gorm:"type:text" json:"request"`
	GraphInfo    string    `gorm:"type:text" json:"graph_info"`
	GraphJobID   string    `gorm:"type:varchar(64)" json:"graph_job_id"`
	JobInfo      string    `gorm:"type:text" json:"job_info"`
	Owner        string    `gorm:"type:varchar(64)" json:"owner"`
	Creator      string    `gorm:"type:varchar(64)" json:"creator"`
	Status       string    `gorm:"type:varchar(32);not null;default:'UP'" json:"status"`
	ProjectID    string    `gorm:"type:varchar(64);not null;index:idx_project_schedule_project" json:"project_id"`
	GraphID      string    `gorm:"type:varchar(64);not null" json:"graph_id"`
	CreateTime   time.Time `json:"create_time"`
}

// TableName returns the table name for GORM.
func (ProjectScheduleDO) TableName() string { return "project_schedule" }

// ProjectScheduleJobDO mirrors Java ProjectScheduleJobDO (table
// project_schedule_job): the pre-rendered job of one schedule task. Its
// tasks live in project_job_task keyed by (project_id, job_id), exactly like
// Java's shared ProjectTaskDO join.
type ProjectScheduleJobDO struct {
	BaseDO
	ProjectID      string     `gorm:"uniqueIndex:upk_project_schedule_job_id;type:varchar(64);not null" json:"project_id"`
	JobID          string     `gorm:"uniqueIndex:upk_project_schedule_job_id;index:idx_project_schedule_job_job;type:varchar(64);not null" json:"job_id"`
	Name           string     `gorm:"type:varchar(128);not null" json:"name"`
	Status         string     `gorm:"type:varchar(32);not null" json:"status"`
	ErrMsg         string     `gorm:"type:text" json:"err_msg"`
	GraphID        string     `gorm:"type:varchar(64)" json:"graph_id"`
	Edges          string     `gorm:"type:text" json:"edges"`
	FinishedTime   *time.Time `json:"finished_time"`
	Owner          string     `gorm:"type:varchar(64)" json:"owner"`
	ScheduleTaskID string     `gorm:"type:varchar(64);index:idx_project_schedule_job_task" json:"schedule_task_id"`
}

// TableName returns the table name for GORM.
func (ProjectScheduleJobDO) TableName() string { return "project_schedule_job" }

// ProjectScheduleTaskDO mirrors Java ProjectScheduleTaskDO (table
// project_schedule_task): one concrete trigger of a schedule.
type ProjectScheduleTaskDO struct {
	BaseDO
	ProjectID               string     `gorm:"type:varchar(64);not null;index" json:"project_id"`
	GraphID                 string     `gorm:"type:varchar(64);not null" json:"graph_id"`
	ScheduleJobID           string     `gorm:"type:varchar(64);index:idx_project_schedule_task_job" json:"schedule_job_id"`
	ScheduleID              string     `gorm:"type:varchar(64);index" json:"schedule_id"`
	ScheduleTaskID          string     `gorm:"uniqueIndex;type:varchar(64);not null" json:"schedule_task_id"`
	Cron                    string     `gorm:"type:varchar(128);not null" json:"cron"`
	ScheduleTaskExpectStart time.Time  `gorm:"column:schedule_task_expect_start;not null;index:idx_project_schedule_task_due" json:"schedule_task_expect_start_time"`
	ScheduleTaskStartTime   *time.Time `json:"schedule_task_start_time,omitempty"`
	ScheduleTaskEndTime     *time.Time `json:"schedule_task_end_time,omitempty"`
	Status                  string     `gorm:"type:varchar(32);not null;default:'TO_BE_RUN';index:idx_project_schedule_task_due" json:"status"`
	Owner                   string     `gorm:"type:varchar(64)" json:"owner"`
	Creator                 string     `gorm:"type:varchar(64)" json:"creator"`
	// JobRequest is base64(JSON(kuscia.CreateJobRequest)) pre-rendered at
	// schedule creation (Java stored base64(protobuf)).
	JobRequest string `gorm:"type:text" json:"job_request"`
	AllReRun   bool   `gorm:"not null;default:false" json:"all_re_run"`
}

// TableName returns the table name for GORM.
func (ProjectScheduleTaskDO) TableName() string { return "project_schedule_task" }

// Schedule / schedule task status values (Java ScheduledStatus enum names).
const (
	ScheduledStatusUp       = "UP"
	ScheduledStatusDown     = "DOWN"
	ScheduledStatusToBeRun  = "TO_BE_RUN"
	ScheduledStatusRunning  = "RUNNING"
	ScheduledStatusStopped  = "STOPPED"
	ScheduledStatusStopping = "STOPPING"
	ScheduledStatusSucceed  = "SUCCEED"
	ScheduledStatusFailed   = "FAILED"
)

// Schedule job status values (Java GraphJobStatus enum names).
const (
	ScheduleJobStatusRunning = "RUNNING"
	ScheduleJobStatusStopped = "STOPPED"
	ScheduleJobStatusSucceed = "SUCCEED"
	ScheduleJobStatusFailed  = "FAILED"
)

// ScheduledStatusFrom mirrors Java ScheduledStatus.from: normalizes legacy /
// job status strings to a schedule task status.
func ScheduledStatusFrom(v string) string {
	switch upper(v) {
	case "UP":
		return ScheduledStatusUp
	case "DOWN":
		return ScheduledStatusDown
	case "TO_BE_RUN":
		return ScheduledStatusToBeRun
	case "FAILED":
		return ScheduledStatusFailed
	case "STOPPED":
		return ScheduledStatusStopped
	case "STOPPING":
		return ScheduledStatusStopping
	case "SUCCESS", "SUCCEED", "SUCCEEDED":
		return ScheduledStatusSucceed
	default:
		return ScheduledStatusRunning
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
