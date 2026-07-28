package model

import "time"

// ProjectJobDO represents a Kuscia job entity.
type ProjectJobDO struct {
	BaseDO
	ProjectID    string     `gorm:"uniqueIndex:upk_project_job_id;type:varchar(64);not null" json:"project_id"`
	JobID        string     `gorm:"uniqueIndex:upk_project_job_id;uniqueIndex:upk_job_id;type:varchar(64);not null" json:"job_id"`
	Name         string     `gorm:"type:varchar(40);not null" json:"name"`
	Status       string     `gorm:"type:varchar(32);not null" json:"status"`
	ErrMsg       string     `gorm:"type:text" json:"err_msg"`
	GraphID      string     `gorm:"type:varchar(64)" json:"graph_id"`
	Edges        string     `gorm:"type:text" json:"edges"`
	FinishedTime *time.Time `json:"finished_time"`
}

func (ProjectJobDO) TableName() string { return "project_job" }

// ProjectJobTaskDO represents a task within a Kuscia job.
type ProjectJobTaskDO struct {
	BaseDO
	ProjectID   string `gorm:"uniqueIndex:upk_project_job_task_id;type:varchar(64);not null" json:"project_id"`
	JobID       string `gorm:"uniqueIndex:upk_project_job_task_id;type:varchar(64);not null" json:"job_id"`
	TaskID      string `gorm:"uniqueIndex:upk_project_job_task_id;type:varchar(64);not null" json:"task_id"`
	Parties     string `gorm:"type:text;not null" json:"parties"`
	Status      string `gorm:"type:varchar(32);not null" json:"status"`
	ErrMsg      string `gorm:"type:text" json:"err_msg"`
	GraphNodeID string `gorm:"type:varchar(64)" json:"graph_node_id"`
	GraphNode   string `gorm:"type:text" json:"graph_node"`
}

func (ProjectJobTaskDO) TableName() string { return "project_job_task" }

// ProjectJobTaskLogDO stores execution logs for a task.
type ProjectJobTaskLogDO struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID string    `gorm:"index:idx_project_job_task_log;type:varchar(64);not null" json:"project_id"`
	JobID     string    `gorm:"index:idx_project_job_task_log;type:varchar(64);not null" json:"job_id"`
	TaskID    string    `gorm:"index:idx_project_job_task_log;type:varchar(64);not null" json:"task_id"`
	Log       string    `gorm:"type:text;not null" json:"log"`
	GmtCreate time.Time `gorm:"autoCreateTime" json:"gmt_create"`
}

func (ProjectJobTaskLogDO) TableName() string { return "project_job_task_log" }
