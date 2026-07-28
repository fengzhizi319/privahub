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

// VoteRequestRepo is the GORM implementation of VoteRequestRepository.
type VoteRequestRepo struct {
	*BaseRepo[model.VoteRequestDO]
}

// NewVoteRequestRepo creates a new VoteRequestRepo.
func NewVoteRequestRepo(db *gorm.DB) *VoteRequestRepo {
	return &VoteRequestRepo{BaseRepo: NewBaseRepo[model.VoteRequestDO](db)}
}

// FindByVoteID retrieves a vote request by vote_id.
func (r *VoteRequestRepo) FindByVoteID(ctx context.Context, voteID string) (*model.VoteRequestDO, error) {
	var vote model.VoteRequestDO
	err := r.DB().WithContext(ctx).Where("vote_id = ?", voteID).First(&vote).Error
	if err != nil {
		return nil, err
	}
	return &vote, nil
}

// FindByInitiator retrieves all votes initiated by a node.
func (r *VoteRequestRepo) FindByInitiator(ctx context.Context, initiator string) ([]model.VoteRequestDO, error) {
	var votes []model.VoteRequestDO
	err := r.DB().WithContext(ctx).Where("initiator = ?", initiator).Order("gmt_create DESC").Find(&votes).Error
	return votes, err
}

// FindByVoter retrieves all votes where a node is a voter.
func (r *VoteRequestRepo) FindByVoter(ctx context.Context, voter string) ([]model.VoteRequestDO, error) {
	var votes []model.VoteRequestDO
	// Use parameterized LIKE pattern to prevent SQL injection
	likePattern := "%" + voter + "%"
	err := r.DB().WithContext(ctx).Where("voters LIKE ?", likePattern).Order("gmt_create DESC").Find(&votes).Error
	return votes, err
}

// VoteInviteRepo is the GORM implementation of VoteInviteRepository.
type VoteInviteRepo struct {
	*BaseRepo[model.VoteInviteDO]
}

// NewVoteInviteRepo creates a new VoteInviteRepo.
func NewVoteInviteRepo(db *gorm.DB) *VoteInviteRepo {
	return &VoteInviteRepo{BaseRepo: NewBaseRepo[model.VoteInviteDO](db)}
}

// FindByVoteID retrieves all invites for a vote.
func (r *VoteInviteRepo) FindByVoteID(ctx context.Context, voteID string) ([]model.VoteInviteDO, error) {
	var invites []model.VoteInviteDO
	err := r.DB().WithContext(ctx).Where("vote_id = ?", voteID).Find(&invites).Error
	return invites, err
}

// FindByParticipant retrieves all invites for a participant.
func (r *VoteInviteRepo) FindByParticipant(ctx context.Context, participantID string) ([]model.VoteInviteDO, error) {
	var invites []model.VoteInviteDO
	err := r.DB().WithContext(ctx).Where("vote_participant_id = ?", participantID).Find(&invites).Error
	return invites, err
}

// FindByVoteAndParticipant retrieves a specific invite.
func (r *VoteInviteRepo) FindByVoteAndParticipant(ctx context.Context, voteID, participantID string) (*model.VoteInviteDO, error) {
	var invite model.VoteInviteDO
	err := r.DB().WithContext(ctx).
		Where("vote_id = ? AND vote_participant_id = ?", voteID, participantID).
		First(&invite).Error
	if err != nil {
		return nil, err
	}
	return &invite, nil
}
