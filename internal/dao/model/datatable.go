package model

// ProjectDatatableDO associates a project with a datatable on a node.
type ProjectDatatableDO struct {
	BaseDO
	ProjectID    string `gorm:"uniqueIndex:upk_project_datatable_id;type:varchar(64);not null" json:"project_id"`
	NodeID       string `gorm:"uniqueIndex:upk_project_datatable_id;type:varchar(64);not null" json:"node_id"`
	DatatableID  string `gorm:"uniqueIndex:upk_project_datatable_id;type:varchar(64);not null" json:"datatable_id"`
	TableConfigs string `gorm:"type:text;not null" json:"table_configs"`
	Source       string `gorm:"type:varchar(16);not null" json:"source"` // IMPORTED / CREATED
}

func (ProjectDatatableDO) TableName() string { return "project_datatable" }

// ProjectFedTableDO represents a federated table composed of multiple node datatables.
type ProjectFedTableDO struct {
	BaseDO
	ProjectID  string `gorm:"uniqueIndex:upk_project_fed_table_id;type:varchar(64);not null" json:"project_id"`
	FedTableID string `gorm:"uniqueIndex:upk_project_fed_table_id;type:varchar(64);not null" json:"fed_table_id"`
	Joins      string `gorm:"type:text;not null" json:"joins"` // JSON: [{nodeId, datatableId}]
}

func (ProjectFedTableDO) TableName() string { return "project_fed_table" }

// TeeNodeDatatableManagementDO tracks TEE datatable authorization operations.
type TeeNodeDatatableManagementDO struct {
	BaseDO
	NodeID       string `gorm:"uniqueIndex:upk_node_datatable_management;type:varchar(64);not null" json:"node_id"`
	TeeNodeID    string `gorm:"uniqueIndex:upk_node_datatable_management;type:varchar(64);not null" json:"tee_node_id"`
	DatatableID  string `gorm:"uniqueIndex:upk_node_datatable_management;type:varchar(64);not null" json:"datatable_id"`
	DatasourceID string `gorm:"type:varchar(64);not null" json:"datasource_id"`
	Kind         string `gorm:"type:varchar(16);not null" json:"kind"`
	JobID        string `gorm:"uniqueIndex:upk_node_datatable_management;type:varchar(64);not null" json:"job_id"`
	Status       string `gorm:"type:varchar(32);not null" json:"status"`
	ErrMsg       string `gorm:"type:text" json:"err_msg"`
	OperateInfo  string `gorm:"type:text" json:"operate_info"`
}

func (TeeNodeDatatableManagementDO) TableName() string { return "tee_node_datatable_management" }

// FeatureTableDO represents a feature table for online serving.
type FeatureTableDO struct {
	BaseDO
	FeatureTableID   string `gorm:"uniqueIndex:upk_feature_table_id;type:varchar(8);not null" json:"feature_table_id"`
	FeatureTableName string `gorm:"type:varchar(32);not null" json:"feature_table_name"`
	NodeID           string `gorm:"type:varchar(64);not null" json:"node_id"`
	Type             string `gorm:"type:varchar(8);not null;default:'HTTP'" json:"type"`
	Description      string `gorm:"type:varchar(64)" json:"description"`
	URL              string `gorm:"type:varchar(64);not null" json:"url"`
	Columns          string `gorm:"type:text;not null" json:"columns"`
	Status           string `gorm:"type:varchar(16);not null" json:"status"` // Available / Unavailable
}

func (FeatureTableDO) TableName() string { return "feature_table" }

// ProjectFeatureTableDO associates a project with a feature table.
type ProjectFeatureTableDO struct {
	BaseDO
	ProjectID      string `gorm:"uniqueIndex:upk_project_feature_table_id;type:varchar(64);not null" json:"project_id"`
	NodeID         string `gorm:"uniqueIndex:upk_project_feature_table_id;type:varchar(64);not null" json:"node_id"`
	FeatureTableID string `gorm:"uniqueIndex:upk_project_feature_table_id;type:varchar(64);not null" json:"feature_table_id"`
	TableConfigs   string `gorm:"type:text;not null" json:"table_configs"`
	Source         string `gorm:"type:varchar(16);not null" json:"source"`
}

func (ProjectFeatureTableDO) TableName() string { return "project_feature_table" }
