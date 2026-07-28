package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"gorm.io/gorm"
)

// Project service errors.
var (
	ErrProjectNotFound         = errors.New("project not found")
	ErrProjectDatatableInvalid = errors.New("project datatable request missing required ids")
)

// firstNonEmpty returns the first non-empty string from the given values.
// It reconciles the dual-field (snake_case / camelCase) DTO convention so the
// service works whether or not the case-normalization middleware ran.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ProjectService handles project management.
type ProjectService struct {
	projectRepo          repository.ProjectRepository
	projectInstRepo      repository.ProjectInstRepository
	projectNodeRepo      repository.ProjectNodeRepository
	projectDatatableRepo repository.DatatableRepository
	db                   *gorm.DB
}

// NewProjectService creates a new ProjectService.
func NewProjectService(
	projectRepo repository.ProjectRepository,
	projectInstRepo repository.ProjectInstRepository,
	projectNodeRepo repository.ProjectNodeRepository,
	projectDatatableRepo repository.DatatableRepository,
	db *gorm.DB,
) *ProjectService {
	return &ProjectService{
		projectRepo:          projectRepo,
		projectInstRepo:      projectInstRepo,
		projectNodeRepo:      projectNodeRepo,
		projectDatatableRepo: projectDatatableRepo,
		db:                   db,
	}
}

// CreateProjectRequest represents a project creation request.
type CreateProjectRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	ComputeMode string   `json:"compute_mode"`
	ComputeFunc string   `json:"compute_func"`
	InstID      string   `json:"inst_id"`
	NodeIDs     []string `json:"node_ids"`
}

// UpdateProjectRequest represents a project update request.
type UpdateProjectRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ProjectVO represents a project view object.
type ProjectVO struct {
	ProjectID   string   `json:"project_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ComputeMode string   `json:"compute_mode"`
	ComputeFunc string   `json:"compute_func"`
	OwnerID     string   `json:"owner_id"`
	Status      int8     `json:"status"`
	InstID      string   `json:"inst_id,omitempty"`
	NodeIDs     []string `json:"node_ids,omitempty"`
	GmtCreate   string   `json:"gmt_create"`
}

// ProjectListResponse represents a paginated project list.
type ProjectListResponse struct {
	Projects []ProjectVO `json:"projects"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	Size     int         `json:"size"`
}

// CreateProject creates a new project.
func (s *ProjectService) CreateProject(ctx context.Context, req *CreateProjectRequest, ownerID string) (*ProjectVO, error) {
	projectID := uuid.New().String()[:8]

	computeMode := req.ComputeMode
	if computeMode == "" {
		computeMode = "mpc"
	}
	computeFunc := req.ComputeFunc
	if computeFunc == "" {
		computeFunc = "ALL"
	}

	project := &model.ProjectDO{
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
		ComputeMode: computeMode,
		ComputeFunc: computeFunc,
		OwnerID:     ownerID,
	}

	if err := s.projectRepo.Create(ctx, project); err != nil {
		return nil, err
	}

	// Associate institution
	if req.InstID != "" {
		_ = s.projectInstRepo.Create(ctx, &model.ProjectInstDO{
			ProjectID: projectID,
			InstID:    req.InstID,
		})
	}

	// Associate nodes
	for _, nodeID := range req.NodeIDs {
		_ = s.projectNodeRepo.Create(ctx, &model.ProjectNodeDO{
			ProjectID: projectID,
			NodeID:    nodeID,
		})
	}

	return s.toProjectVO(ctx, project), nil
}

// GetProject retrieves a project by ID.
func (s *ProjectService) GetProject(ctx context.Context, projectID string) (*ProjectVO, error) {
	project, err := s.projectRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, ErrProjectNotFound
	}
	return s.toProjectVO(ctx, project), nil
}

// ListProjects retrieves projects with pagination.
func (s *ProjectService) ListProjects(ctx context.Context, page, size int, name string) (*ProjectListResponse, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	projects, total, err := s.projectRepo.PageQuery(ctx, page, size, name)
	if err != nil {
		return nil, err
	}

	result := make([]ProjectVO, 0, len(projects))
	for i := range projects {
		result = append(result, *s.toProjectVO(ctx, &projects[i]))
	}

	return &ProjectListResponse{
		Projects: result,
		Total:    total,
		Page:     page,
		Size:     size,
	}, nil
}

// UpdateProject updates a project.
func (s *ProjectService) UpdateProject(ctx context.Context, req *UpdateProjectRequest) error {
	project, err := s.projectRepo.FindByProjectID(ctx, req.ProjectID)
	if err != nil {
		return ErrProjectNotFound
	}

	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = req.Description
	}

	return s.projectRepo.Update(ctx, project)
}

// DeleteProject deletes a project.
func (s *ProjectService) DeleteProject(ctx context.Context, projectID string) error {
	project, err := s.projectRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return ErrProjectNotFound
	}
	return s.projectRepo.Delete(ctx, project.ID)
}

