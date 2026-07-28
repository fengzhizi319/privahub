package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
)

// Project service errors.
var (
	ErrProjectNotFound = errors.New("project not found")
)

// ProjectService handles project management.
type ProjectService struct {
	projectRepo     repository.ProjectRepository
	projectInstRepo repository.ProjectInstRepository
	projectNodeRepo repository.ProjectNodeRepository
}

// NewProjectService creates a new ProjectService.
func NewProjectService(
	projectRepo repository.ProjectRepository,
	projectInstRepo repository.ProjectInstRepository,
	projectNodeRepo repository.ProjectNodeRepository,
) *ProjectService {
	return &ProjectService{
		projectRepo:     projectRepo,
		projectInstRepo: projectInstRepo,
		projectNodeRepo: projectNodeRepo,
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
	ProjectID     string `json:"project_id" binding:"required"`
	DatatableID   string `json:"datatable_id" binding:"required"`
	DatatableName string `json:"datatable_name"`
	NodeID        string `json:"node_id"`
	DatasourceID  string `json:"datasource_id"`
}

// ProjDeleteDatatableRequest represents removing a datatable from a project.
type ProjDeleteDatatableRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
	NodeID      string `json:"node_id"`
}

// ProjGetDatatableRequest represents getting a datatable from a project.
type ProjGetDatatableRequest struct {
	ProjectID   string `json:"project_id" binding:"required"`
	DatatableID string `json:"datatable_id" binding:"required"`
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
