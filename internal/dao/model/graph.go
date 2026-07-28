package model

// ProjectGraphDO represents a DAG graph entity.
type ProjectGraphDO struct {
	BaseDO
	ProjectID      string `gorm:"uniqueIndex:upk_project_graph;type:varchar(64);not null" json:"project_id"`
	GraphID        string `gorm:"uniqueIndex:upk_project_graph;type:varchar(64);not null" json:"graph_id"`
	Name           string `gorm:"type:varchar(128)" json:"name"`
	Edges          string `gorm:"type:text" json:"edges"`
	OwnerID        string `gorm:"type:varchar(64);not null;default:''" json:"owner_id"`
	NodeMaxIndex   int    `gorm:"not null" json:"node_max_index"`
	MaxParallelism int    `gorm:"default:1" json:"max_parallelism"`
}

func (ProjectGraphDO) TableName() string { return "project_graph" }

// ProjectGraphNodeDO represents a node in a DAG graph.
type ProjectGraphNodeDO struct {
	BaseDO
	ProjectID   string `gorm:"uniqueIndex:upk_project_graph_node;type:varchar(64);not null" json:"project_id"`
	GraphID     string `gorm:"uniqueIndex:upk_project_graph_node;type:varchar(64);not null" json:"graph_id"`
	GraphNodeID string `gorm:"uniqueIndex:upk_project_graph_node;type:varchar(64);not null" json:"graph_node_id"`
	CodeName    string `gorm:"type:varchar(64)" json:"code_name"`
	Label       string `gorm:"type:varchar(64)" json:"label"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Inputs      string `gorm:"type:text" json:"inputs"`
	Outputs     string `gorm:"type:text" json:"outputs"`
	NodeDef     string `gorm:"type:text" json:"node_def"`
}

func (ProjectGraphNodeDO) TableName() string { return "project_graph_node" }

// ProjectGraphNodeKusciaParamsDO stores Kuscia runtime params for a graph node.
type ProjectGraphNodeKusciaParamsDO struct {
	BaseDO
	ProjectID     string `gorm:"uniqueIndex:upk_project_graph_node_kuscia_params_id;type:varchar(64)" json:"project_id"`
	GraphID       string `gorm:"uniqueIndex:upk_project_graph_node_kuscia_params_id;type:varchar(64)" json:"graph_id"`
	GraphNodeID   string `gorm:"uniqueIndex:upk_project_graph_node_kuscia_params_id;type:varchar(64)" json:"graph_node_id"`
	JobID         string `gorm:"type:varchar(64);not null" json:"job_id"`
	TaskID        string `gorm:"type:varchar(64);not null" json:"task_id"`
	Inputs        string `gorm:"type:text" json:"inputs"`
	Outputs       string `gorm:"type:text" json:"outputs"`
	NodeEvalParam string `gorm:"type:text" json:"node_eval_param"`
}

func (ProjectGraphNodeKusciaParamsDO) TableName() string { return "project_graph_node_kuscia_params" }
