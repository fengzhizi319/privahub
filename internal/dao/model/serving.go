package model

// ProjectModelDO associates a project with a trained model.
type ProjectModelDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_model_id;type:varchar(64);not null" json:"project_id"`
	ModelID   string `gorm:"uniqueIndex:upk_project_model_id;type:varchar(64);not null" json:"model_id"`
}

func (ProjectModelDO) TableName() string { return "project_model" }

// Model pack statuses (Java ModelStatsEnum codes).
const (
	ModelStatsInit        int8 = 0
	ModelStatsPublishing  int8 = 1
	ModelStatsPublished   int8 = 2
	ModelStatsOffline     int8 = 3
	ModelStatsPublishFail int8 = 4
	ModelStatsDiscarded   int8 = 5
)

var modelStatsNames = []string{"INIT", "PUBLISHING", "PUBLISHED", "OFFLINE", "PUBLISH_FAIL", "DISCARDED"}

// ModelStatsName mirrors ModelStatsEnum.parse ("" when unknown).
func ModelStatsName(code int8) string {
	if code >= 0 && int(code) < len(modelStatsNames) {
		return modelStatsNames[code]
	}
	return ""
}

// ModelStatsCode mirrors ModelStatsEnum.valueOf(name).getCode().
func ModelStatsCode(name string) (int8, bool) {
	for i, n := range modelStatsNames {
		if n == name {
			return int8(i), true
		}
	}
	return 0, false
}

// ProjectModelPackDO represents a packaged model with metadata (Java ProjectModelPackDO).
type ProjectModelPackDO struct {
	BaseDO
	ProjectID string `gorm:"type:varchar(64);not null" json:"project_id"`
	ModelID   string `gorm:"uniqueIndex:upk_model_id;type:varchar(64);not null" json:"model_id"`
	Initiator string `gorm:"type:varchar(64);not null" json:"initiator"`
	ModelName string `gorm:"type:varchar(256);not null" json:"model_name"`
	ModelDesc string `gorm:"type:text" json:"model_desc"`
	// ModelStats is a ModelStatsEnum code (ModelStats* constants).
	ModelStats int8   `gorm:"not null;default:0" json:"model_stats"`
	ServingID  string `gorm:"type:varchar(64);not null;default:''" json:"serving_id"`
	// SampleTables is the JSON map {nodeId: datatableId}.
	SampleTables string `gorm:"type:text;not null" json:"sample_tables"`
	// ModelList is the JSON array of packed graph node ids.
	ModelList     string `gorm:"type:text;not null" json:"model_list"`
	TrainID       string `gorm:"type:varchar(64);not null" json:"train_id"`
	ModelReportID string `gorm:"type:varchar(64);not null" json:"model_report_id"`
	GraphDetail   string `gorm:"type:text" json:"graph_detail"`
	// ModelDatasource is the JSON array [{partyId, datasource}] (Java partyDataSources).
	ModelDatasource string `gorm:"type:text;not null" json:"model_datasource"`
}

func (ProjectModelPackDO) TableName() string { return "project_model_pack" }

// ProjectModelServingDO represents a model serving deployment (Java ProjectModelServingDO).
type ProjectModelServingDO struct {
	BaseDO
	ProjectID string `gorm:"type:varchar(64);not null" json:"project_id"`
	ServingID string `gorm:"uniqueIndex:upk_serving_id;type:varchar(64);not null" json:"serving_id"`
	Initiator string `gorm:"type:varchar(64);not null" json:"initiator"`
	// ServingInputConfig is the KusciaServingConfig JSON (proto field names).
	ServingInputConfig string `gorm:"type:text;not null" json:"serving_input_config"`
	// Parties is a JSON array of ServingParty JSON strings.
	Parties string `gorm:"type:text" json:"parties"`
	// PartyEndpoints is the JSON array [{nodeId, endpoints}].
	PartyEndpoints string `gorm:"type:text" json:"party_endpoints"`
	ServingStats   string `gorm:"type:varchar(16)" json:"serving_stats"` // init / success / failed
	ErrorMsg       string `gorm:"type:text" json:"error_msg"`
}

func (ProjectModelServingDO) TableName() string { return "project_model_serving" }

// ProjectModelExportDO tracks a model export (pack) Kuscia job until it
// finishes (replaces Java MODEL_EXPORT_CACHE). Payload holds the ModelExportDTO JSON.
type ProjectModelExportDO struct {
	BaseDO
	JobID     string `gorm:"uniqueIndex:upk_model_export_job_id;type:varchar(64);not null" json:"job_id"`
	ProjectID string `gorm:"type:varchar(64);not null" json:"project_id"`
	Status    string `gorm:"type:varchar(32);not null" json:"status"`
	ErrMsg    string `gorm:"type:text" json:"err_msg"`
	Payload   string `gorm:"type:text;not null" json:"payload"`
}

func (ProjectModelExportDO) TableName() string { return "project_model_export" }
