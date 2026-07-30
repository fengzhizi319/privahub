package repository

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// NodeRepo is the GORM implementation of NodeRepository.
type NodeRepo struct {
	*BaseRepo[model.NodeDO]
}

// NewNodeRepo creates a new NodeRepo.
func NewNodeRepo(db *gorm.DB) *NodeRepo {
	return &NodeRepo{BaseRepo: NewBaseRepo[model.NodeDO](db)}
}

// FindByNodeID retrieves a node by its node_id.
func (r *NodeRepo) FindByNodeID(ctx context.Context, nodeID string) (*model.NodeDO, error) {
	var node model.NodeDO
	err := r.DB().WithContext(ctx).Where("node_id = ?", nodeID).First(&node).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// FindByControlNodeID retrieves nodes by control_node_id.
func (r *NodeRepo) FindByControlNodeID(ctx context.Context, controlNodeID string) ([]model.NodeDO, error) {
	var nodes []model.NodeDO
	err := r.DB().WithContext(ctx).Where("control_node_id = ?", controlNodeID).Find(&nodes).Error
	return nodes, err
}

// FindByType retrieves nodes by type.
func (r *NodeRepo) FindByType(ctx context.Context, nodeType string) ([]model.NodeDO, error) {
	var nodes []model.NodeDO
	err := r.DB().WithContext(ctx).Where("type = ?", nodeType).Find(&nodes).Error
	return nodes, err
}

// NodeRouteRepo is the GORM implementation of NodeRouteRepository.
type NodeRouteRepo struct {
	*BaseRepo[model.NodeRouteDO]
}

// NewNodeRouteRepo creates a new NodeRouteRepo.
func NewNodeRouteRepo(db *gorm.DB) *NodeRouteRepo {
	return &NodeRouteRepo{BaseRepo: NewBaseRepo[model.NodeRouteDO](db)}
}

// FindBySrcNodeID retrieves routes by source node.
func (r *NodeRouteRepo) FindBySrcNodeID(ctx context.Context, srcNodeID string) ([]model.NodeRouteDO, error) {
	var routes []model.NodeRouteDO
	err := r.DB().WithContext(ctx).Where("src_node_id = ?", srcNodeID).Find(&routes).Error
	return routes, err
}

// FindByDstNodeID retrieves routes by destination node.
func (r *NodeRouteRepo) FindByDstNodeID(ctx context.Context, dstNodeID string) ([]model.NodeRouteDO, error) {
	var routes []model.NodeRouteDO
	err := r.DB().WithContext(ctx).Where("dst_node_id = ?", dstNodeID).Find(&routes).Error
	return routes, err
}

// FindByPair retrieves a route by source and destination node pair.
func (r *NodeRouteRepo) FindByPair(ctx context.Context, srcNodeID, dstNodeID string) (*model.NodeRouteDO, error) {
	var route model.NodeRouteDO
	err := r.DB().WithContext(ctx).Where("src_node_id = ? AND dst_node_id = ?", srcNodeID, dstNodeID).First(&route).Error
	if err != nil {
		return nil, err
	}
	return &route, nil
}

// ProjectRepo is the GORM implementation of ProjectRepository.
type ProjectRepo struct {
	*BaseRepo[model.ProjectDO]
}

// NewProjectRepo creates a new ProjectRepo.
func NewProjectRepo(db *gorm.DB) *ProjectRepo {
	return &ProjectRepo{BaseRepo: NewBaseRepo[model.ProjectDO](db)}
}

// FindByProjectID retrieves a project by project_id.
func (r *ProjectRepo) FindByProjectID(ctx context.Context, projectID string) (*model.ProjectDO, error) {
	var project model.ProjectDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).First(&project).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// FindByOwnerID retrieves projects by owner.
func (r *ProjectRepo) FindByOwnerID(ctx context.Context, ownerID string) ([]model.ProjectDO, error) {
	var projects []model.ProjectDO
	err := r.DB().WithContext(ctx).Where("owner_id = ?", ownerID).Find(&projects).Error
	return projects, err
}

// PageQuery retrieves projects with pagination and optional name filter.
func (r *ProjectRepo) PageQuery(ctx context.Context, page, size int, name string) ([]model.ProjectDO, int64, error) {
	var projects []model.ProjectDO
	var total int64

	query := r.DB().WithContext(ctx).Model(&model.ProjectDO{})
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	err := query.Offset(offset).Limit(size).Order("gmt_create DESC").Find(&projects).Error
	return projects, total, err
}

// InstRepo is the GORM implementation of InstRepository.
type InstRepo struct {
	*BaseRepo[model.InstDO]
}

// NewInstRepo creates a new InstRepo.
func NewInstRepo(db *gorm.DB) *InstRepo {
	return &InstRepo{BaseRepo: NewBaseRepo[model.InstDO](db)}
}

// FindByInstID retrieves an institution by inst_id.
func (r *InstRepo) FindByInstID(ctx context.Context, instID string) (*model.InstDO, error) {
	var inst model.InstDO
	err := r.DB().WithContext(ctx).Where("inst_id = ?", instID).First(&inst).Error
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

// ProjectInstRepo is the GORM implementation of ProjectInstRepository.
type ProjectInstRepo struct {
	*BaseRepo[model.ProjectInstDO]
}

// NewProjectInstRepo creates a new ProjectInstRepo.
func NewProjectInstRepo(db *gorm.DB) *ProjectInstRepo {
	return &ProjectInstRepo{BaseRepo: NewBaseRepo[model.ProjectInstDO](db)}
}

// FindByProjectID retrieves all inst associations for a project.
func (r *ProjectInstRepo) FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectInstDO, error) {
	var insts []model.ProjectInstDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).Find(&insts).Error
	return insts, err
}

