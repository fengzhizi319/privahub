package repository

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// DatatableRepo is the GORM implementation of DatatableRepository.
type DatatableRepo struct {
	*BaseRepo[model.ProjectDatatableDO]
}

// NewDatatableRepo creates a new DatatableRepo.
func NewDatatableRepo(db *gorm.DB) *DatatableRepo {
	return &DatatableRepo{BaseRepo: NewBaseRepo[model.ProjectDatatableDO](db)}
}

// FindByProjectID retrieves all datatables for a project.
func (r *DatatableRepo) FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectDatatableDO, error) {
	var datatables []model.ProjectDatatableDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).Find(&datatables).Error
	return datatables, err
}

// FindByNodeID retrieves all datatables for a node.
func (r *DatatableRepo) FindByNodeID(ctx context.Context, nodeID string) ([]model.ProjectDatatableDO, error) {
	var datatables []model.ProjectDatatableDO
	err := r.DB().WithContext(ctx).Where("node_id = ?", nodeID).Find(&datatables).Error
	return datatables, err
}

// FindByProjectAndNodeID retrieves datatables for a project and node.
func (r *DatatableRepo) FindByProjectAndNodeID(ctx context.Context, projectID, nodeID string) ([]model.ProjectDatatableDO, error) {
	var datatables []model.ProjectDatatableDO
	err := r.DB().WithContext(ctx).Where("project_id = ? AND node_id = ?", projectID, nodeID).Find(&datatables).Error
	return datatables, err
}

// FindByProjectNodeDatatable retrieves a specific datatable.
func (r *DatatableRepo) FindByProjectNodeDatatable(ctx context.Context, projectID, nodeID, datatableID string) (*model.ProjectDatatableDO, error) {
	var dt model.ProjectDatatableDO
	err := r.DB().WithContext(ctx).
		Where("project_id = ? AND node_id = ? AND datatable_id = ?", projectID, nodeID, datatableID).
		First(&dt).Error
	if err != nil {
		return nil, err
	}
	return &dt, nil
}

// FindAll retrieves all datatables.
func (r *DatatableRepo) FindAll(ctx context.Context) ([]model.ProjectDatatableDO, error) {
	var datatables []model.ProjectDatatableDO
	err := r.DB().WithContext(ctx).Find(&datatables).Error
	return datatables, err
}

// DeleteByProjectID deletes all datatable associations for a project.
func (r *DatatableRepo) DeleteByProjectID(ctx context.Context, projectID string) error {
	return r.DB().WithContext(ctx).Where("project_id = ?", projectID).Delete(&model.ProjectDatatableDO{}).Error
}

// FedTableRepo is the GORM implementation of FedTableRepository.
type FedTableRepo struct {
	*BaseRepo[model.ProjectFedTableDO]
}

// NewFedTableRepo creates a new FedTableRepo.
func NewFedTableRepo(db *gorm.DB) *FedTableRepo {
	return &FedTableRepo{BaseRepo: NewBaseRepo[model.ProjectFedTableDO](db)}
}

// FindByProjectID retrieves all federated tables for a project.
func (r *FedTableRepo) FindByProjectID(ctx context.Context, projectID string) ([]model.ProjectFedTableDO, error) {
	var tables []model.ProjectFedTableDO
	err := r.DB().WithContext(ctx).Where("project_id = ?", projectID).Find(&tables).Error
	return tables, err
}

// FindByFedTableID retrieves a federated table by ID.
func (r *FedTableRepo) FindByFedTableID(ctx context.Context, fedTableID string) (*model.ProjectFedTableDO, error) {
	var table model.ProjectFedTableDO
	err := r.DB().WithContext(ctx).Where("fed_table_id = ?", fedTableID).First(&table).Error
	if err != nil {
		return nil, err
	}
	return &table, nil
}
