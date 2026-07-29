package service

import (
	"context"
	"errors"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Datasource service errors.
var (
	ErrDatasourceNotFound = errors.New("datasource not found")
)

// DatasourceService handles datasource management.
type DatasourceService struct {
	db           *gorm.DB
	kusciaClient *kuscia.Client
}

// NewDatasourceService creates a new DatasourceService.
func NewDatasourceService(db *gorm.DB, kusciaClient *kuscia.Client) *DatasourceService {
	return &DatasourceService{db: db, kusciaClient: kusciaClient}
}

// --- Request / Response DTOs ---

// CreateDatasourceRequest represents a datasource creation request.
type CreateDatasourceRequest struct {
	Name           string   `json:"name" binding:"required"`
	Type           string   `json:"type"`
	OwnerID        string   `json:"owner_id" binding:"required"`
	ConnectionInfo string   `json:"connection_info"`
	Description    string   `json:"description"`
	NodeIDs        []string `json:"node_ids"`
}

// CreateDatasourceVO is the response for datasource creation.
type CreateDatasourceVO struct {
	DatasourceID string `json:"datasource_id"`
}

// DatasourceListRequest represents a datasource list request.
type DatasourceListRequest struct {
	OwnerID    string `json:"owner_id"`
	OwnerIDAlt string `json:"ownerId"`
	Name       string `json:"name"`
}

// DatasourceVO represents a datasource view object.
type DatasourceVO struct {
	DatasourceID string `json:"datasource_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	OwnerID      string `json:"owner_id"`
	Description  string `json:"description"`
	GmtCreate    string `json:"gmt_create"`
}

// DatasourceListVO represents a datasource list response.
type DatasourceListVO struct {
	Datasources []DatasourceVO `json:"datasources"`
	Total       int64          `json:"total"`
}

// DatasourceDetailRequest represents a datasource detail request.
type DatasourceDetailRequest struct {
	DatasourceID    string `json:"datasource_id"`
	DatasourceIDAlt string `json:"datasourceId"`
	OwnerID         string `json:"owner_id"`
	OwnerIDAlt      string `json:"ownerId"`
}

// DatasourceDetailVO represents a datasource detail view object.
type DatasourceDetailVO struct {
	DatasourceID   string   `json:"datasource_id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Status         string   `json:"status"`
	OwnerID        string   `json:"owner_id"`
	ConnectionInfo string   `json:"connection_info"`
	Description    string   `json:"description"`
	NodeIDs        []string `json:"node_ids"`
	GmtCreate      string   `json:"gmt_create"`
}

// DeleteDatasourceRequest represents a datasource deletion request.
type DeleteDatasourceRequest struct {
	DatasourceID    string `json:"datasource_id"`
	DatasourceIDAlt string `json:"datasourceId"`
	OwnerID         string `json:"owner_id"`
	OwnerIDAlt      string `json:"ownerId"`
}

// TestDatasourceRequest represents a datasource test request.
type TestDatasourceRequest struct {
	DatasourceID    string `json:"datasource_id"`
	DatasourceIDAlt string `json:"datasourceId"`
	ConnectionInfo  string `json:"connection_info"`
	Type            string `json:"type"`
}

// TestDatasourceVO represents a datasource test result.
type TestDatasourceVO struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// DatasourceNodesRequest represents a datasource-nodes query request (frontend contract).
type DatasourceNodesRequest struct {
	OwnerID      string `json:"ownerId"`
	DatasourceID string `json:"datasourceId"`
}

// DataSourceRelatedNode represents a node associated with a datasource.
type DataSourceRelatedNode struct {
	NodeID   string `json:"nodeId"`
	NodeName string `json:"nodeName"`
	Status   string `json:"status"`
}

// DatasourceNodesVO represents the datasource-nodes response.
type DatasourceNodesVO struct {
	Nodes []DataSourceRelatedNode `json:"nodes"`
}

// --- Service Methods ---

// CreateDatasource creates a new datasource.
func (s *DatasourceService) CreateDatasource(ctx context.Context, req *CreateDatasourceRequest) (*CreateDatasourceVO, error) {
	datasourceID := uuid.New().String()[:8]

	dsType := req.Type
	if dsType == "" {
		dsType = "OSS"
	}

	ds := &model.DatasourceDO{
		DatasourceID:   datasourceID,
		Name:           req.Name,
		Type:           dsType,
		Status:         "Available",
		OwnerID:        req.OwnerID,
		ConnectionInfo: req.ConnectionInfo,
		Description:    req.Description,
	}

	if err := s.db.WithContext(ctx).Create(ds).Error; err != nil {
		return nil, err
	}

	// Associate nodes
	for _, nodeID := range req.NodeIDs {
		_ = s.db.WithContext(ctx).Create(&model.DatasourceNodeDO{
			DatasourceID: datasourceID,
			NodeID:       nodeID,
		}).Error
	}

	return &CreateDatasourceVO{DatasourceID: datasourceID}, nil
}

// ListDatasources lists datasources.
func (s *DatasourceService) ListDatasources(ctx context.Context, req *DatasourceListRequest) (*DatasourceListVO, error) {
	if req.OwnerID == "" {
		req.OwnerID = req.OwnerIDAlt
	}
	var datasources []model.DatasourceDO
	query := s.db.WithContext(ctx).Model(&model.DatasourceDO{})

	if req.OwnerID != "" {
		query = query.Where("owner_id = ?", req.OwnerID)
	}
	if req.Name != "" {
		// Use parameterized LIKE pattern to prevent SQL injection
		likePattern := "%" + req.Name + "%"
		query = query.Where("name LIKE ?", likePattern)
	}

	if err := query.Find(&datasources).Error; err != nil {
		return nil, err
	}

	result := make([]DatasourceVO, 0, len(datasources))
	for _, ds := range datasources {
		result = append(result, DatasourceVO{
			DatasourceID: ds.DatasourceID,
			Name:         ds.Name,
			Type:         ds.Type,
			Status:       ds.Status,
			OwnerID:      ds.OwnerID,
			Description:  ds.Description,
			GmtCreate:    ds.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}

	return &DatasourceListVO{
		Datasources: result,
		Total:       int64(len(result)),
	}, nil
}

// GetDatasourceDetail retrieves datasource detail.
func (s *DatasourceService) GetDatasourceDetail(ctx context.Context, req *DatasourceDetailRequest) (*DatasourceDetailVO, error) {
	if req.DatasourceID == "" {
		req.DatasourceID = req.DatasourceIDAlt
	}
	if req.OwnerID == "" {
		req.OwnerID = req.OwnerIDAlt
	}
	var ds model.DatasourceDO
	if err := s.db.WithContext(ctx).Where("datasource_id = ?", req.DatasourceID).First(&ds).Error; err != nil {
		return nil, ErrDatasourceNotFound
	}

	// Get associated nodes
	var nodes []model.DatasourceNodeDO
	_ = s.db.WithContext(ctx).Where("datasource_id = ?", req.DatasourceID).Find(&nodes).Error
	nodeIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.NodeID)
	}

	return &DatasourceDetailVO{
		DatasourceID:   ds.DatasourceID,
		Name:           ds.Name,
		Type:           ds.Type,
		Status:         ds.Status,
		OwnerID:        ds.OwnerID,
		ConnectionInfo: ds.ConnectionInfo,
		Description:    ds.Description,
		NodeIDs:        nodeIDs,
		GmtCreate:      ds.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// DeleteDatasource deletes a datasource.
func (s *DatasourceService) DeleteDatasource(ctx context.Context, req *DeleteDatasourceRequest) error {
	var ds model.DatasourceDO
	if err := s.db.WithContext(ctx).Where("datasource_id = ?", req.DatasourceID).First(&ds).Error; err != nil {
		return ErrDatasourceNotFound
	}

	// Delete node associations
	_ = s.db.WithContext(ctx).Where("datasource_id = ?", req.DatasourceID).Delete(&model.DatasourceNodeDO{}).Error

	return s.db.WithContext(ctx).Delete(&ds).Error
}

// TestDatasource tests a datasource connection via Kuscia Ping.
func (s *DatasourceService) TestDatasource(ctx context.Context, req *TestDatasourceRequest) (*TestDatasourceVO, error) {
	// Use Kuscia Ping to verify connectivity
	if s.kusciaClient != nil {
		if err := s.kusciaClient.Ping(ctx); err != nil {
			return &TestDatasourceVO{
				Success: false,
				Message: "connection failed: " + err.Error(),
			}, nil
		}
	}

	return &TestDatasourceVO{
		Success: true,
		Message: "connection test passed",
	}, nil
}

// GetDatasourceNodes lists nodes associated with a datasource.
func (s *DatasourceService) GetDatasourceNodes(ctx context.Context, req *DatasourceNodesRequest) (*DatasourceNodesVO, error) {
	var rels []model.DatasourceNodeDO
	if err := s.db.WithContext(ctx).Where("datasource_id = ?", req.DatasourceID).Find(&rels).Error; err != nil {
		return nil, err
	}

	nodes := make([]DataSourceRelatedNode, 0, len(rels))
	for _, rel := range rels {
		item := DataSourceRelatedNode{
			NodeID: rel.NodeID,
			Status: "Available",
		}
		var node model.NodeDO
		if err := s.db.WithContext(ctx).Where("node_id = ?", rel.NodeID).First(&node).Error; err == nil {
			item.NodeName = node.Name
		}
		nodes = append(nodes, item)
	}

	return &DatasourceNodesVO{Nodes: nodes}, nil
}
