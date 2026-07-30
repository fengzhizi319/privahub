// Package repository provides data access interfaces and GORM implementations.
package repository

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
)

// BaseRepository defines common CRUD operations for all entities.
type BaseRepository[T any] interface {
	Create(ctx context.Context, entity *T) error
	BatchCreate(ctx context.Context, entities []T) error
	Update(ctx context.Context, entity *T) error
	Delete(ctx context.Context, id int64) error
	FindByID(ctx context.Context, id int64) (*T, error)
	FindAll(ctx context.Context) ([]T, error)
}

// InstRepository provides access to institution data.
type InstRepository interface {
	BaseRepository[model.InstDO]
	FindByInstID(ctx context.Context, instID string) (*model.InstDO, error)
}

// NodeRepository provides access to node data.
type NodeRepository interface {
	BaseRepository[model.NodeDO]
	FindByNodeID(ctx context.Context, nodeID string) (*model.NodeDO, error)
	FindByControlNodeID(ctx context.Context, controlNodeID string) ([]model.NodeDO, error)
	FindByType(ctx context.Context, nodeType string) ([]model.NodeDO, error)
}

// NodeRouteRepository provides access to node route data.
type NodeRouteRepository interface {
	BaseRepository[model.NodeRouteDO]
	FindBySrcNodeID(ctx context.Context, srcNodeID string) ([]model.NodeRouteDO, error)
	FindByDstNodeID(ctx context.Context, dstNodeID string) ([]model.NodeRouteDO, error)
	FindByPair(ctx context.Context, srcNodeID, dstNodeID string) (*model.NodeRouteDO, error)
}

// ProjectRepository provides access to project data.
type ProjectRepository interface {
	BaseRepository[model.ProjectDO]
	FindByProjectID(ctx context.Context, projectID string) (*model.ProjectDO, error)
	FindByOwnerID(ctx context.Context, ownerID string) ([]model.ProjectDO, error)
	PageQuery(ctx context.Context, page, size int, name string) ([]model.ProjectDO, int64, error)
}

// ProjectInstRepository provides access to project-institution associations.
type ProjectInstRepository interface {
	BaseRepository[model.ProjectInstDO]
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectInstDO, error)
	FindByInstID(ctx context.Context, instID string) ([]model.ProjectInstDO, error)
	DeleteByProjectID(ctx context.Context, projectID string) error
}

// ProjectNodeRepository provides access to project-node associations.
type ProjectNodeRepository interface {
	BaseRepository[model.ProjectNodeDO]
	FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectNodeDO, error)
	FindByNodeID(ctx context.Context, nodeID string) ([]model.ProjectNodeDO, error)
	DeleteByProjectID(ctx context.Context, projectID string) error
}
