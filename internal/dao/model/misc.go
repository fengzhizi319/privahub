package model

// ProjectRuleDO associates a project with a rule.
type ProjectRuleDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_rule_id;type:varchar(64);not null" json:"project_id"`
	RuleID    string `gorm:"uniqueIndex:upk_project_rule_id;type:varchar(64);not null" json:"rule_id"`
}

func (ProjectRuleDO) TableName() string { return "project_rule" }

// ProjectReportDO associates a project with a report.
type ProjectReportDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_report_id;type:varchar(64);not null" json:"project_id"`
	ReportID  string `gorm:"uniqueIndex:upk_project_report_id;type:varchar(64);not null" json:"report_id"`
	Content   string `gorm:"type:varchar(64);not null" json:"content"`
}

func (ProjectReportDO) TableName() string { return "project_report" }

// ProjectReadDataDO stores data read results for a project.
type ProjectReadDataDO struct {
	BaseDO
	ProjectID  string `gorm:"uniqueIndex:upk_project_read_data_id;type:varchar(64);not null" json:"project_id"`
	OutputID   string `gorm:"type:varchar(64);not null" json:"output_id"`
	ReportID   string `gorm:"uniqueIndex:upk_project_read_data_id;type:varchar(64);not null" json:"report_id"`
	Hash       string `gorm:"type:varchar(64);not null;default:''" json:"hash"`
	Task       string `gorm:"type:varchar(64);not null;default:''" json:"task"`
	GrapNodeID string `gorm:"type:varchar(64);not null;default:''" json:"grap_node_id"`
	Content    string `gorm:"type:varchar(64);not null" json:"content"`
	Raw        string `gorm:"type:varchar(64);not null" json:"raw"`
}

func (ProjectReadDataDO) TableName() string { return "project_read_data" }

// ProjectResultDO stores generated resources (model, fed_table, rule, report) from jobs.
type ProjectResultDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_result_kind_node_ref_id;type:varchar(64);not null" json:"project_id"`
	Kind      string `gorm:"uniqueIndex:upk_project_result_kind_node_ref_id;type:varchar(16);not null" json:"kind"`
	NodeID    string `gorm:"uniqueIndex:upk_project_result_kind_node_ref_id;type:varchar(64);not null" json:"node_id"`
	RefID     string `gorm:"uniqueIndex:upk_project_result_kind_node_ref_id;type:varchar(64);not null" json:"ref_id"`
	JobID     string `gorm:"type:varchar(64)" json:"job_id"`
	TaskID    string `gorm:"type:varchar(64)" json:"task_id"`
}

func (ProjectResultDO) TableName() string { return "project_result" }

// EdgeDataSyncLogDO tracks edge data synchronization state.
type EdgeDataSyncLogDO struct {
	SyncTableName  string `gorm:"primaryKey;column:table_name;type:varchar(64);not null" json:"table_name"`
	LastUpdateTime string `gorm:"type:varchar(64);not null" json:"last_update_time"`
}

func (EdgeDataSyncLogDO) TableName() string { return "edge_data_sync_log" }
