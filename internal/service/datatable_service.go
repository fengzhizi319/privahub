package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
)

// Datatable service errors.
var (
	ErrDatatableNotFound = errors.New("datatable not found")
)

// DatatableService handles datatable management.
type DatatableService struct {
	datatableRepo repository.DatatableRepository
	fedTableRepo  repository.FedTableRepository
}

// NewDatatableService creates a new DatatableService.
func NewDatatableService(
	datatableRepo repository.DatatableRepository,
	fedTableRepo repository.FedTableRepository,
) *DatatableService {
	return &DatatableService{
		datatableRepo: datatableRepo,
		fedTableRepo:  fedTableRepo,
	}
}

// --- Request / Response DTOs ---

// RegisterDatatableRequest represents a datatable registration request.
type RegisterDatatableRequest struct {
	ProjectID    string `json:"project_id" binding:"required"`
	NodeID       string `json:"node_id" binding:"required"`
	DatatableID  string `json:"datatable_id"`
	TableConfigs string `json:"table_configs"`
	Source       string `json:"source"`
}

// DatatableVO represents a datatable view object.
type DatatableVO struct {
	ProjectID    string `json:"project_id"`
	NodeID       string `json:"node_id"`
	DatatableID  string `json:"datatable_id"`
	TableConfigs string `json:"table_configs"`
	Source       string `json:"source"`
	GmtCreate    string `json:"gmt_create"`
}

// ListDatatableRequest represents a datatable list request.
type ListDatatableRequest struct {
	ProjectID  string `json:"project_id"`
	NodeID     string `json:"node_id"`
	OwnerID    string `json:"owner_id"`
	NodeIDAlt  string `json:"nodeId"`
	OwnerIDAlt string `json:"ownerId"`
	PageNumber int    `json:"pageNumber"`
	PageSize   int    `json:"pageSize"`
}

// GetDatatableRequest represents a datatable detail request.
type GetDatatableRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	NodeID      string `json:"node_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
}

// DeleteDatatableRequest represents a datatable deletion request.
type DeleteDatatableRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	NodeID      string `json:"node_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
}

// GrantDatatableRequest represents a datatable grant request.
type GrantDatatableRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	NodeID      string `json:"node_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
	TargetNode  string `json:"target_node" binding:"required"`
}

// CreateFedTableRequest represents a federated table creation request.
type CreateFedTableRequest struct {
	ProjectID string    `json:"project_id" binding:"required"`
	Joins     []FedJoin `json:"joins" binding:"required"`
}

// FedJoin represents a single join entry in a federated table.
type FedJoin struct {
	NodeID      string `json:"node_id"`
	DatatableID string `json:"datatable_id"`
}

// FedTableVO represents a federated table view object.
type FedTableVO struct {
	FedTableID string `json:"fed_table_id"`
	ProjectID  string `json:"project_id"`
	Joins      string `json:"joins"`
	GmtCreate  string `json:"gmt_create"`
}

// --- Service Methods ---

// RegisterDatatable registers a datatable to a project.
func (s *DatatableService) RegisterDatatable(ctx context.Context, req *RegisterDatatableRequest) (*DatatableVO, error) {
	datatableID := req.DatatableID
	if datatableID == "" {
		datatableID = uuid.New().String()[:8]
	}

	source := req.Source
	if source == "" {
		source = "IMPORTED"
	}

	dt := &model.ProjectDatatableDO{
		ProjectID:    req.ProjectID,
		NodeID:       req.NodeID,
		DatatableID:  datatableID,
		TableConfigs: req.TableConfigs,
		Source:       source,
	}

	if err := s.datatableRepo.Create(ctx, dt); err != nil {
		return nil, err
	}

	return &DatatableVO{
		ProjectID:    req.ProjectID,
		NodeID:       req.NodeID,
		DatatableID:  datatableID,
		TableConfigs: req.TableConfigs,
		Source:       source,
		GmtCreate:    dt.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// ListDatatables lists datatables for a project.
func (s *DatatableService) ListDatatables(ctx context.Context, req *ListDatatableRequest) ([]DatatableVO, error) {
	var datatables []model.ProjectDatatableDO
	var err error

	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = req.NodeIDAlt
	}
	if nodeID == "" {
		nodeID = req.OwnerID
	}
	if nodeID == "" {
		nodeID = req.OwnerIDAlt
	}

	if req.ProjectID != "" && nodeID != "" {
		datatables, err = s.datatableRepo.FindByProjectAndNodeID(ctx, req.ProjectID, nodeID)
	} else if req.ProjectID != "" {
		datatables, err = s.datatableRepo.FindByProjectID(ctx, req.ProjectID)
	} else if nodeID != "" {
		datatables, err = s.datatableRepo.FindByNodeID(ctx, nodeID)
	} else {
		datatables, err = s.datatableRepo.FindAll(ctx)
	}
	if err != nil {
		return nil, err
	}

	result := make([]DatatableVO, 0, len(datatables))
	for _, dt := range datatables {
		result = append(result, DatatableVO{
			ProjectID:    dt.ProjectID,
			NodeID:       dt.NodeID,
			DatatableID:  dt.DatatableID,
			TableConfigs: dt.TableConfigs,
			Source:       dt.Source,
			GmtCreate:    dt.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// GetDatatable retrieves a datatable detail.
func (s *DatatableService) GetDatatable(ctx context.Context, req *GetDatatableRequest) (*DatatableVO, error) {
	dt, err := s.datatableRepo.FindByProjectNodeDatatable(ctx, req.ProjectID, req.NodeID, req.DatatableID)
	if err != nil {
		return nil, ErrDatatableNotFound
	}

	return &DatatableVO{
		ProjectID:    dt.ProjectID,
		NodeID:       dt.NodeID,
		DatatableID:  dt.DatatableID,
		TableConfigs: dt.TableConfigs,
		Source:       dt.Source,
		GmtCreate:    dt.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}

// DeleteDatatable deletes a datatable from a project.
func (s *DatatableService) DeleteDatatable(ctx context.Context, req *DeleteDatatableRequest) error {
	dt, err := s.datatableRepo.FindByProjectNodeDatatable(ctx, req.ProjectID, req.NodeID, req.DatatableID)
	if err != nil {
		return ErrDatatableNotFound
	}

	return s.datatableRepo.Delete(ctx, dt.ID)
}

// CreateFedTable creates a federated table.
func (s *DatatableService) CreateFedTable(ctx context.Context, req *CreateFedTableRequest) (*FedTableVO, error) {
	fedTableID := uuid.New().String()[:8]

	joinsJSON := "["
	for i, j := range req.Joins {
		if i > 0 {
			joinsJSON += ","
		}
		joinsJSON += `{"node_id":"` + j.NodeID + `","datatable_id":"` + j.DatatableID + `"}`
	}
	joinsJSON += "]"

	fedTable := &model.ProjectFedTableDO{
		ProjectID:  req.ProjectID,
		FedTableID: fedTableID,
		Joins:      joinsJSON,
	}

	if err := s.fedTableRepo.Create(ctx, fedTable); err != nil {
		return nil, err
	}

	return &FedTableVO{
		FedTableID: fedTableID,
		ProjectID:  req.ProjectID,
		Joins:      joinsJSON,
		GmtCreate:  fedTable.GmtCreate.Format("2006-01-02 15:04:05"),
	}, nil
}