// FindByInstID retrieves all project associations for an inst.
func (r *ProjectInstRepo) FindByInstID(ctx context.Context, instID string) ([]model.ProjectInstDO, error) {
	var insts []model.ProjectInstDO
	err := r.DB().WithContext(ctx).Where("inst_id = ?", instID).Find(&insts).Error
	return insts, err
}

// DeleteByProjectID deletes all inst associations for a project.
func (r *ProjectInstRepo) DeleteByProjectID(ctx context.Context, projectID string) error {
	return r.DB().WithContext(ctx).Where("project_id = ?", projectID).Delete(&model.ProjectInstDO{}).Error
}

// ProjectNodeRepo is the GORM implementation of ProjectNodeRepository.
type ProjectNodeRepo struct {
	*BaseRepo[model.ProjectNodeDO]
}

// NewProjectNodeRepo creates a new ProjectNodeRepo.
func NewProjectNodeRepo(db *gorm.DB) *ProjectNodeRepo {
	return &ProjectNodeRepo{BaseRepo: NewBaseRepo[model.ProjectNodeDO](db)}
}

// FindByProjectID retrieves all node associations for a project.
func (r *ProjectNodeRepo) FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectNodeDO, error) {
	var nodes []model.ProjectNodeDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).Find(&nodes).Error
	return nodes, err
}

// FindByNodeID retrieves all project associations for a node.
func (r *ProjectNodeRepo) FindByNodeID(ctx context.Context, nodeID string) ([]model.ProjectNodeDO, error) {
	var nodes []model.ProjectNodeDO
	err := r.DB().WithContext(ctx).Where("node_id = ?", nodeID).Find(&nodes).Error
	return nodes, err
}

// DeleteByProjectID deletes all node associations for a project.
func (r *ProjectNodeRepo) DeleteByProjectID(ctx context.Context, projectID string) error {
	return r.DB().WithContext(ctx).Where("project_id = ?", projectID).Delete(&model.ProjectNodeDO{}).Error
}
