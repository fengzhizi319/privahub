package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Datatable service errors.
var (
	ErrDatatableNotFound = errors.New("datatable not found")
)

// DatatableService handles datatable management.
type DatatableService struct {
	datatableRepo repository.DatatableRepository
	fedTableRepo  repository.FedTableRepository
	db            *gorm.DB
	kusciaClient  *kuscia.Client
}

// NewDatatableService creates a new DatatableService.
func NewDatatableService(
	datatableRepo repository.DatatableRepository,
	fedTableRepo repository.FedTableRepository,
	db *gorm.DB,
	kusciaClient *kuscia.Client,
) *DatatableService {
	return &DatatableService{
		datatableRepo: datatableRepo,
		fedTableRepo:  fedTableRepo,
		db:            db,
		kusciaClient:  kusciaClient,
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

// --- Compat (camelCase) DTOs aligned with the frontend OpenAPI contract ---

// TableColumnCompat represents a table column in the frontend contract.
type TableColumnCompat struct {
	ColName        string `json:"colName"`
	ColType        string `json:"colType"`
	ColComment     string `json:"colComment"`
	Classification string `json:"classification,omitempty"`
}

// CreateDatatableCompatRequest represents a datatable creation request (frontend contract).
type CreateDatatableCompatRequest struct {
	OwnerID        string              `json:"ownerId" binding:"required"`
	NodeIDs        []string            `json:"nodeIds" binding:"required"`
	DatatableName  string              `json:"datatableName" binding:"required"`
	DatasourceID   string              `json:"datasourceId"`
	DatasourceName string              `json:"datasourceName"`
	DatasourceType string              `json:"datasourceType"`
	RelativeURI    string              `json:"relativeUri"`
	Columns        []TableColumnCompat `json:"columns"`
	Desc           string              `json:"desc"`
	NullStrs       []string            `json:"nullStrs"`
}

// DataTableNodeInfo represents a created datatable node entry.
type DataTableNodeInfo struct {
	DomainDataID string `json:"domainDataId"`
	NodeID       string `json:"nodeId"`
}

// CreateDatatableCompatVO represents a datatable creation response.
type CreateDatatableCompatVO struct {
	DataTableNodeInfos []DataTableNodeInfo `json:"dataTableNodeInfos"`
	FailedCreatedNodes map[string]string   `json:"failedCreatedNodes,omitempty"`
}

// GetDatatableCompatRequest represents a datatable detail request (frontend contract).
type GetDatatableCompatRequest struct {
	NodeID         string `json:"nodeId"`
	DatatableID    string `json:"datatableId"`
	DatasourceType string `json:"datasourceType"`
	Type           string `json:"type"`
	TeeNodeID      string `json:"teeNodeId"`
}

// DatatableCompatVO represents a datatable view object (frontend contract, camelCase).
type DatatableCompatVO struct {
	DatatableID     string              `json:"datatableId"`
	DatatableName   string              `json:"datatableName"`
	Status          string              `json:"status"`
	PushToTeeStatus string              `json:"pushToTeeStatus,omitempty"`
	PushToTeeErrMsg string              `json:"pushToTeeErrMsg,omitempty"`
	DatasourceID    string              `json:"datasourceId"`
	DatasourceType  string              `json:"datasourceType"`
	DatasourceName  string              `json:"datasourceName"`
	NodeID          string              `json:"nodeId"`
	RelativeURI     string              `json:"relativeUri"`
	Type            string              `json:"type"`
	Description     string              `json:"description"`
	Schema          []TableColumnCompat `json:"schema"`
	NullStrs        []string            `json:"nullStrs,omitempty"`
}

// DatatableNodeCompatVO represents a datatable-with-node view object (frontend contract).
type DatatableNodeCompatVO struct {
	DatatableVO *DatatableCompatVO `json:"datatableVO"`
	NodeName    string             `json:"nodeName"`
	NodeID      string             `json:"nodeId"`
}

// PushDatatableToTeeRequest represents a push-to-TEE request (frontend contract).
type PushDatatableToTeeRequest struct {
	NodeID       string `json:"nodeId" binding:"required"`
	DatatableID  string `json:"datatableId" binding:"required"`
	TeeNodeID    string `json:"teeNodeId"`
	DatasourceID string `json:"datasourceId"`
	RelativeURI  string `json:"relativeUri"`
}

// --- Compat Service Methods ---

// CreateDatatableCompat creates a datatable on each target node via Kuscia DomainData (best-effort).
func (s *DatatableService) CreateDatatableCompat(ctx context.Context, req *CreateDatatableCompatRequest) (*CreateDatatableCompatVO, error) {
	vo := &CreateDatatableCompatVO{
		DataTableNodeInfos: make([]DataTableNodeInfo, 0, len(req.NodeIDs)),
		FailedCreatedNodes: make(map[string]string),
	}

	columns := make([]kuscia.DataColumn, 0, len(req.Columns))
	for _, c := range req.Columns {
		columns = append(columns, kuscia.DataColumn{
			Name:        c.ColName,
			Type:        c.ColType,
			Description: c.ColComment,
		})
	}

	for _, nodeID := range req.NodeIDs {
		domainDataID := uuid.New().String()[:8]

		created := true
		if s.kusciaClient != nil {
			err := s.kusciaClient.CreateDomainData(ctx, &kuscia.CreateDomainDataRequest{
				DomainID:     nodeID,
				DomainDataID: domainDataID,
				Name:         req.DatatableName,
				Type:         "table",
				RelativeURI:  req.RelativeURI,
				DatasourceID: req.DatasourceID,
				Columns:      columns,
				Author:       req.OwnerID,
			})
			if err != nil {
				created = false
				vo.FailedCreatedNodes[nodeID] = err.Error()
			}
		}

		if created {
			vo.DataTableNodeInfos = append(vo.DataTableNodeInfos, DataTableNodeInfo{
				DomainDataID: domainDataID,
				NodeID:       nodeID,
			})
		}
	}

	if len(vo.DataTableNodeInfos) == 0 && len(req.NodeIDs) > 0 {
		return vo, errors.New("all nodes failed to create datatable")
	}

	return vo, nil
}

// GetDatatableCompat retrieves a datatable detail in the frontend contract shape.
func (s *DatatableService) GetDatatableCompat(ctx context.Context, req *GetDatatableCompatRequest) (*DatatableNodeCompatVO, error) {
	vo := &DatatableNodeCompatVO{NodeID: req.NodeID}

	// Resolve node name.
	var node model.NodeDO
	if err := s.db.WithContext(ctx).Where("node_id = ?", req.NodeID).First(&node).Error; err == nil {
		vo.NodeName = node.Name
	}

	dtVO := &DatatableCompatVO{
		DatatableID:    req.DatatableID,
		NodeID:         req.NodeID,
		DatasourceType: req.DatasourceType,
		Type:           req.Type,
		Status:         "Available",
		Schema:         make([]TableColumnCompat, 0),
	}

	// Prefer Kuscia for full metadata; degrade gracefully when unavailable.
	if s.kusciaClient != nil {
		if resp, err := s.kusciaClient.QueryDomainData(ctx, req.NodeID, req.DatatableID); err == nil {
			dtVO.DatatableName = resp.Data.Name
			dtVO.RelativeURI = resp.Data.RelativeURI
			dtVO.DatasourceID = resp.Data.DatasourceID
			if dtVO.Type == "" {
				dtVO.Type = resp.Data.Type
			}
			for _, c := range resp.Data.Columns {
				dtVO.Schema = append(dtVO.Schema, TableColumnCompat{
					ColName:    c.Name,
					ColType:    c.Type,
					ColComment: c.Description,
				})
			}
		}
	}

	// Resolve datasource name/type from the datasource id.
	if dtVO.DatasourceID != "" {
		var ds model.DatasourceDO
		if err := s.db.WithContext(ctx).Where("datasource_id = ?", dtVO.DatasourceID).First(&ds).Error; err == nil {
			dtVO.DatasourceName = ds.Name
			if dtVO.DatasourceType == "" {
				dtVO.DatasourceType = ds.Type
			}
		}
	}

	// Attach latest push-to-TEE status if present.
	var teeMgmt model.TeeNodeDatatableManagementDO
	teeQuery := s.db.WithContext(ctx).Where("datatable_id = ? AND node_id = ?", req.DatatableID, req.NodeID)
	if req.TeeNodeID != "" {
		teeQuery = teeQuery.Where("tee_node_id = ?", req.TeeNodeID)
	}
	if err := teeQuery.Order("gmt_create DESC").First(&teeMgmt).Error; err == nil {
		dtVO.PushToTeeStatus = teeMgmt.Status
		dtVO.PushToTeeErrMsg = teeMgmt.ErrMsg
	}

	vo.DatatableVO = dtVO
	return vo, nil
}

// PushDatatableToTee records a push-to-TEE authorization for a datatable.
func (s *DatatableService) PushDatatableToTee(ctx context.Context, req *PushDatatableToTeeRequest) error {
	teeNodeID := req.TeeNodeID
	if teeNodeID == "" {
		teeNodeID = "tee"
	}

	mgmt := &model.TeeNodeDatatableManagementDO{
		NodeID:       req.NodeID,
		TeeNodeID:    teeNodeID,
		DatatableID:  req.DatatableID,
		DatasourceID: req.DatasourceID,
		Kind:         "PUSH",
		JobID:        uuid.New().String()[:8],
		Status:       "SUCCESS",
		OperateInfo:  req.RelativeURI,
	}

	// Best-effort grant to the TEE domain via Kuscia.
	if s.kusciaClient != nil {
		if err := s.kusciaClient.GrantDomainData(ctx, &kuscia.GrantDomainDataRequest{
			DomainID:     req.NodeID,
			DomainDataID: req.DatatableID,
			GrantDomain:  teeNodeID,
		}); err != nil {
			mgmt.Status = "FAILED"
			mgmt.ErrMsg = err.Error()
		}
	}

	return s.db.WithContext(ctx).Create(mgmt).Error
}
