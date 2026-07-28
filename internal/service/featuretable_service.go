package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// FeatureTableService handles feature datasource management.
type FeatureTableService struct {
	db *gorm.DB
}

// NewFeatureTableService creates a new FeatureTableService.
func NewFeatureTableService(db *gorm.DB) *FeatureTableService {
	return &FeatureTableService{db: db}
}

// --- DTOs ---

// CreateFeatureDatasourceRequest represents a feature datasource creation request.
type CreateFeatureDatasourceRequest struct {
	NodeID           string `json:"node_id" binding:"required"`
	FeatureTableName string `json:"feature_table_name" binding:"required"`
	Type             string `json:"type"`
	Description      string `json:"description"`
	URL              string `json:"url" binding:"required"`
	Columns          string `json:"columns" binding:"required"`
	ProjectID        string `json:"project_id"`
}

// FeatureDataSourceVO represents a feature datasource view object.
type FeatureDataSourceVO struct {
	FeatureTableID   string `json:"feature_table_id"`
	FeatureTableName string `json:"feature_table_name"`
	NodeID           string `json:"node_id"`
	Type             string `json:"type"`
	Description      string `json:"description"`
	URL              string `json:"url"`
	Columns          string `json:"columns"`
	Status           string `json:"status"`
	GmtCreate        string `json:"gmt_create"`
}

// --- Service Methods ---

// CreateFeatureTable creates a new feature datasource.
func (s *FeatureTableService) CreateFeatureTable(ctx context.Context, req *CreateFeatureDatasourceRequest) error {
	featureTableID := uuid.New().String()[:8]
	ftType := req.Type
	if ftType == "" {
		ftType = "HTTP"
	}

	ft := &model.FeatureTableDO{
		FeatureTableID:   featureTableID,
		FeatureTableName: req.FeatureTableName,
		NodeID:           req.NodeID,
		Type:             ftType,
		Description:      req.Description,
		URL:              req.URL,
		Columns:          req.Columns,
		Status:           "Available",
	}

	if err := s.db.WithContext(ctx).Create(ft).Error; err != nil {
		return err
	}

	// If project_id is provided, also create project-feature association
	if req.ProjectID != "" {
		pft := &model.ProjectFeatureTableDO{
			ProjectID:      req.ProjectID,
			NodeID:         req.NodeID,
			FeatureTableID: featureTableID,
			TableConfigs:   req.Columns,
			Source:         "manual",
		}
		_ = s.db.WithContext(ctx).Create(pft).Error
	}

	return nil
}

// FeatureDatasourceList lists feature datasources for a node.
func (s *FeatureTableService) FeatureDatasourceList(ctx context.Context, nodeID string) ([]FeatureDataSourceVO, error) {
	var tables []model.FeatureTableDO
	if err := s.db.WithContext(ctx).
		Where("node_id = ?", nodeID).
		Order("gmt_create DESC").
		Find(&tables).Error; err != nil {
		return nil, err
	}

	return s.toFeatureVOList(tables), nil
}

// ProjectFeatureTableList lists feature datasources for a project+node.
func (s *FeatureTableService) ProjectFeatureTableList(ctx context.Context, nodeID, projectID string) ([]FeatureDataSourceVO, error) {
	// Find feature table IDs associated with this project
	var projectFTs []model.ProjectFeatureTableDO
	if err := s.db.WithContext(ctx).
		Where("node_id = ? AND project_id = ?", nodeID, projectID).
		Find(&projectFTs).Error; err != nil {
		return nil, err
	}

	if len(projectFTs) == 0 {
		return []FeatureDataSourceVO{}, nil
	}

	ftIDs := make([]string, 0, len(projectFTs))
	for _, pft := range projectFTs {
		ftIDs = append(ftIDs, pft.FeatureTableID)
	}

	var tables []model.FeatureTableDO
	if err := s.db.WithContext(ctx).
		Where("feature_table_id IN ?", ftIDs).
		Order("gmt_create DESC").
		Find(&tables).Error; err != nil {
		return nil, err
	}

	return s.toFeatureVOList(tables), nil
}

func (s *FeatureTableService) toFeatureVOList(tables []model.FeatureTableDO) []FeatureDataSourceVO {
	result := make([]FeatureDataSourceVO, 0, len(tables))
	for _, ft := range tables {
		result = append(result, FeatureDataSourceVO{
			FeatureTableID:   ft.FeatureTableID,
			FeatureTableName: ft.FeatureTableName,
			NodeID:           ft.NodeID,
			Type:             ft.Type,
			Description:      ft.Description,
			URL:              ft.URL,
			Columns:          ft.Columns,
			Status:           ft.Status,
			GmtCreate:        ft.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result
}
