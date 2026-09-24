package service

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/gorm"
)

// DataResourceType is the kind of data resource checked by DataResourceAuth
// (Java DataResourceTypeEnum).
type DataResourceType string

const (
	DataResourceNodeID    DataResourceType = "NODE_ID"
	DataResourceProjectID DataResourceType = "PROJECT_ID"
	DataResourceInstID    DataResourceType = "INST_ID"
)

// projectStatusArchived is Java ProjectStatusEnum.ARCHIVED.
const projectStatusArchived = 2

// DataResourceAuth implements Java DataResourceAspect + DefaultDataResourceAuth:
// whether the current user may access a node / project / institution.
//
// Rules:
//   - EDGE platform: only NODE_ID is checked (Java skips other types on edge).
//   - CENTER owner users may access everything.
//   - NODE_ID: ownerId == nodeId, or the node belongs to the user's institution
//     (node.inst_id == ownerId).
//   - PROJECT_ID: AUTONOMY -> project_inst(project, ownerId) exists; otherwise
//     project_node(project, ownerId) exists. An ARCHIVED project is still
//     readable while it has project nodes (Java checkNodeArchive).
//   - INST_ID: ownerId == instId, or the owner node belongs to instId.
type DataResourceAuth struct {
	db  *gorm.DB
	env *EnvService
}

// NewDataResourceAuth creates the checker.
func NewDataResourceAuth(db *gorm.DB, env *EnvService) *DataResourceAuth {
	return &DataResourceAuth{db: db, env: env}
}

// Check reports whether user may access the resource. A nil user is denied.
func (a *DataResourceAuth) Check(ctx context.Context, user *auth.UserContext, kind DataResourceType, id string) bool {
	if user == nil {
		return false
	}
	if a.env != nil && a.env.IsEdge() && kind != DataResourceNodeID {
		return true
	}
	if user.IsCenterOwner() {
		return true
	}
	if id == "" {
		return false
	}
	switch kind {
	case DataResourceNodeID:
		return user.OwnerID == id || a.nodeInInst(ctx, id, user.OwnerID)
	case DataResourceProjectID:
		return a.checkProject(ctx, user, id)
	case DataResourceInstID:
		return user.OwnerID == id || a.nodeInInst(ctx, user.OwnerID, id)
	}
	return false
}

func (a *DataResourceAuth) nodeInInst(ctx context.Context, nodeID, instID string) bool {
	if a.db == nil || instID == "" {
		return false
	}
	if !a.db.Migrator().HasColumn(&model.NodeDO{}, "inst_id") {
		return false
	}
	var count int64
	if err := a.db.WithContext(ctx).Model(&model.NodeDO{}).
		Where("node_id = ? AND inst_id = ?", nodeID, instID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (a *DataResourceAuth) checkProject(ctx context.Context, user *auth.UserContext, projectID string) bool {
	if a.db == nil {
		return user.ContainProjectID(projectID)
	}
	var count int64
	if a.env != nil && a.env.IsAutonomy() {
		a.db.WithContext(ctx).Model(&model.ProjectInstDO{}).
			Where("project_id = ? AND inst_id = ?", projectID, user.OwnerID).Count(&count)
	} else {
		a.db.WithContext(ctx).Model(&model.ProjectNodeDO{}).
			Where("project_id = ? AND node_id = ?", projectID, user.OwnerID).Count(&count)
	}
	if count > 0 {
		return true
	}
	var project model.ProjectDO
	if err := a.db.WithContext(ctx).Where("project_id = ?", projectID).First(&project).Error; err != nil {
		return false
	}
	if int(project.Status) == projectStatusArchived {
		var nodes int64
		a.db.WithContext(ctx).Model(&model.ProjectNodeDO{}).Where("project_id = ?", projectID).Count(&nodes)
		return nodes > 0
	}
	return false
}
