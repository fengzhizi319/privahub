// Package model defines all database entity structs (DO) for SecretPad-Go.
// These structs map 1:1 to the Java SecretPad persistence entities.
package model

import (
	"time"

	"gorm.io/gorm"
)

// BaseDO provides common fields for all entities.
type BaseDO struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	IsDeleted   int8           `gorm:"default:0;not null" json:"-"`
	GmtCreate   time.Time      `gorm:"autoCreateTime" json:"gmt_create"`
	GmtModified time.Time      `gorm:"autoUpdateTime" json:"gmt_modified"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// InstDO represents an institution entity.
type InstDO struct {
	BaseDO
	InstID string `gorm:"uniqueIndex:upk_inst_id;type:varchar(64);not null" json:"inst_id"`
	Name   string `gorm:"type:varchar(256)" json:"name"`
}

func (InstDO) TableName() string { return "inst" }

// NodeDO represents a computing node entity.
type NodeDO struct {
	BaseDO
	NodeID        string `gorm:"uniqueIndex:upk_node_id;type:varchar(64);not null" json:"node_id"`
	Name          string `gorm:"type:varchar(256);not null" json:"name"`
	Auth          string `gorm:"type:text" json:"auth"`
	Description   string `gorm:"type:text;default:''" json:"description"`
	ControlNodeID string `gorm:"type:varchar(64);not null" json:"control_node_id"`
	NetAddress    string `gorm:"type:varchar(100)" json:"net_address"`
	Token         string `gorm:"type:varchar(100)" json:"token"`
	Type          string `gorm:"type:varchar(10);default:'normal'" json:"type"`
	Mode          int    `gorm:"default:0;not null" json:"mode"` // 0:mpc 1:tee 2:mpc&tee
	MasterNodeID  string `gorm:"type:varchar(64);default:'master'" json:"master_node_id"`
}

func (NodeDO) TableName() string { return "node" }

// NodeRouteDO represents a communication route between two nodes.
type NodeRouteDO struct {
	BaseDO
	RouteID       string `gorm:"type:varchar(64);not null" json:"route_id"`
	SrcNodeID     string `gorm:"uniqueIndex:upk_route_src_dst;type:varchar(64);not null" json:"src_node_id"`
	DstNodeID     string `gorm:"uniqueIndex:upk_route_src_dst;type:varchar(64);not null" json:"dst_node_id"`
	SrcNetAddress string `gorm:"type:varchar(100)" json:"src_net_address"`
	DstNetAddress string `gorm:"type:varchar(100)" json:"dst_net_address"`
}

func (NodeRouteDO) TableName() string { return "node_route" }

// ProjectDO represents a privacy computing project.
type ProjectDO struct {
	BaseDO
	ProjectID   string `gorm:"uniqueIndex:upk_project_id;type:varchar(64);not null" json:"project_id"`
	Name        string `gorm:"type:varchar(256);not null" json:"name"`
	ComputeMode string `gorm:"type:varchar(64);default:'mpc';not null" json:"compute_mode"`
	ComputeFunc string `gorm:"type:varchar(64);default:'ALL';not null" json:"compute_func"`
	ProjectInfo string `gorm:"type:text;default:'';not null" json:"project_info"`
	Description string `gorm:"type:text;default:''" json:"description"`
	OwnerID     string `gorm:"type:varchar(64);not null;default:''" json:"owner_id"`
	Status      int8   `gorm:"default:0;not null" json:"status"`
}

func (ProjectDO) TableName() string { return "project" }

// ProjectInstDO associates a project with an institution.
type ProjectInstDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_inst_id;type:varchar(64);not null" json:"project_id"`
	InstID    string `gorm:"uniqueIndex:upk_project_inst_id;type:varchar(64);not null" json:"inst_id"`
}

func (ProjectInstDO) TableName() string { return "project_inst" }

// ProjectNodeDO associates a project with a node.
type ProjectNodeDO struct {
	BaseDO
	ProjectID string `gorm:"uniqueIndex:upk_project_node_id;type:varchar(64);not null" json:"project_id"`
	NodeID    string `gorm:"uniqueIndex:upk_project_node_id;type:varchar(64);not null" json:"node_id"`
}

func (ProjectNodeDO) TableName() string { return "project_node" }
