package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/dao/sqlutil"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Project service errors.
var (
	ErrProjectNotFound         = errors.New("project not found")
	ErrProjectDatatableInvalid = errors.New("project datatable request missing required ids")
)

// firstNonEmpty returns the first non-empty string from the given values.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ProjectService handles project management (Java ProjectServiceImpl).
type ProjectService struct {
	projectRepo          repository.ProjectRepository
	projectInstRepo      repository.ProjectInstRepository
	projectNodeRepo      repository.ProjectNodeRepository
	projectDatatableRepo repository.DatatableRepository
	db                   *gorm.DB
	kuscia               GraphKusciaAPI
	// teeAuth pushes / authorizes datatables of TEE projects (DatatableService).
	teeAuth TeeDatatableAuthorizer
}

// SetTeeAuthorizer wires the TEE push/auth jobs of datatable/add (TEE projects).
func (s *ProjectService) SetTeeAuthorizer(a TeeDatatableAuthorizer) *ProjectService {
	s.teeAuth = a
	return s
}

// NewProjectService creates a new ProjectService.
func NewProjectService(
	projectRepo repository.ProjectRepository,
	projectInstRepo repository.ProjectInstRepository,
	projectNodeRepo repository.ProjectNodeRepository,
	projectDatatableRepo repository.DatatableRepository,
	db *gorm.DB,
	kusciaClient *kuscia.Client,
) *ProjectService {
	return &ProjectService{
		projectRepo:          projectRepo,
		projectInstRepo:      projectInstRepo,
		projectNodeRepo:      projectNodeRepo,
		projectDatatableRepo: projectDatatableRepo,
		db:                   db,
		kuscia:               toGraphKuscia(kusciaClient),
	}
}

// SetKuscia overrides the Kuscia API (tests).
func (s *ProjectService) SetKuscia(k GraphKusciaAPI) { s.kuscia = k }

// --- DTOs (Java camelCase contract) ---

// CreateProjectRequest mirrors Java CreateProjectRequest (instId / nodeIds are Go extensions).
type CreateProjectRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	ComputeMode string   `json:"computeMode"`
	ComputeFunc string   `json:"computeFunc"`
	TeeNodeID   string   `json:"teeNodeId"`
	InstID      string   `json:"instId"`
	NodeIDs     []string `json:"nodeIds"`
}

// UpdateProjectRequest mirrors Java UpdateProjectRequest.
type UpdateProjectRequest struct {
	ProjectID   string `json:"projectId" binding:"required"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ProjectDatatableBaseVO mirrors Java ProjectDatatableBaseVO.
type ProjectDatatableBaseVO struct {
	DatatableID   string `json:"datatableId"`
	DatatableName string `json:"datatableName"`
}

// ProjectNodeVO mirrors Java ProjectNodeVO.
type ProjectNodeVO struct {
	NodeID     string                   `json:"nodeId"`
	NodeName   string                   `json:"nodeName"`
	NodeType   string                   `json:"nodeType,omitempty"`
	Datatables []ProjectDatatableBaseVO `json:"datatables,omitempty"`
}

// ProjectVO mirrors Java ProjectVO (ownerId / instId / nodeIds / computeFunc are kept for compatibility).
type ProjectVO struct {
	ProjectID   string          `json:"projectId"`
	ProjectName string          `json:"projectName"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Nodes       []ProjectNodeVO `json:"nodes"`
	GraphCount  int64           `json:"graphCount"`
	JobCount    int64           `json:"jobCount"`
	GmtCreate   string          `json:"gmtCreate"`
	ComputeMode string          `json:"computeMode"`
	ComputeFunc string          `json:"computeFunc,omitempty"`
	TeeNodeID   string          `json:"teeNodeId"`
	OwnerID     string          `json:"ownerId,omitempty"`
	InstID      string          `json:"instId,omitempty"`
	NodeIDs     []string        `json:"nodeIds,omitempty"`
}