// AddNode adds a node to a project.
func (s *ProjectService) AddNode(ctx context.Context, projectID, nodeID string) error {
	_, err := s.projectRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return ErrProjectNotFound
	}

	return s.projectNodeRepo.Create(ctx, &model.ProjectNodeDO{
		ProjectID: projectID,
		NodeID:    nodeID,
	})
}

// AddInst adds an institution to a project.
func (s *ProjectService) AddInst(ctx context.Context, projectID, instID string) error {
	_, err := s.projectRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return ErrProjectNotFound
	}

	return s.projectInstRepo.Create(ctx, &model.ProjectInstDO{
		ProjectID: projectID,
		InstID:    instID,
	})
}

// ArchiveProject archives a project (sets status to archived).
func (s *ProjectService) ArchiveProject(ctx context.Context, projectID string) error {
	project, err := s.projectRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return ErrProjectNotFound
	}
	project.Status = 1 // 1 = archived
	return s.projectRepo.Update(ctx, project)
}

// --- Project Datatable DTOs ---

// AddDatatableRequest represents adding a datatable to a project.
type AddDatatableRequest struct {
	ProjectID        string          `json:"project_id"`
	ProjectIDAlt     string          `json:"projectId"`
	DatatableID      string          `json:"datatable_id"`
	DatatableIDAlt   string          `json:"datatableId"`
	DatatableName    string          `json:"datatable_name"`
	DatatableNameAlt string          `json:"datatableName"`
	NodeID           string          `json:"node_id"`
	NodeIDAlt        string          `json:"nodeId"`
	DatasourceID     string          `json:"datasource_id"`
	DatasourceIDAlt  string          `json:"datasourceId"`
	TeeNodeID        string          `json:"tee_node_id"`
	TeeNodeIDAlt     string          `json:"teeNodeId"`
	Type             string          `json:"type"`
	Configs          json.RawMessage `json:"configs"`
}

// ProjDeleteDatatableRequest represents removing a datatable from a project.
type ProjDeleteDatatableRequest struct {
	ProjectID      string `json:"project_id"`
	ProjectIDAlt   string `json:"projectId"`
	DatatableID    string `json:"datatable_id"`
	DatatableIDAlt string `json:"datatableId"`
	NodeID         string `json:"node_id"`
	NodeIDAlt      string `json:"nodeId"`
}

// ProjGetDatatableRequest represents getting a datatable from a project.
type ProjGetDatatableRequest struct {
	ProjectID      string `json:"project_id"`
	ProjectIDAlt   string `json:"projectId"`
	DatatableID    string `json:"datatable_id"`
	DatatableIDAlt string `json:"datatableId"`
}

// ProjectDatatableVO represents a project datatable view object.
type ProjectDatatableVO struct {
	ProjectID     string `json:"project_id"`
	DatatableID   string `json:"datatable_id"`
	DatatableName string `json:"datatable_name"`
	NodeID        string `json:"node_id"`
	DatasourceID  string `json:"datasource_id"`
	GmtCreate     string `json:"gmt_create"`
}

func (s *ProjectService) toProjectVO(ctx context.Context, project *model.ProjectDO) *ProjectVO {
	vo := &ProjectVO{
		ProjectID:   project.ProjectID,
		Name:        project.Name,
		Description: project.Description,
		ComputeMode: project.ComputeMode,
		ComputeFunc: project.ComputeFunc,
		OwnerID:     project.OwnerID,
		Status:      project.Status,
		GmtCreate:   project.GmtCreate.Format("2006-01-02 15:04:05"),
	}

	// Get associated inst
	insts, err := s.projectInstRepo.FindByProjectID(ctx, project.ProjectID)
	if err == nil && len(insts) > 0 {
		vo.InstID = insts[0].InstID
	}

	// Get associated nodes
	nodes, err := s.projectNodeRepo.FindByProjectID(ctx, project.ProjectID)
	if err == nil {
		nodeIDs := make([]string, 0, len(nodes))
		for _, n := range nodes {
			nodeIDs = append(nodeIDs, n.NodeID)
		}
		vo.NodeIDs = nodeIDs
	}

	return vo
}

// --- Project Datatable operations ---

// AddDatatable associates a datatable with a project on a node. The operation
// is idempotent: re-adding an existing association updates its column configs.
// Column configs are persisted verbatim as JSON in ProjectDatatableDO.TableConfigs.
func (s *ProjectService) AddDatatable(ctx context.Context, req *AddDatatableRequest) error {
	projectID := firstNonEmpty(req.ProjectID, req.ProjectIDAlt)
	nodeID := firstNonEmpty(req.NodeID, req.NodeIDAlt)
	datatableID := firstNonEmpty(req.DatatableID, req.DatatableIDAlt)
	if projectID == "" || nodeID == "" || datatableID == "" {
		return ErrProjectDatatableInvalid
	}

	tableConfigs := "[]"
	if len(req.Configs) > 0 {
		tableConfigs = string(req.Configs)
	}

	existing, err := s.projectDatatableRepo.FindByProjectNodeDatatable(ctx, projectID, nodeID, datatableID)
	if err == nil && existing != nil {
		existing.TableConfigs = tableConfigs
		return s.projectDatatableRepo.Update(ctx, existing)
	}

	return s.projectDatatableRepo.Create(ctx, &model.ProjectDatatableDO{
		ProjectID:    projectID,
		NodeID:       nodeID,
		DatatableID:  datatableID,
		TableConfigs: tableConfigs,
		Source:       "IMPORTED",
	})
}

