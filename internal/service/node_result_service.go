package service

import (
	"context"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/sqlutil"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Node result endpoints (node/result/list, node/result/detail), Java
// NodeServiceImpl.listAllNodeResults / getNodeResultDetail: results come
// from project_result (written by the job status sync) enriched with Kuscia
// DomainData metadata. Owned by the graph / results migration work.

// ListNodeResultRequest mirrors Java ListNodeResultRequest.
type ListNodeResultRequest struct {
	OwnerID          string   `json:"ownerId"`
	PageSize         int      `json:"pageSize"`
	PageNumber       int      `json:"pageNumber"`
	NodeNamesFilter  []string `json:"nodeNamesFilter"`
	KindFilters      []string `json:"kindFilters"`
	DataVendorFilter string   `json:"dataVendorFilter"`
	NameFilter       string   `json:"nameFilter"`
	TimeSortingRule  string   `json:"timeSortingRule"`
	TeeNodeID        string   `json:"teeNodeId"`
}

// NodeResultsCompatVO mirrors Java NodeResultsVO.
type NodeResultsCompatVO struct {
	DomainDataID      string `json:"domainDataId,omitempty"`
	DatasourceID      string `json:"datasourceId,omitempty"`
	DatasourceType    string `json:"datasourceType,omitempty"`
	ProductName       string `json:"productName,omitempty"`
	DatatableType     string `json:"datatableType,omitempty"`
	SourceProjectID   string `json:"sourceProjectId,omitempty"`
	SourceProjectName string `json:"sourceProjectName,omitempty"`
	RelativeURI       string `json:"relativeUri,omitempty"`
	JobID             string `json:"jobId,omitempty"`
	TrainFlow         string `json:"trainFlow,omitempty"`
	PullFromTeeStatus string `json:"pullFromTeeStatus,omitempty"`
	PullFromTeeErrMsg string `json:"pullFromTeeErrMsg,omitempty"`
	GmtCreate         string `json:"gmtCreate,omitempty"`
	ComputeMode       string `json:"computeMode,omitempty"`
}

// NodeAllResultsCompatVO mirrors Java NodeAllResultsVO.
type NodeAllResultsCompatVO struct {
	NodeResultsVO *NodeResultsCompatVO `json:"nodeResultsVO,omitempty"`
	NodeID        string               `json:"nodeId,omitempty"`
	NodeName      string               `json:"nodeName,omitempty"`
}

// AllNodeResultsListCompatVO mirrors Java AllNodeResultsListVO.
type AllNodeResultsListCompatVO struct {
	NodeAllResultsVOList []NodeAllResultsCompatVO `json:"nodeAllResultsVOList"`
	TotalNodeResultNums  int                      `json:"totalNodeResultNums"`
}

// GetNodeResultDetailRequest mirrors Java GetNodeResultDetailRequest.
type GetNodeResultDetailRequest struct {
	NodeID       string `json:"nodeId" binding:"required"`
	DomainDataID string `json:"domainDataId" binding:"required"`
	DataType     string `json:"dataType"`
	DataVendor   string `json:"dataVendor"`
}

// NodeResultDetailCompatVO mirrors Java NodeResultDetailVO.
type NodeResultDetailCompatVO struct {
	NodeResultsVO     *NodeResultsCompatVO `json:"nodeResultsVO,omitempty"`
	GraphDetailVO     *GraphDetailVO       `json:"graphDetailVO,omitempty"`
	TableColumnVOList []TableColumnCompat  `json:"tableColumnVOList"`
	Output            *GraphNodeOutputVO   `json:"output,omitempty"`
	Datasource        string               `json:"datasource,omitempty"`
}

// resultKindAliases lets kind filters use either ResultKind names or DomainData types.
func resultKindFilter(filters []string) []string {
	out := make([]string, 0, len(filters))
	for _, f := range filters {
		if k := graphjob.ResultKindFromDatatable(f); k != "" {
			out = append(out, k)
			continue
		}
		out = append(out, f)
	}
	return out
}

func (s *NodeService) nodeResultVO(ctx context.Context, r *model.ProjectResultDO, teeNodeID string) *NodeResultsCompatVO {
	vo := &NodeResultsCompatVO{
		DomainDataID:  r.RefID,
		ProductName:   r.RefID,
		DatatableType: graphjob.OutputTypeFromResultKind(r.Kind),
		JobID:         r.JobID,
		GmtCreate:     rfc3339(r.GmtCreate),
	}
	var p model.ProjectDO
	if s.db.WithContext(ctx).Where("project_id = ?", r.ProjectID).First(&p).Error == nil {
		vo.SourceProjectID, vo.SourceProjectName = p.ProjectID, p.Name
		vo.ComputeMode = normalizeComputeMode(p.ComputeMode)
		var j model.ProjectJobDO
		if s.db.WithContext(ctx).Where("job_id = ?", r.JobID).First(&j).Error == nil {
			vo.TrainFlow = j.Name
		}
	}
	var tee model.TeeNodeDatatableManagementDO
	q := s.db.WithContext(ctx).Where("node_id = ? AND datatable_id = ? AND kind = ?", r.NodeID, r.RefID, TeeJobKindPull)
	if teeNodeID != "" {
		q = q.Where("tee_node_id = ?", teeNodeID)
	}
	if q.
		Order("gmt_create DESC").First(&tee).Error == nil {
		vo.PullFromTeeStatus, vo.PullFromTeeErrMsg = tee.Status, tee.ErrMsg
	}
	return vo
}

func (s *NodeService) enrichFromKuscia(ctx context.Context, nodeID string, vo *NodeResultsCompatVO) {
	if s.kusciaClient == nil {
		return
	}
	dd, err := s.kc(nodeID).QueryDomainData(ctx, nodeID, vo.DomainDataID)
	if err != nil || dd == nil {
		return
	}
	s.applyDomainData(ctx, nodeID, vo, dd)
}

func (s *NodeService) applyDomainData(ctx context.Context, nodeID string, vo *NodeResultsCompatVO, dd *kuscia.QueryDomainDataResponse) {
	vo.RelativeURI = dd.Data.RelativeURI
	vo.DatasourceID = dd.Data.DatasourceID
	vo.DatatableType = firstNonEmpty(dd.Data.Type, vo.DatatableType)
	if list, err := s.kc(nodeID).ListDomainDataSource(ctx, nodeID); err == nil {
		for _, ds := range list {
			if ds.DatasourceID == dd.Data.DatasourceID {
				vo.DatasourceType = kusciaDatasourceTypeToPlatform(ds.Type)
			}
		}
	}
}

// ReadyInstNodes mirrors Java NodeManager.listReadyNode: the nodes of an
// institution whose Kuscia domain is Ready (per-node channel, Java
// getNodeDTOP2P).
func (s *NodeService) ReadyInstNodes(ctx context.Context, instID string) []model.NodeDO {
	var nodes []model.NodeDO
	if instID == "" || s.db.WithContext(ctx).Where("inst_id = ?", instID).Order("id").Find(&nodes).Error != nil {
		return nil
	}
	out := make([]model.NodeDO, 0, len(nodes))
	for i := range nodes {
		vo, err := s.nodeVOP2P(ctx, &nodes[i])
		if err == nil && vo.NodeStatus == kuscia.DomainStatusReady {
			out = append(out, nodes[i])
		}
	}
	return out
}

// ReadyInstNodeIDs mirrors Java NodeManager.listReadyNodeByIds: the Ready
// nodes of an institution, intersected with nodeIDs when given.
func (s *NodeService) ReadyInstNodeIDs(ctx context.Context, instID string, nodeIDs []string) []string {
	want := map[string]bool{}
	for _, id := range nodeIDs {
		want[id] = true
	}
	out := []string{}
	for _, n := range s.ReadyInstNodes(ctx, instID) {
		if len(want) == 0 || want[n.NodeID] {
			out = append(out, n.NodeID)
		}
	}
	return out
}

// ReadyInstNodeIDsByNames mirrors Java NodeManager.listReadyNodeByNames: the
// Ready nodes of an institution, filtered by node name when names are given.
func (s *NodeService) ReadyInstNodeIDsByNames(ctx context.Context, instID string, names []string) []string {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := []string{}
	for _, n := range s.ReadyInstNodes(ctx, instID) {
		if len(want) == 0 || want[n.Name] {
			out = append(out, n.NodeID)
		}
	}
	return distinctStrings(out)
}

// instNodeIDs returns the node ids of an institution (Java
// nodeRepository.findByInstId).
func instNodeIDs(ctx context.Context, db *gorm.DB, instID string) []string {
	ids := []string{}
	if db != nil && instID != "" {
		db.WithContext(ctx).Model(&model.NodeDO{}).Where("inst_id = ?", instID).Order("id").Pluck("node_id", &ids)
	}
	return ids
}

// checkNodesInInst mirrors Java InstServiceImpl.checkNodesInInst: every node
// must belong to the institution (an empty list does not match).
func checkNodesInInst(ctx context.Context, db *gorm.DB, instID string, nodeIDs []string) bool {
	if len(nodeIDs) == 0 {
		return false
	}
	all := map[string]bool{}
	for _, id := range instNodeIDs(ctx, db, instID) {
		all[id] = true
	}
	for _, id := range nodeIDs {
		if !all[id] {
			return false
		}
	}
	return true
}

// resultNodeIDs picks the nodes whose results are listed (Java
// listAllNodeResults): AUTONOMY lists the Ready nodes of the local
// institution (nodeNamesFilter holds node ids there); otherwise only ownerId.
func (s *NodeService) resultNodeIDs(ctx context.Context, req *ListNodeResultRequest) []string {
	if s.isAutonomy() {
		instID := ""
		if e := s.envSvc(); e != nil {
			instID = e.GetInstID()
		}
		return s.ReadyInstNodeIDs(ctx, instID, req.NodeNamesFilter)
	}
	return []string{req.OwnerID}
}

// ListNodeResults mirrors Java listAllNodeResults / listResults /
// NodeManager.listResult:
//   - kindFilters on the result kind;
//   - nameFilter matches the result id (DomainData id), the source project
//     name or the train flow (job) name (Java filterByName);
//   - dataVendorFilter is accepted but, like Java NodeManager.listResult,
//     not applied (results carry no vendor);
//   - the TEE pull status is looked up for teeNodeId (default: the configured
//     TEE node, Java teeNodeId).
func (s *NodeService) ListNodeResults(ctx context.Context, req *ListNodeResultRequest) (*AllNodeResultsListCompatVO, error) {
	vo := &AllNodeResultsListCompatVO{NodeAllResultsVOList: make([]NodeAllResultsCompatVO, 0)}
	nodeIDs := s.resultNodeIDs(ctx, req)
	if len(nodeIDs) == 0 {
		return vo, nil
	}
	q := s.db.WithContext(ctx).Model(&model.ProjectResultDO{}).Where("node_id IN ?", nodeIDs)
	if kinds := resultKindFilter(req.KindFilters); len(kinds) > 0 {
		q = q.Where("kind IN ?", kinds)
	}
	if name := strings.TrimSpace(req.NameFilter); name != "" {
		like := sqlutil.Contains(name)
		q = q.Where("ref_id LIKE ? ESCAPE '!' OR project_id IN (?) OR job_id IN (?)", like,
			s.db.Model(&model.ProjectDO{}).Select("project_id").Where("name LIKE ? ESCAPE '!'", like),
			s.db.Model(&model.ProjectJobDO{}).Select("job_id").Where("name LIKE ? ESCAPE '!'", like))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	order := "gmt_create DESC"
	if strings.EqualFold(req.TimeSortingRule, "ascending") || strings.EqualFold(req.TimeSortingRule, "asc") {
		order = "gmt_create ASC"
	}
	q = q.Order(order).Order("id DESC")
	if req.PageSize > 0 {
		page := req.PageNumber
		if page < 1 {
			page = 1
		}
		q = q.Offset((page - 1) * req.PageSize).Limit(req.PageSize)
	}
	var rows []model.ProjectResultDO
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	teeNodeID := s.teeDomain(req.TeeNodeID)
	for i := range rows {
		r := &rows[i]
		rvo := s.nodeResultVO(ctx, r, teeNodeID)
		s.enrichFromKuscia(ctx, r.NodeID, rvo)
		vo.NodeAllResultsVOList = append(vo.NodeAllResultsVOList, NodeAllResultsCompatVO{
			NodeResultsVO: rvo, NodeID: r.NodeID, NodeName: nodeName(ctx, s.db, r.NodeID),
		})
	}
	vo.TotalNodeResultNums = int(total)
	return vo, nil
}

// teeDomain returns the request TEE node id or the configured one.
func (s *NodeService) teeDomain(req string) string {
	if strings.TrimSpace(req) != "" {
		return strings.TrimSpace(req)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.teeNodeID != "" {
		return s.teeNodeID
	}
	return "tee"
}

// GetNodeResultDetail mirrors Java getNodeResultDetail / NodeManager.getNodeResult:
// the DomainData must exist (QUERY_DATATABLE_FAILED / DATATABLE_NOT_EXISTS),
// the project_result row must exist (PROJECT_RESULT_NOT_FOUND), and
// `datasource` is the path of the datatable's datasource (Java
// dataService.queryDataSources: only default-data-source has a path).
func (s *NodeService) GetNodeResultDetail(ctx context.Context, req *GetNodeResultDetailRequest) (*NodeResultDetailCompatVO, error) {
	var dd *kuscia.QueryDomainDataResponse
	if s.kusciaClient != nil {
		resp, err := s.kc(req.NodeID).QueryDomainData(ctx, req.NodeID, req.DomainDataID)
		switch {
		case err != nil && kuscia.IsNotFound(err):
			return nil, padErr(errcode.DatatableNotExists)
		case err != nil:
			return nil, bizErr(errcode.DatatableQueryDatatableFailed, err.Error())
		case resp == nil:
			return nil, padErr(errcode.DatatableNotExists)
		}
		dd = resp
	}
	var r model.ProjectResultDO
	if err := s.db.WithContext(ctx).Where("node_id = ? AND ref_id = ?", req.NodeID, req.DomainDataID).First(&r).Error; err != nil {
		return nil, padErr(errcode.ProjectResultNotFound)
	}
	vo := &NodeResultDetailCompatVO{NodeResultsVO: s.nodeResultVO(ctx, &r, ""), TableColumnVOList: make([]TableColumnCompat, 0)}
	if dd != nil {
		s.applyDomainData(ctx, req.NodeID, vo.NodeResultsVO, dd)
		vo.Datasource = datasourcePath(dd.Data.DatasourceID)
		for _, c := range dd.Data.Columns {
			vo.TableColumnVOList = append(vo.TableColumnVOList, TableColumnCompat{ColName: c.Name, ColType: c.Type, ColComment: c.Comment})
		}
	}
	js := &JobService{db: s.db, kuscia: toGraphKuscia(s.kusciaClient)}
	if job, err := js.GetJob(ctx, &GetJobRequest{ProjectID: r.ProjectID, JobID: r.JobID}); err == nil {
		vo.GraphDetailVO = job.Graph
	}
	var task model.ProjectJobTaskDO
	if s.db.WithContext(ctx).Where("project_id = ? AND job_id = ? AND task_id = ?", r.ProjectID, r.JobID, r.TaskID).First(&task).Error == nil {
		refID := r.RefID
		var p model.ProjectDO
		if s.db.WithContext(ctx).Where("project_id = ?", r.ProjectID).First(&p).Error == nil && isTeeComputeMode(p.ComputeMode) {
			// TEE: a pulled result "{nodeId}-{teeResultId}" renders the TEE side output.
			_, refID = RebuildTeeResultID(r.NodeID, r.RefID, "")
		}
		outputID := strings.TrimPrefix(refID, r.JobID+"-")
		if out, err := newOutputRenderer(s.db, toGraphKuscia(s.kusciaClient)).taskOutput(ctx, &task, outputID); err == nil {
			vo.Output = out
		}
	}
	return vo, nil
}

// Java KusciaDataSourceConstants.DEFAULT_DATA_SOURCE(_PATH).
const (
	defaultDataSourceID   = "default-data-source"
	defaultDataSourcePath = "/home/kuscia/var/storage/data"
)

// datasourcePath mirrors Java DataServiceImpl.queryDataSources lookup: only
// the default data source has a known path, others yield "".
func datasourcePath(datasourceID string) string {
	if datasourceID == defaultDataSourceID {
		return defaultDataSourcePath
	}
	return ""
}