// ProjectListResponse is the paginated list (Go extension, only when paging is requested).
type ProjectListResponse struct {
	Projects []ProjectVO `json:"projects"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	Size     int         `json:"size"`
}

// ProjectListRequest is the project/list request (paging is optional).
type ProjectListRequest struct {
	Page int    `json:"page"`
	Size int    `json:"size"`
	Name string `json:"name"`
}

func normalizeComputeMode(m string) string {
	switch strings.ToUpper(strings.TrimSpace(m)) {
	case "TEE":
		return "TEE"
	case "", "MPC", "FL":
		return "MPC"
	default:
		return strings.ToUpper(m)
	}
}

func (s *ProjectService) openProject(ctx context.Context, projectID string) (*model.ProjectDO, error) {
	p, err := s.projectRepo.FindByProjectID(ctx, projectID)
	if err != nil || p == nil {
		return nil, padErrBase(ErrProjectNotFound, errcode.ProjectNotExists)
	}
	return p, nil
}

// CreateProject mirrors ProjectServiceImpl.createProject.
func (s *ProjectService) CreateProject(ctx context.Context, req *CreateProjectRequest, ownerID string) (*ProjectVO, error) {
	mode := normalizeComputeMode(req.ComputeMode)
	if mode == "TEE" && req.TeeNodeID == "" {
		return nil, padErr(errcode.SystemValidationError, "teeNodeId can not be empty in TEE mode")
	}
	info, _ := json.Marshal(map[string]string{"teeDomainId": req.TeeNodeID})
	project := &model.ProjectDO{
		ProjectID:   graphjob.RandomLetters(8),
		Name:        req.Name,
		Description: req.Description,
		ComputeMode: mode,
		ComputeFunc: firstNonEmpty(req.ComputeFunc, "ALL"),
		ProjectInfo: string(info),
		OwnerID:     ownerID,
	}
	nodeIDs := append([]string(nil), req.NodeIDs...)
	if op := OperatorFrom(ctx); op.isEdge() && op.OwnerID != "" {
		nodeIDs = append(nodeIDs, op.OwnerID)
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(project).Error; err != nil {
			return err
		}
		if req.InstID != "" {
			if err := upsertProjectInst(tx, project.ProjectID, req.InstID); err != nil {
				return err
			}
		}
		for _, n := range nodeIDs {
			if err := upsertProjectNode(tx, project.ProjectID, n); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.toProjectVO(ctx, project, false), nil
}

// GetProject mirrors ProjectServiceImpl.getProject (nodes with imported datatables).
func (s *ProjectService) GetProject(ctx context.Context, projectID string) (*ProjectVO, error) {
	p, err := s.openProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return s.toProjectVO(ctx, p, true), nil
}

// ListAllProjects mirrors ProjectServiceImpl.listProject: every project (EDGE
// users only see the projects of their node).
func (s *ProjectService) ListAllProjects(ctx context.Context, name string) ([]ProjectVO, error) {
	q := s.db.WithContext(ctx).Model(&model.ProjectDO{})
	if op := OperatorFrom(ctx); op.isEdge() && op.OwnerID != "" {
		q = q.Where("project_id IN (?)", s.db.Model(&model.ProjectNodeDO{}).Select("project_id").Where("node_id = ?", op.OwnerID))
	}
	if name != "" {
		q = q.Where("name LIKE ? ESCAPE '!'", sqlutil.Contains(name))
	}
	var projects []model.ProjectDO
	if err := q.Order("gmt_create DESC").Find(&projects).Error; err != nil {
		return nil, err
	}
	out := make([]ProjectVO, 0, len(projects))
	for i := range projects {
		out = append(out, *s.toProjectVO(ctx, &projects[i], false))
	}
	return out, nil
}

// ListProjects returns a page of projects (Go extension for callers sending page/size).
func (s *ProjectService) ListProjects(ctx context.Context, page, size int, name string) (*ProjectListResponse, error) {
	all, err := s.ListAllProjects(ctx, name)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	start := (page - 1) * size
	if start > len(all) {
		start = len(all)
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return &ProjectListResponse{Projects: all[start:end], Total: int64(len(all)), Page: page, Size: size}, nil
}

// UpdateProject mirrors ProjectServiceImpl.updateProject.
func (s *ProjectService) UpdateProject(ctx context.Context, req *UpdateProjectRequest) error {
	project, err := s.openProject(ctx, req.ProjectID)
	if err != nil {
		return err
	}
	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = req.Description
	}
	return s.projectRepo.Update(ctx, project)
}

// DeleteProject mirrors ProjectServiceImpl.deleteProject: refused while the
// project still has graphs.
func (s *ProjectService) DeleteProject(ctx context.Context, projectID string) error {
	project, err := s.openProject(ctx, projectID)
	if err != nil {
		return err
	}
	var graphs int64
	s.db.WithContext(ctx).Model(&model.ProjectGraphDO{}).Where("project_id = ?", projectID).Count(&graphs)
	if graphs > 0 {
		return padErr(errcode.ProjectGraphNotEmpty)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&model.ProjectNodeDO{}, &model.ProjectInstDO{}, &model.ProjectDatatableDO{}} {
			if err := tx.Unscoped().Where("project_id = ?", projectID).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&model.ProjectDO{}, project.ID).Error
	})
}

func upsertProjectNode(tx *gorm.DB, projectID, nodeID string) error {
	var existing model.ProjectNodeDO
	err := tx.Unscoped().Where("project_id = ? AND node_id = ?", projectID, nodeID).First(&existing).Error
	if err == nil {
		if existing.DeletedAt.Valid {
			return tx.Unscoped().Model(&existing).Updates(map[string]any{"deleted_at": nil, "is_deleted": 0}).Error
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&model.ProjectNodeDO{ProjectID: projectID, NodeID: nodeID}).Error
}

func upsertProjectInst(tx *gorm.DB, projectID, instID string) error {
	var existing model.ProjectInstDO
	err := tx.Unscoped().Where("project_id = ? AND inst_id = ?", projectID, instID).First(&existing).Error
	if err == nil {
		if existing.DeletedAt.Valid {
			return tx.Unscoped().Model(&existing).Updates(map[string]any{"deleted_at": nil, "is_deleted": 0}).Error
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&model.ProjectInstDO{ProjectID: projectID, InstID: instID}).Error
}

// AddNode mirrors addNodeToProject (existence checks, idempotent).
func (s *ProjectService) AddNode(ctx context.Context, projectID, nodeID string) error {
	project, err := s.openProject(ctx, projectID)
	if err != nil {
		return err
	}
	var n model.NodeDO
	if err := s.db.WithContext(ctx).Where("node_id = ?", nodeID).First(&n).Error; err != nil {
		return padErr(errcode.NodeNotExistError)
	}
	if err := upsertProjectNode(s.db.WithContext(ctx), projectID, nodeID); err != nil {
		return err
	}
	return s.projectRepo.Update(ctx, project) // touch gmt_modified
}

// AddInst mirrors addInstToProject (existence checks, idempotent).
func (s *ProjectService) AddInst(ctx context.Context, projectID, instID string) error {
	if _, err := s.openProject(ctx, projectID); err != nil {
		return err
	}
	var inst model.InstDO
	if err := s.db.WithContext(ctx).Where("inst_id = ?", instID).First(&inst).Error; err != nil {
		return padErr(errcode.InstNotExists)
	}
	return upsertProjectInst(s.db.WithContext(ctx), projectID, instID)
}

// ArchiveProject archives a project (sets status to archived).
func (s *ProjectService) ArchiveProject(ctx context.Context, projectID string) error {
	project, err := s.openProject(ctx, projectID)
	if err != nil {
		return err
	}
	project.Status = 1
	return s.projectRepo.Update(ctx, project)
}

func (s *ProjectService) toProjectVO(ctx context.Context, p *model.ProjectDO, withDatatables bool) *ProjectVO {
	vo := &ProjectVO{
		ProjectID:   p.ProjectID,
		ProjectName: p.Name,
		Name:        p.Name,
		Description: p.Description,
		ComputeMode: normalizeComputeMode(p.ComputeMode),
		ComputeFunc: p.ComputeFunc,
		TeeNodeID:   projectTeeNodeID(p),
		OwnerID:     p.OwnerID,
		GmtCreate:   rfc3339(p.GmtCreate),
		Nodes:       []ProjectNodeVO{},
	}
	s.db.WithContext(ctx).Model(&model.ProjectGraphDO{}).Where("project_id = ?", p.ProjectID).Count(&vo.GraphCount)
	s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).Where("project_id = ?", p.ProjectID).Count(&vo.JobCount)
	var insts []model.ProjectInstDO
	s.db.WithContext(ctx).Where("project_id = ?", p.ProjectID).Order("id ASC").Find(&insts)
	if len(insts) > 0 {
		vo.InstID = insts[0].InstID
	}
	var pns []model.ProjectNodeDO
	s.db.WithContext(ctx).Where("project_id = ?", p.ProjectID).Order("id ASC").Find(&pns)
	dtsByNode := map[string][]ProjectDatatableBaseVO{}
	if withDatatables {
		var dts []model.ProjectDatatableDO
		s.db.WithContext(ctx).Where("project_id = ? AND source = ?", p.ProjectID, "IMPORTED").Order("id ASC").Find(&dts)
		for _, dt := range dts {
			name := dt.DatatableID
			if s.kuscia != nil {
				if dd, err := s.kuscia.QueryDomainData(ctx, dt.NodeID, dt.DatatableID); err == nil && dd != nil && dd.Data.Name != "" {
					name = dd.Data.Name
				}
			}
			dtsByNode[dt.NodeID] = append(dtsByNode[dt.NodeID], ProjectDatatableBaseVO{DatatableID: dt.DatatableID, DatatableName: name})
		}
	}
	for _, pn := range pns {
		nvo := ProjectNodeVO{NodeID: pn.NodeID, NodeName: pn.NodeID}
		var n model.NodeDO
		if s.db.WithContext(ctx).Where("node_id = ?", pn.NodeID).First(&n).Error == nil {
			nvo.NodeName, nvo.NodeType = n.Name, n.Type
		}
		if withDatatables {
			nvo.Datatables = dtsByNode[pn.NodeID]
			if nvo.Datatables == nil {
				nvo.Datatables = []ProjectDatatableBaseVO{}
			}
		}
		vo.Nodes = append(vo.Nodes, nvo)
		vo.NodeIDs = append(vo.NodeIDs, pn.NodeID)
	}
	return vo
}

// --- Project Datatable ---

// TableColumnConfigParam mirrors Java TableColumnConfigParam.
type TableColumnConfigParam struct {
	ColName        string `json:"colName"`
	IsAssociateKey bool   `json:"isAssociateKey"`
	IsGroupKey     bool   `json:"isGroupKey"`
	IsLabelKey     bool   `json:"isLabelKey"`
	IsProtection   bool   `json:"isProtection"`
}

// AddDatatableRequest mirrors Java AddProjectDatatableRequest.
type AddDatatableRequest struct {
	ProjectID    string                   `json:"projectId"`
	NodeID       string                   `json:"nodeId"`
	DatatableID  string                   `json:"datatableId"`
	DatasourceID string                   `json:"datasourceId"`
	TeeNodeID    string                   `json:"teeNodeId"`
	Type         string                   `json:"type"`
	Configs      []TableColumnConfigParam `json:"configs"`
}

// ProjDeleteDatatableRequest mirrors Java DeleteProjectDatatableRequest.
type ProjDeleteDatatableRequest struct {
	ProjectID   string `json:"projectId"`
	NodeID      string `json:"nodeId"`
	DatatableID string `json:"datatableId"`
	Type        string `json:"type"`
}

// ProjGetDatatableRequest mirrors Java GetProjectDatatableRequest.
type ProjGetDatatableRequest struct {
	ProjectID   string `json:"projectId"`
	NodeID      string `json:"nodeId"`
	DatatableID string `json:"datatableId"`
	Type        string `json:"type"`
}

// TableColumnConfigVO mirrors Java TableColumnConfigVO.
type TableColumnConfigVO struct {
	ColName        string `json:"colName"`
	ColType        string `json:"colType"`
	ColComment     string `json:"colComment"`
	IsAssociateKey bool   `json:"isAssociateKey"`
	IsGroupKey     bool   `json:"isGroupKey"`
	IsLabelKey     bool   `json:"isLabelKey"`
	IsProtection   bool   `json:"isProtection"`
}

// ProjectDatatableVO mirrors Java ProjectDatatableVO; datatableVO / nodeId /
// nodeName keep the DatatableNodeVO shape the new frontend validates.
type ProjectDatatableVO struct {
	DatatableID   string                `json:"datatableId"`
	DatatableName string                `json:"datatableName"`
	Configs       []TableColumnConfigVO `json:"configs"`
	NodeID        string                `json:"nodeId"`
	NodeName      string                `json:"nodeName"`
	DatatableVO   *DatatableVO          `json:"datatableVO,omitempty"`
}

func configVOs(cfgs []graphjob.ColumnConfig) []TableColumnConfigVO {
	out := make([]TableColumnConfigVO, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, TableColumnConfigVO(c))
	}
	return out
}

// mergeColumnFlags mirrors TableColumnConfig.from(column, configs...): the
// column schema is kept and the four flags come from the request.
func mergeColumnFlags(cols []graphjob.ColumnConfig, params []TableColumnConfigParam) []graphjob.ColumnConfig {
	byName := make(map[string]TableColumnConfigParam, len(params))
	for _, p := range params {
		byName[p.ColName] = p
	}
	out := make([]graphjob.ColumnConfig, 0, len(cols))
	for _, c := range cols {
		p, ok := byName[c.ColName]
		c.IsAssociateKey = ok && p.IsAssociateKey
		c.IsGroupKey = ok && p.IsGroupKey
		c.IsLabelKey = ok && p.IsLabelKey
		c.IsProtection = ok && p.IsProtection
		out = append(out, c)
	}
	return out
}

func (s *ProjectService) requireProjectNode(ctx context.Context, projectID, nodeID string) error {
	var cnt int64
	s.db.WithContext(ctx).Model(&model.ProjectNodeDO{}).Where("project_id = ? AND node_id = ?", projectID, nodeID).Count(&cnt)
	if cnt == 0 {
		return padErr(errcode.ProjectNodeNotExists)
	}
	return nil
}

// datatableColumns loads a datatable's schema from Kuscia (Java openDatatable).
func (s *ProjectService) datatableColumns(ctx context.Context, nodeID, datatableID string, fallback []TableColumnConfigParam) ([]graphjob.ColumnConfig, string, error) {
	if s.kuscia == nil {
		cols := make([]graphjob.ColumnConfig, 0, len(fallback))
		for _, p := range fallback {
			cols = append(cols, graphjob.ColumnConfig{ColName: p.ColName})
		}
		return cols, datatableID, nil
	}
	dd, err := s.kuscia.QueryDomainData(ctx, nodeID, datatableID)
	if err != nil || dd == nil {
		return nil, "", padErr(errcode.DatatableNotExists)
	}
	cols := make([]graphjob.ColumnConfig, 0, len(dd.Data.Columns))
	for _, c := range dd.Data.Columns {
		cols = append(cols, graphjob.ColumnConfig{ColName: c.Name, ColType: c.Type, ColComment: c.Comment})
	}
	return cols, firstNonEmpty(dd.Data.Name, datatableID), nil
}

func featureTableColumns(ft *model.FeatureTableDO) []graphjob.ColumnConfig {
	var cols []map[string]any
	_ = json.Unmarshal([]byte(ft.Columns), &cols)
	out := make([]graphjob.ColumnConfig, 0, len(cols))
	str := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok {
				return v
			}
		}
		return ""
	}
	for _, c := range cols {
		out = append(out, graphjob.ColumnConfig{
			ColName:    str(c, "colName", "col_name", "name"),
			ColType:    str(c, "colType", "col_type", "type"),
			ColComment: str(c, "colComment", "col_comment", "comment"),
		})
	}
	return out
}

// AddDatatable mirrors ProjectServiceImpl.addDatatableToProject.
func (s *ProjectService) AddDatatable(ctx context.Context, req *AddDatatableRequest) error {
	if req.ProjectID == "" || req.NodeID == "" || req.DatatableID == "" {
		return ErrProjectDatatableInvalid
	}
	project, err := s.openProject(ctx, req.ProjectID)
	if err != nil {
		return err
	}
	if err := s.requireProjectNode(ctx, req.ProjectID, req.NodeID); err != nil {
		return err
	}
	datasourceID := firstNonEmpty(req.DatasourceID, graphjob.DefaultDataSourceID)
	if strings.EqualFold(req.Type, "HTTP") {
		var ft model.FeatureTableDO
		if err := s.db.WithContext(ctx).Where("node_id = ? AND feature_table_id = ?", req.NodeID, req.DatatableID).First(&ft).Error; err != nil {
			return padErr(errcode.ProjectFeatureTableNotExists)
		}
		cfgs := mergeColumnFlags(featureTableColumns(&ft), req.Configs)
		return upsertBy(ctx, s.db, &model.ProjectFeatureTableDO{
			ProjectID: req.ProjectID, NodeID: req.NodeID, FeatureTableID: req.DatatableID,
			TableConfigs: graphjob.ColumnConfigsJSON(cfgs), Source: "IMPORTED",
		}, "project_id = ? AND node_id = ? AND feature_table_id = ?", req.ProjectID, req.NodeID, req.DatatableID)
	}
	cols, _, err := s.datatableColumns(ctx, req.NodeID, req.DatatableID, req.Configs)
	if err != nil {
		return err
	}
	var pns []model.ProjectNodeDO
	s.db.WithContext(ctx).Where("project_id = ?", req.ProjectID).Find(&pns)
	for _, pn := range pns {
		if strings.EqualFold(pn.NodeID, req.NodeID) {
			continue
		}
		if err := grantDomainData(ctx, s.kuscia, req.NodeID, pn.NodeID, req.DatatableID); err != nil {
			return padErr(errcode.DatatableCreateDatatableGrantFailed, err.Error())
		}
	}
	if isTeeComputeMode(project.ComputeMode) {
		// Java: check routes with the TEE node, push the table when needed and
		// authorize it to the project (TeeJob kind Auth).
		if s.teeAuth == nil {
			return padErr(errcode.DatatableCreateTeeJobFailed, "tee authorizer not configured")
		}
		// teeNodeId may be blank: the datatable service falls back to tee.domain-id.
		if _, err := s.teeAuth.AuthDatatableToTee(ctx, req.ProjectID, req.NodeID, req.DatatableID, req.TeeNodeID, datasourceID); err != nil {
			return err
		}
	}
	return upsertBy(ctx, s.db, &model.ProjectDatatableDO{
		ProjectID: req.ProjectID, NodeID: req.NodeID, DatatableID: req.DatatableID,
		TableConfigs: graphjob.ColumnConfigsJSON(mergeColumnFlags(cols, req.Configs)), Source: "IMPORTED",
	}, "project_id = ? AND node_id = ? AND datatable_id = ?", req.ProjectID, req.NodeID, req.DatatableID)
}

// DeleteDatatable mirrors deleteDatatableToProject (must exist).
func (s *ProjectService) DeleteDatatable(ctx context.Context, req *ProjDeleteDatatableRequest) error {
	if req.ProjectID == "" || req.NodeID == "" || req.DatatableID == "" {
		return ErrProjectDatatableInvalid
	}
	if strings.EqualFold(req.Type, "HTTP") {
		res := s.db.WithContext(ctx).Unscoped().Where("project_id = ? AND node_id = ? AND feature_table_id = ?", req.ProjectID, req.NodeID, req.DatatableID).
			Delete(&model.ProjectFeatureTableDO{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return padErr(errcode.ProjectFeatureTableNotExists)
		}
		return nil
	}
	res := s.db.WithContext(ctx).Unscoped().Where("project_id = ? AND node_id = ? AND datatable_id = ?", req.ProjectID, req.NodeID, req.DatatableID).
		Delete(&model.ProjectDatatableDO{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return padErr(errcode.ProjectDatatableNotExists)
	}
	return nil
}

// GetDatatable mirrors getProjectDatatable: the project level column config.
func (s *ProjectService) GetDatatable(ctx context.Context, req *ProjGetDatatableRequest) (*ProjectDatatableVO, error) {
	if _, err := s.openProject(ctx, req.ProjectID); err != nil {
		return nil, err
	}
	vo := &ProjectDatatableVO{DatatableID: req.DatatableID, NodeID: req.NodeID, NodeName: nodeName(ctx, s.db, req.NodeID)}
	if strings.EqualFold(req.Type, "HTTP") {
		var pft model.ProjectFeatureTableDO
		if err := s.db.WithContext(ctx).Where("project_id = ? AND node_id = ? AND feature_table_id = ?", req.ProjectID, req.NodeID, req.DatatableID).First(&pft).Error; err != nil {
			return nil, padErr(errcode.ProjectFeatureTableNotExists)
		}
		var ft model.FeatureTableDO
		if err := s.db.WithContext(ctx).Where("node_id = ? AND feature_table_id = ?", req.NodeID, req.DatatableID).First(&ft).Error; err != nil {
			return nil, padErr(errcode.ProjectFeatureTableNotExists)
		}
		vo.DatatableName = ft.FeatureTableName
		vo.Configs = configVOs(graphjob.ParseColumnConfigs(pft.TableConfigs))
		vo.DatatableVO = &DatatableVO{DatatableID: req.DatatableID, DatatableName: ft.FeatureTableName, NodeID: req.NodeID, Type: "HTTP"}
		return vo, nil
	}
	var pd model.ProjectDatatableDO
	if err := s.db.WithContext(ctx).Where("project_id = ? AND node_id = ? AND datatable_id = ?", req.ProjectID, req.NodeID, req.DatatableID).First(&pd).Error; err != nil {
		return nil, padErr(errcode.ProjectDatatableNotExists)
	}
	cfgs := graphjob.ParseColumnConfigs(pd.TableConfigs)
	vo.Configs = configVOs(cfgs)
	vo.DatatableName = req.DatatableID
	dvo := &DatatableVO{DatatableID: req.DatatableID, DatatableName: req.DatatableID, NodeID: req.NodeID}
	if s.kuscia != nil {
		if dd, err := s.kuscia.QueryDomainData(ctx, req.NodeID, req.DatatableID); err == nil && dd != nil {
			vo.DatatableName = firstNonEmpty(dd.Data.Name, req.DatatableID)
			dvo.DatatableName = vo.DatatableName
			dvo.DatasourceID = dd.Data.DatasourceID
			dvo.RelativeURI = dd.Data.RelativeURI
			dvo.Type = dd.Data.Type
		}
	}
	vo.DatatableVO = dvo
	return vo, nil
}

// UpdateTableConfig mirrors updateProjectTableConfig: the association must
// exist; columns keep their schema and take the four flags from the request.
func (s *ProjectService) UpdateTableConfig(ctx context.Context, req *AddDatatableRequest) error {
	if req.ProjectID == "" || req.DatatableID == "" {
		return ErrProjectDatatableInvalid
	}
	if strings.EqualFold(req.Type, "HTTP") {
		var pft model.ProjectFeatureTableDO
		if err := s.db.WithContext(ctx).Where("project_id = ? AND node_id = ? AND feature_table_id = ?", req.ProjectID, req.NodeID, req.DatatableID).First(&pft).Error; err != nil {
			return padErr(errcode.ProjectFeatureTableNotExists)
		}
		pft.TableConfigs = graphjob.ColumnConfigsJSON(mergeColumnFlags(graphjob.ParseColumnConfigs(pft.TableConfigs), req.Configs))
		return s.db.WithContext(ctx).Save(&pft).Error
	}
	q := s.db.WithContext(ctx).Where("project_id = ? AND datatable_id = ?", req.ProjectID, req.DatatableID)
	if req.NodeID != "" {
		q = q.Where("node_id = ?", req.NodeID)
	}
	var pd model.ProjectDatatableDO
	if err := q.Order("id ASC").First(&pd).Error; err != nil {
		return padErr(errcode.ProjectDatatableNotExists)
	}
	pd.TableConfigs = graphjob.ColumnConfigsJSON(mergeColumnFlags(graphjob.ParseColumnConfigs(pd.TableConfigs), req.Configs))
	return s.db.WithContext(ctx).Save(&pd).Error
}

// --- Project Datasource aggregation ---

// ProjectDatasourceNodeVO mirrors Java ProjectGraphDomainDataSourceVO.
type ProjectDatasourceNodeVO struct {
	NodeID      string                  `json:"nodeId"`
	NodeName    string                  `json:"nodeName"`
	DataSources []ProjectDataSourceItem `json:"dataSources"`
}

// ListProjectDatasources mirrors getProjectGraphDomainDataSource: for each
// node the operator controls, the Kuscia datasources (TEE: LOCAL only).
func (s *ProjectService) ListProjectDatasources(ctx context.Context, projectID string) ([]ProjectDatasourceNodeVO, error) {
	if _, err := s.openProject(ctx, projectID); err != nil {
		return nil, err
	}
	nodes := controlNodeIDs(ctx, s.db, projectID)
	sort.Strings(nodes)
	out := make([]ProjectDatasourceNodeVO, 0, len(nodes))
	for _, n := range nodes {
		items, err := domainDataSources(ctx, s.db, s.kuscia, projectID, n)
		if err != nil {
			items = []ProjectDataSourceItem{}
		}
		out = append(out, ProjectDatasourceNodeVO{NodeID: n, NodeName: nodeName(ctx, s.db, n), DataSources: items})
	}
	return out, nil
}

// ProjectGraphOutputVO lists a graph node's declared output tables.
type ProjectGraphOutputVO struct {
	GraphID     string   `json:"graphId"`
	GraphNodeID string   `json:"graphNodeId"`
	CodeName    string   `json:"codeName,omitempty"`
	Label       string   `json:"label,omitempty"`
	Outputs     []string `json:"outputs"`
}

// ProjectOutputVO mirrors Java ProjectOutputVO.
type ProjectOutputVO struct {
	ProjectID   string                 `json:"projectId"`
	ProjectName string                 `json:"projectName"`
	Description string                 `json:"description"`
	ComputeMode string                 `json:"computeMode"`
	GmtCreate   string                 `json:"gmtCreate"`
	GraphCount  int64                  `json:"graphCount"`
	JobCount    int64                  `json:"jobCount"`
	Nodes       []ProjectGraphOutputVO `json:"nodes"`
}

// GetProjectOutTable mirrors getProjectAllOutTable.
func (s *ProjectService) GetProjectOutTable(ctx context.Context, projectID, graphID string) (*ProjectOutputVO, error) {
	project, err := s.openProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	vo := &ProjectOutputVO{
		ProjectID: projectID, ProjectName: project.Name, Description: project.Description,
		ComputeMode: normalizeComputeMode(project.ComputeMode), GmtCreate: rfc3339(project.GmtCreate),
		Nodes: make([]ProjectGraphOutputVO, 0),
	}
	s.db.WithContext(ctx).Model(&model.ProjectGraphDO{}).Where("project_id = ?", projectID).Count(&vo.GraphCount)
	s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).Where("project_id = ?", projectID).Count(&vo.JobCount)
	q := s.db.WithContext(ctx).Where("project_id = ?", projectID)
	if graphID != "" {
		q = q.Where("graph_id = ?", graphID)
	}
	var nodes []model.ProjectGraphNodeDO
	q.Order("id ASC").Find(&nodes)
	for _, gn := range nodes {
		outs := splitOutputs(gn.Outputs)
		if len(outs) == 0 {
			continue
		}
		vo.Nodes = append(vo.Nodes, ProjectGraphOutputVO{GraphID: gn.GraphID, GraphNodeID: gn.GraphNodeID, CodeName: gn.CodeName, Label: gn.Label, Outputs: outs})
	}
	return vo, nil
}
