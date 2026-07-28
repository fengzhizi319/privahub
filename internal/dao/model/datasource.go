package model

// DatasourceDO represents a datasource entity.
type DatasourceDO struct {
	BaseDO
	DatasourceID   string `gorm:"uniqueIndex:upk_datasource_id;type:varchar(64);not null" json:"datasource_id"`
	Name           string `gorm:"type:varchar(256);not null" json:"name"`
	Type           string `gorm:"type:varchar(32);not null;default:'OSS'" json:"type"` // OSS, HTTP, LOCAL_FS
	Status         string `gorm:"type:varchar(32);not null;default:'Available'" json:"status"`
	OwnerID        string `gorm:"type:varchar(64);not null" json:"owner_id"`
	ConnectionInfo string `gorm:"type:text" json:"connection_info"` // JSON config
	Description    string `gorm:"type:varchar(512)" json:"description"`
}

func (DatasourceDO) TableName() string { return "datasource" }

// DatasourceNodeDO associates a datasource with a node.
type DatasourceNodeDO struct {
	BaseDO
	DatasourceID string `gorm:"uniqueIndex:upk_datasource_node;type:varchar(64);not null" json:"datasource_id"`
	NodeID       string `gorm:"uniqueIndex:upk_datasource_node;type:varchar(64);not null" json:"node_id"`
}

func (DatasourceNodeDO) TableName() string { return "datasource_node" }

// ProjectGraphDomainDatasourceDO binds a datasource to a project graph domain.
type ProjectGraphDomainDatasourceDO struct {
	BaseDO
	ProjectID    string `gorm:"uniqueIndex:upk_project_graph_domain_datasource;type:varchar(64);not null" json:"project_id"`
	GraphID      string `gorm:"uniqueIndex:upk_project_graph_domain_datasource;type:varchar(64);not null" json:"graph_id"`
	DomainID     string `gorm:"uniqueIndex:upk_project_graph_domain_datasource;type:varchar(64);not null" json:"domain_id"`
	DatasourceID string `gorm:"type:varchar(64);not null" json:"datasource_id"`
}

func (ProjectGraphDomainDatasourceDO) TableName() string { return "project_graph_domain_datasource" }