// DeleteDatatable removes a datatable association from a project. It is
// idempotent: deleting a non-existent association is a no-op success.
func (s *ProjectService) DeleteDatatable(ctx context.Context, req *ProjDeleteDatatableRequest) error {
	projectID := firstNonEmpty(req.ProjectID, req.ProjectIDAlt)
	nodeID := firstNonEmpty(req.NodeID, req.NodeIDAlt)
	datatableID := firstNonEmpty(req.DatatableID, req.DatatableIDAlt)
	if projectID == "" || nodeID == "" || datatableID == "" {
		return ErrProjectDatatableInvalid
	}

	existing, err := s.projectDatatableRepo.FindByProjectNodeDatatable(ctx, projectID, nodeID, datatableID)
	if err != nil || existing == nil {
		return nil
	}
	return s.projectDatatableRepo.Delete(ctx, existing.ID)
}

// UpdateTableConfig updates the column configs of a project datatable. If the
// association does not exist yet it is created (upsert) so config edits are
// durable even before an explicit add.
func (s *ProjectService) UpdateTableConfig(ctx context.Context, req *AddDatatableRequest) error {
	projectID := firstNonEmpty(req.ProjectID, req.ProjectIDAlt)
	nodeID := firstNonEmpty(req.NodeID, req.NodeIDAlt)
	datatableID := firstNonEmpty(req.DatatableID, req.DatatableIDAlt)
	if projectID == "" || nodeID == "" || datatableID == "" {
		return ErrProjectDatatableInvalid
	}

	existing, err := s.projectDatatableRepo.FindByProjectNodeDatatable(ctx, projectID, nodeID, datatableID)
	if err != nil || existing == nil {
		return s.AddDatatable(ctx, req)
	}
	if len(req.Configs) > 0 {
		existing.TableConfigs = string(req.Configs)
	}
	return s.projectDatatableRepo.Update(ctx, existing)
}

// --- Project Datasource aggregation ---

// ProjectDataSourceItem is a single datasource available on a project node
// (frontend DataSource contract: dataSourceId/dataSourceName/nodeId/type).
type ProjectDataSourceItem struct {
	DataSourceID   string `json:"dataSourceId"`
	DataSourceName string `json:"dataSourceName"`
	NodeID         string `json:"nodeId"`
	Type           string `json:"type"`
}

// ProjectDatasourceNodeVO groups a project node with its available datasources
// (frontend ProjectGraphDomainDataSourceVO contract).
type ProjectDatasourceNodeVO struct {
	NodeID      string                  `json:"nodeId"`
	NodeName    string                  `json:"nodeName"`
	DataSources []ProjectDataSourceItem `json:"dataSources"`
}

// ListProjectDatasources aggregates the datasources available on each node of a
// project. For every project node it resolves the node name and the datasources
// bound to that node (via datasource_node), returning a per-node grouping that
// matches the frontend project/datasource/list contract. Degrades gracefully to
// an empty list when the project has no nodes or a lookup fails.
func (s *ProjectService) ListProjectDatasources(ctx context.Context, projectID string) ([]ProjectDatasourceNodeVO, error) {
	result := make([]ProjectDatasourceNodeVO, 0)

	pnodes, err := s.projectNodeRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return result, nil
	}

	for _, pn := range pnodes {
		nodeName := ""
		var node model.NodeDO
		if err := s.db.WithContext(ctx).Where("node_id = ?", pn.NodeID).First(&node).Error; err == nil {
			nodeName = node.Name
		}

		items := make([]ProjectDataSourceItem, 0)
		var dsNodes []model.DatasourceNodeDO
		if err := s.db.WithContext(ctx).Where("node_id = ?", pn.NodeID).Find(&dsNodes).Error; err == nil {
			for _, dsn := range dsNodes {
				var ds model.DatasourceDO
				if err := s.db.WithContext(ctx).Where("datasource_id = ?", dsn.DatasourceID).First(&ds).Error; err == nil {
					items = append(items, ProjectDataSourceItem{
						DataSourceID:   ds.DatasourceID,
						DataSourceName: ds.Name,
						NodeID:         pn.NodeID,
						Type:           ds.Type,
					})
				}
			}
		}

		result = append(result, ProjectDatasourceNodeVO{
			NodeID:      pn.NodeID,
			NodeName:    nodeName,
			DataSources: items,
		})
	}

	return result, nil
}
