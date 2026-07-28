package service

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// GraphDatasource service errors.
var (
	ErrGraphDatasourceNotFound = errors.New("graph domain datasource not found")
)

// GraphDatasourceService handles project-graph-domain-datasource bindings.
type GraphDatasourceService struct {
	db *gorm.DB
}

// NewGraphDatasourceService creates a new GraphDatasourceService.
func NewGraphDatasourceService(db *gorm.DB) *GraphDatasourceService {
	return &GraphDatasourceService{db: db}
}

// --- DTOs ---

// GraphDatasourceBindRequest represents a bind request.
type GraphDatasourceBindRequest struct {
	ProjectID    string `json:"project_id" binding:"required"`
	GraphID      string `json:"graph_id" binding:"required"`
	DomainID     string `json:"domain_id" binding:"required"`
	DatasourceID string `json:"datasource_id" binding:"required"`
}

// GraphDatasourceVO represents a graph domain datasource view object.
type GraphDatasourceVO struct {
	ProjectID    string `json:"project_id"`
	GraphID      string `json:"graph_id"`
	DomainID     string `json:"domain_id"`
	DatasourceID string `json:"datasource_id"`
}

// --- Service Methods ---

// Bind creates or updates a graph-domain-datasource binding.
func (s *GraphDatasourceService) Bind(ctx context.Context, req *GraphDatasourceBindRequest) error {
	var existing model.ProjectGraphDomainDatasourceDO
	err := s.db.WithContext(ctx).
		Where("project_id = ? AND graph_id = ? AND domain_id = ?", req.ProjectID, req.GraphID, req.DomainID).
		First(&existing).Error

	if err == nil {
		// Update existing binding
		return s.db.WithContext(ctx).Model(&existing).
			Update("datasource_id", req.DatasourceID).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Create new binding
	binding := &model.ProjectGraphDomainDatasourceDO{
		ProjectID:    req.ProjectID,
		GraphID:      req.GraphID,
		DomainID:     req.DomainID,
		DatasourceID: req.DatasourceID,
	}
	return s.db.WithContext(ctx).Create(binding).Error
}

// Get retrieves a specific binding.
func (s *GraphDatasourceService) Get(ctx context.Context, projectID, graphID, domainID string) (*GraphDatasourceVO, error) {
	var binding model.ProjectGraphDomainDatasourceDO
	if err := s.db.WithContext(ctx).
		Where("project_id = ? AND graph_id = ? AND domain_id = ?", projectID, graphID, domainID).
		First(&binding).Error; err != nil {
		return nil, ErrGraphDatasourceNotFound
	}

	return &GraphDatasourceVO{
		ProjectID:    binding.ProjectID,
		GraphID:      binding.GraphID,
		DomainID:     binding.DomainID,
		DatasourceID: binding.DatasourceID,
	}, nil
}

// Unbind removes a binding.
func (s *GraphDatasourceService) Unbind(ctx context.Context, projectID, graphID, domainID string) error {
	result := s.db.WithContext(ctx).
		Where("project_id = ? AND graph_id = ? AND domain_id = ?", projectID, graphID, domainID).
		Delete(&model.ProjectGraphDomainDatasourceDO{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrGraphDatasourceNotFound
	}
	return nil
}

// ListByProject lists all bindings for a project.
func (s *GraphDatasourceService) ListByProject(ctx context.Context, projectID string) ([]GraphDatasourceVO, error) {
	var bindings []model.ProjectGraphDomainDatasourceDO
	if err := s.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Find(&bindings).Error; err != nil {
		return nil, err
	}

	result := make([]GraphDatasourceVO, 0, len(bindings))
	for _, b := range bindings {
		result = append(result, GraphDatasourceVO{
			ProjectID:    b.ProjectID,
			GraphID:      b.GraphID,
			DomainID:     b.DomainID,
			DatasourceID: b.DatasourceID,
		})
	}
	return result, nil
}

// ListByDomain lists datasource IDs bound to a domain.
func (s *GraphDatasourceService) ListByDomain(ctx context.Context, domainID string) ([]string, error) {
	var bindings []model.ProjectGraphDomainDatasourceDO
	if err := s.db.WithContext(ctx).
		Where("domain_id = ?", domainID).
		Find(&bindings).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, b := range bindings {
		if !seen[b.DatasourceID] {
			seen[b.DatasourceID] = true
			result = append(result, b.DatasourceID)
		}
	}
	return result, nil
}
