package model

// ProjectModelDO associates a project with a trained model.
type ProjectModelDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_model_id;type:varchar(64);not null" json:"project_id"`
	ModelID   string `gorm:"uniqueIndex:upk_project_model_id;type:varchar(64);not null" json:"model_id"`
}

func (ProjectModelDO) TableName() string { return "project_model" }

// ProjectModelPackDO represents a packaged model with metadata.
type ProjectModelPackDO struct {
	BaseDO
	ProjectID       string `gorm:"type:varchar(64);not null" json:"project_id"`
	ModelID         string `gorm:"uniqueIndex:upk_model_id;type:varchar(64);not null" json:"model_id"`
	Initiator       string `gorm:"type:varchar(64);not null" json:"initiator"`
	ModelName       string `gorm:"type:varchar(256);not null" json:"model_name"`
	ModelDesc       string `gorm:"type:text" json:"model_desc"`
	ModelStats      int8   `gorm:"not null;default:0" json:"model_stats"` // 0:online 1:offline 2:discard 3:deleted
	ServingID       string `gorm:"type:varchar(64)" json:"serving_id"`
	SampleTables    string `gorm:"type:text;not null" json:"sample_tables"`
	ModelList       string `gorm:"type:text;not null" json:"model_list"`
	TrainID         string `gorm:"type:varchar(64);not null" json:"train_id"`
	ModelReportID   string `gorm:"type:varchar(64);not null" json:"model_report_id"`
	GraphDetail     string `gorm:"type:text" json:"graph_detail"`
	ModelDatasource string `gorm:"type:varchar(128);not null" json:"model_datasource"`
}

func (ProjectModelPackDO) TableName() string { return "project_model_pack" }

// ProjectModelServingDO represents a model serving deployment.
type ProjectModelServingDO struct {
	BaseDO
	ProjectID          string `gorm:"type:varchar(64);not null" json:"project_id"`
	ServingID          string `gorm:"uniqueIndex:upk_serving_id;type:varchar(64);not null" json:"serving_id"`
	Initiator          string `gorm:"type:varchar(64);not null" json:"initiator"`
	ServingInputConfig string `gorm:"type:text;not null" json:"serving_input_config"`
	Parties            string `gorm:"type:text" json:"parties"`
	PartyEndpoints     string `gorm:"type:text" json:"party_endpoints"`
	ServingStats       string `gorm:"type:varchar(16)" json:"serving_stats"` // init / success / failed
	ErrorMsg           string `gorm:"type:text" json:"error_msg"`
}

func (ProjectModelServingDO) TableName() string { return "project_model_serving" }
