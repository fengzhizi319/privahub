package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Graph service errors.
var (
	ErrGraphNotFound = errors.New("graph not found")
)

// DefaultGraphNodeMaxIndex mirrors GraphServiceImpl.DEFAULT_INITIAL_INDEX.
const DefaultGraphNodeMaxIndex = 32

// GraphService handles DAG graph management and execution (Java GraphServiceImpl).
type GraphService struct {
	graphRepo     repository.GraphRepository
	graphNodeRepo repository.GraphNodeRepository
	jobRepo       repository.JobRepository
	taskRepo      repository.TaskRepository
	taskLogRepo   repository.TaskLogRepository
	kuscia        GraphKusciaAPI
	db            *gorm.DB
	partitionRule *PartitionRuleService

	// Optional collaborators configured after construction via
	// SetTeeConfig / SetTeeSigner / WithKusciaPool (guarded by extMu).
	extMu  sync.RWMutex
	tee    TeeGraphConfig
	signer TeeTaskSigner
	pool   *kuscia.ClientPool
}

// NewGraphService creates a new GraphService.
func NewGraphService(
	graphRepo repository.GraphRepository,
	graphNodeRepo repository.GraphNodeRepository,
	jobRepo repository.JobRepository,
	taskRepo repository.TaskRepository,
	taskLogRepo repository.TaskLogRepository,
	kusciaClient *kuscia.Client,
	db *gorm.DB,
) *GraphService {
	return &GraphService{
		graphRepo:     graphRepo,
		graphNodeRepo: graphNodeRepo,
		jobRepo:       jobRepo,
		taskRepo:      taskRepo,
		taskLogRepo:   taskLogRepo,
		kuscia:        toGraphKuscia(kusciaClient),
		db:            db,
		partitionRule: NewPartitionRuleService(),
		tee:           TeeGraphConfig{}.withDefaults(),
	}
}

// SetKuscia overrides the Kuscia API (tests / alternative transports).
func (s *GraphService) SetKuscia(k GraphKusciaAPI) { s.kuscia = k }

func (s *GraphService) settings() *graphjob.Settings { return graphjob.GetSettings() }

// --- Request / Response DTOs (Java camelCase contract) ---

// CreateGraphRequest mirrors Java CreateGraphRequest.
type CreateGraphRequest struct {
	ProjectID  string                   `json:"projectId"`
	Name       string                   `json:"name"`
	TemplateID string                   `json:"templateId,omitempty"`
	Nodes      []graphjob.GraphNodeInfo `json:"nodes"`
	Edges      []graphjob.GraphEdge     `json:"edges"`
}

// CreateGraphVO mirrors Java CreateGraphVO.
type CreateGraphVO struct {
	GraphID string `json:"graphId"`
}

// ListGraphRequest mirrors Java ListGraphRequest.
type ListGraphRequest struct {
	ProjectID string `json:"projectId"`
}

// GraphMetaVO mirrors Java GraphMetaVO.
type GraphMetaVO struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
	Name      string `json:"name"`
	OwnerID   string `json:"ownerId"`
	GmtCreate string `json:"gmtCreate,omitempty"`
}

// GetGraphRequest mirrors Java GetGraphRequest.
type GetGraphRequest struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
}

// NodeSimpleInfo mirrors Java NodeSimpleInfo.
type NodeSimpleInfo struct {
	NodeID   string `json:"nodeId"`
	NodeName string `json:"nodeName"`
}

// ProjectResultBaseVO mirrors Java ProjectResultBaseVO.
type ProjectResultBaseVO struct {
	Kind  string `json:"kind"`
	RefID string `json:"refId"`
}

// GraphNodeDetail mirrors Java GraphNodeDetail (GraphNodeInfo + runtime info).
type GraphNodeDetail struct {
	graphjob.GraphNodeInfo
	Status   string                `json:"status"`
	JobID    string                `json:"jobId,omitempty"`
	TaskID   string                `json:"taskId,omitempty"`
	Results  []ProjectResultBaseVO `json:"results,omitempty"`
	Parties  []NodeSimpleInfo      `json:"parties,omitempty"`
	Progress *float64              `json:"progress,omitempty"`
}

// GraphDataSourceConfigVO mirrors GraphDetailVO.DataSourceConfig.
type GraphDataSourceConfigVO struct {
	EditEnable     bool   `json:"editEnable"`
	NodeID         string `json:"nodeId"`
	NodeName       string `json:"nodeName"`
	DataSourceName string `json:"dataSourceName"`
	DataSourceID   string `json:"dataSourceId"`
}

// GraphDetailVO mirrors Java GraphDetailVO.
type GraphDetailVO struct {
	ProjectID        string                    `json:"projectId"`
	GraphID          string                    `json:"graphId"`
	Name             string                    `json:"name"`
	Nodes            []GraphNodeDetail         `json:"nodes"`
	Edges            []graphjob.GraphEdge      `json:"edges"`
	MaxParallelism   int                       `json:"maxParallelism"`
	DataSourceConfig []GraphDataSourceConfigVO `json:"dataSourceConfig,omitempty"`
	// NodeMaxIndex is a Go extension kept for backward compatibility.
	NodeMaxIndex int `json:"nodeMaxIndex"`
}

// DeleteGraphRequest mirrors Java DeleteGraphRequest.
type DeleteGraphRequest struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
}

// GraphDataSourceConfig mirrors FullUpdateGraphRequest.GraphDataSourceConfig.
type GraphDataSourceConfig struct {
	NodeID       string `json:"nodeId"`
	DataSourceID string `json:"dataSourceId"`
}

// FullUpdateGraphRequest mirrors Java FullUpdateGraphRequest. Nodes / Edges
// are only replaced when present (non-nil) in the request.
type FullUpdateGraphRequest struct {
	ProjectID        string                   `json:"projectId"`
	GraphID          string                   `json:"graphId"`
	Nodes            []graphjob.GraphNodeInfo `json:"nodes"`
	Edges            []graphjob.GraphEdge     `json:"edges"`
	MaxParallelism   *int                     `json:"maxParallelism"`
	DataSourceConfig []GraphDataSourceConfig  `json:"dataSourceConfig"`
}

// UpdateGraphMetaRequest mirrors Java UpdateGraphMetaRequest.
type UpdateGraphMetaRequest struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
	Name      string `json:"name"`
}

// UpdateGraphNodeRequest mirrors Java UpdateGraphNodeRequest.
type UpdateGraphNodeRequest struct {
	ProjectID string                  `json:"projectId"`
	GraphID   string                  `json:"graphId"`
	Node      *graphjob.GraphNodeInfo `json:"node"`
}

// StartGraphRequest mirrors Java StartGraphRequest. An empty Nodes list runs
// every node of the graph (Go extension used by the scheduler).
type StartGraphRequest struct {
	ProjectID  string   `json:"projectId"`
	GraphID    string   `json:"graphId"`
	Nodes      []string `json:"nodes"`
	Breakpoint bool     `json:"breakpoint"`
}

// StartGraphVO mirrors Java StartGraphVO.
type StartGraphVO struct {
	JobID string `json:"jobId"`
}

// StopGraphRequest mirrors Java StopGraphNodeRequest (jobId is a Go extension).
type StopGraphRequest struct {
	ProjectID   string `json:"projectId"`
	GraphID     string `json:"graphId"`
	GraphNodeID string `json:"graphNodeId"`
	JobID       string `json:"jobId,omitempty"`
}

// ListGraphNodeStatusRequest mirrors Java ListGraphNodeStatusRequest.
type ListGraphNodeStatusRequest struct {
	ProjectID string `json:"projectId"`
	GraphID   string `json:"graphId"`
}

// GraphNodeStatusVO mirrors Java GraphNodeStatusVO.
type GraphNodeStatusVO struct {
	GraphNodeID string           `json:"graphNodeId"`
	TaskID      string           `json:"taskId,omitempty"`
	JobID       string           `json:"jobId,omitempty"`
	Status      string           `json:"status"`
	Progress    *float64         `json:"progress,omitempty"`
	Parties     []NodeSimpleInfo `json:"parties,omitempty"`
}

// GraphStatusVO mirrors Java GraphStatus.
type GraphStatusVO struct {
	Finished bool                `json:"finished"`
	Nodes    []GraphNodeStatusVO `json:"nodes"`
}

// GraphNodeOutputRequest mirrors Java GraphNodeOutputRequest.
type GraphNodeOutputRequest struct {
	ProjectID   string `json:"projectId"`
	GraphID     string `json:"graphId"`
	GraphNodeID string `json:"graphNodeId"`
	OutputID    string `json:"outputId"`
}

// GraphNodeLogsRequest mirrors Java GraphNodeLogsRequest.
type GraphNodeLogsRequest struct {
	ProjectID   string `json:"projectId"`
	GraphID     string `json:"graphId"`
	GraphNodeID string `json:"graphNodeId"`
}

// GraphNodeTaskLogsVO mirrors Java GraphNodeTaskLogsVO.
type GraphNodeTaskLogsVO struct {
	Status string   `json:"status"`
	Logs   []string `json:"logs"`
}

// RefreshNodeMaxIndexRequest mirrors Java GraphNodeMaxIndexRefreshRequest.
type RefreshNodeMaxIndexRequest struct {
	ProjectID    string `json:"projectId"`
	GraphID      string `json:"graphId"`
	CurrentIndex *int   `json:"currentIndex"`
}

// GraphNodeMaxIndexVO mirrors Java GraphNodeMaxIndexRefreshVO.
type GraphNodeMaxIndexVO struct {
	MaxIndex int `json:"maxIndex"`
}

// --- conversions ---

// splitOutputs parses a stored JSON string array.
func splitOutputs(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func jsonStrings(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// rawJSONMessage converts a stored JSON string to json.RawMessage; empty or
// invalid input yields null.
func rawJSONMessage(raw string) json.RawMessage {
	if raw == "" || !json.Valid([]byte(raw)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(raw)
}

// canonicalNodeDef compacts a nodeDef and drops the snake_case twins injected
// by the case-normalization middleware (attr_paths / checkpoint_uri).
func canonicalNodeDef(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		raw = json.RawMessage(s)
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil {
		if _, ok := m["attrPaths"]; ok {
			delete(m, "attr_paths")
		}
		if _, ok := m["checkpointUri"]; ok {
			delete(m, "checkpoint_uri")
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

func nodeInfoFromDO(n *model.ProjectGraphNodeDO) graphjob.GraphNodeInfo {
	info := graphjob.GraphNodeInfo{
		CodeName:    n.CodeName,
		GraphNodeID: n.GraphNodeID,
		Label:       n.Label,
		X:           n.X,
		Y:           n.Y,
		Inputs:      splitOutputs(n.Inputs),
		Outputs:     splitOutputs(n.Outputs),
		NodeDef:     rawJSONMessage(n.NodeDef),
	}
	if string(info.NodeDef) == "null" {
		info.NodeDef = json.RawMessage("{}")
	}
	if info.Inputs == nil {
		info.Inputs = []string{}
	}
	if info.Outputs == nil {
		info.Outputs = []string{}
	}
	return info
}

func nodeDOFromInfo(projectID, graphID string, n *graphjob.GraphNodeInfo) *model.ProjectGraphNodeDO {
	return &model.ProjectGraphNodeDO{
		ProjectID:   projectID,
		GraphID:     graphID,
		GraphNodeID: n.GraphNodeID,
		CodeName:    n.CodeName,
		Label:       n.Label,
		X:           n.X,
		Y:           n.Y,
		Inputs:      jsonStrings(n.Inputs),
		Outputs:     jsonStrings(n.Outputs),
		NodeDef:     canonicalNodeDef(n.NodeDef),
	}
}

// taskNodeInfo returns the task's graph node snapshot, falling back to the
// current graph node for legacy tasks created without a snapshot.
func (s *GraphService) taskNodeInfo(ctx context.Context, t *model.ProjectJobTaskDO, graphID string) graphjob.GraphNodeInfo {
	return loadTaskNodeInfo(ctx, s.db, t, graphID)
}

func loadTaskNodeInfo(ctx context.Context, db *gorm.DB, t *model.ProjectJobTaskDO, graphID string) graphjob.GraphNodeInfo {
	var info graphjob.GraphNodeInfo
	if t.GraphNode != "" && json.Unmarshal([]byte(t.GraphNode), &info) == nil && info.GraphNodeID != "" {
		return info
	}
	if db != nil {
		q := db.WithContext(ctx).Where("project_id = ? AND graph_node_id = ?", t.ProjectID, t.GraphNodeID)
		if graphID != "" {
			q = q.Where("graph_id = ?", graphID)
		}
		var n model.ProjectGraphNodeDO
		if q.First(&n).Error == nil {
			return nodeInfoFromDO(&n)
		}
	}
	return graphjob.GraphNodeInfo{GraphNodeID: t.GraphNodeID, Inputs: []string{}, Outputs: []string{}}
}

func taskProgress(t *model.ProjectJobTaskDO) *float64 {
	if t.ExtraInfo == "" {
		return nil
	}
	var ei struct {
		Progress *float64 `json:"progress"`
	}
	if json.Unmarshal([]byte(t.ExtraInfo), &ei) != nil {
		return nil
	}
	return ei.Progress
}

func taskParties(t *model.ProjectJobTaskDO) []string {
	var p []string
	_ = json.Unmarshal([]byte(t.Parties), &p)
	return p
}

// nodeSimpleInfos resolves node names (Java nodeRepository.findByNodeIdIn).
func nodeSimpleInfos(ctx context.Context, db *gorm.DB, nodeIDs []string) []NodeSimpleInfo {
	out := make([]NodeSimpleInfo, 0, len(nodeIDs))
	if len(nodeIDs) == 0 {
		return out
	}
	names := map[string]string{}
	if db != nil {
		var nodes []model.NodeDO
		db.WithContext(ctx).Where("node_id IN ?", nodeIDs).Find(&nodes)
		for _, n := range nodes {
			names[n.NodeID] = n.Name
		}
	}
	for _, id := range nodeIDs {
		name, ok := names[id]
		if !ok {
			continue
		}
		out = append(out, NodeSimpleInfo{NodeID: id, NodeName: name})
	}
	return out
}

func nodeName(ctx context.Context, db *gorm.DB, nodeID string) string {
	if db == nil {
		return nodeID
	}
	var n model.NodeDO
	if db.WithContext(ctx).Where("node_id = ?", nodeID).First(&n).Error == nil && n.Name != "" {
		return n.Name
	}
	return nodeID
}

// --- lookups ---

func (s *GraphService) loadGraph(ctx context.Context, projectID, graphID string) (*model.ProjectGraphDO, error) {
	g, err := s.graphRepo.FindByProjectAndGraphID(ctx, projectID, graphID)
	if err != nil || g == nil {
		return nil, padErrBase(ErrGraphNotFound, errcode.GraphNotExists)
	}
	return g, nil
}

// ownerCheck mirrors GraphServiceImpl.ownerCheck. Graphs created before
// ownership was tracked (empty owner) and anonymous operators are allowed.
func ownerCheck(ctx context.Context, g *model.ProjectGraphDO) error {
	op := OperatorFrom(ctx)
	if op.OwnerID == "" || g.OwnerID == "" {
		return nil
	}
	if op.OwnerID != g.OwnerID {
		return padErr(errcode.GraphNonOurCreationCanViewed)
	}
	return nil
}

func (s *GraphService) loadGraphNodes(ctx context.Context, projectID, graphID string) ([]model.ProjectGraphNodeDO, error) {
	return s.graphNodeRepo.FindByGraphID(ctx, projectID, graphID)
}

// latestTask mirrors ProjectJobTaskRepository.findLatestTasks (joins project_job).
func latestTask(ctx context.Context, db *gorm.DB, projectID, graphNodeID string) (*model.ProjectJobTaskDO, error) {
	var t model.ProjectJobTaskDO
	err := db.WithContext(ctx).Model(&model.ProjectJobTaskDO{}).
		Select("project_job_task.*").
		Joins("JOIN project_job ON project_job.job_id = project_job_task.job_id AND project_job.deleted_at IS NULL").
		Where("project_job_task.project_id = ? AND project_job_task.graph_node_id = ?", projectID, graphNodeID).
		Order("project_job_task.id DESC").
		Limit(1).
		Take(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// lastTwoTasks mirrors findLastTimeTasks (no join).
func lastTwoTasks(ctx context.Context, db *gorm.DB, projectID, graphNodeID string) []model.ProjectJobTaskDO {
	var ts []model.ProjectJobTaskDO
	db.WithContext(ctx).Where("project_id = ? AND graph_node_id = ?", projectID, graphNodeID).
		Order("id DESC").Limit(2).Find(&ts)
	return ts
}

// --- Graph CRUD ---

// CreateGraph mirrors GraphServiceImpl.createGraph.
func (s *GraphService) CreateGraph(ctx context.Context, req *CreateGraphRequest) (*CreateGraphVO, error) {
	if req.ProjectID == "" {
		return nil, padErr(errcode.ProjectNotExists)
	}
	var cnt int64
	s.db.WithContext(ctx).Model(&model.ProjectDO{}).Where("project_id = ?", req.ProjectID).Count(&cnt)
	if cnt == 0 {
		return nil, padErrBase(ErrProjectNotFound, errcode.ProjectNotExists)
	}
	graphID := graphjob.RandomLetters(8)
	g := &model.ProjectGraphDO{
		ProjectID:      req.ProjectID,
		GraphID:        graphID,
		Name:           req.Name,
		Edges:          graphjob.EdgesJSON(req.Edges),
		OwnerID:        OperatorFrom(ctx).OwnerID,
		NodeMaxIndex:   DefaultGraphNodeMaxIndex,
		MaxParallelism: 1,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(g).Error; err != nil {
			return err
		}
		for i := range req.Nodes {
			if err := tx.Create(nodeDOFromInfo(req.ProjectID, graphID, &req.Nodes[i])).Error; err != nil {
				return err
			}
		}
		return s.initDefaultDataSources(ctx, tx, req.ProjectID, graphID)
	})
	if err != nil {
		return nil, err
	}
	return &CreateGraphVO{GraphID: graphID}, nil
}

// ListGraph mirrors GraphServiceImpl.listGraph.
func (s *GraphService) ListGraph(ctx context.Context, req *ListGraphRequest) ([]GraphMetaVO, error) {
	graphs, err := s.graphRepo.FindByProjectID(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}
	result := make([]GraphMetaVO, 0, len(graphs))
	for _, g := range graphs {
		result = append(result, GraphMetaVO{
			ProjectID: g.ProjectID,
			GraphID:   g.GraphID,
			Name:      g.Name,
			OwnerID:   g.OwnerID,
			GmtCreate: rfc3339(g.GmtCreate),
		})
	}
	return result, nil
}

// GetGraphDetail mirrors GraphServiceImpl.getGraphDetail.
func (s *GraphService) GetGraphDetail(ctx context.Context, req *GetGraphRequest) (*GraphDetailVO, error) {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}
	nodes, err := s.loadGraphNodes(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}
	status, err := s.latestTaskStatus(ctx, req.ProjectID, nodes)
	if err != nil {
		return nil, err
	}
	statusByNode := make(map[string]GraphNodeStatusVO, len(status.Nodes))
	for _, st := range status.Nodes {
		statusByNode[st.GraphNodeID] = st
	}
	details := make([]GraphNodeDetail, 0, len(nodes))
	for i := range nodes {
		st := statusByNode[nodes[i].GraphNodeID]
		if st.Status == "" {
			st.Status = graphjob.TaskStaging
		}
		details = append(details, GraphNodeDetail{
			GraphNodeInfo: nodeInfoFromDO(&nodes[i]),
			Status:        st.Status,
			JobID:         st.JobID,
			TaskID:        st.TaskID,
			Parties:       st.Parties,
			Progress:      st.Progress,
		})
	}
	edges := graphjob.ParseEdges(g.Edges)
	if edges == nil {
		edges = []graphjob.GraphEdge{}
	}
	return &GraphDetailVO{
		ProjectID:        g.ProjectID,
		GraphID:          g.GraphID,
		Name:             g.Name,
		Nodes:            details,
		Edges:            edges,
		MaxParallelism:   g.MaxParallelism,
		DataSourceConfig: s.graphDataSourceConfig(ctx, g.ProjectID, g.GraphID),
		NodeMaxIndex:     g.NodeMaxIndex,
	}, nil
}

// DeleteGraph mirrors GraphServiceImpl.deleteGraph (owner check + cascade).
func (s *GraphService) DeleteGraph(ctx context.Context, req *DeleteGraphRequest) error {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return err
	}
	if err := ownerCheck(ctx, g); err != nil {
		return err
	}
	if s.db == nil {
		if err := s.graphNodeRepo.DeleteByGraphID(ctx, req.ProjectID, req.GraphID); err != nil {
			return err
		}
		return s.graphRepo.Delete(ctx, g.ID)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		where := "project_id = ? AND graph_id = ?"
		if err := tx.Unscoped().Where(where, req.ProjectID, req.GraphID).Delete(&model.ProjectGraphNodeDO{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where(where, req.ProjectID, req.GraphID).Delete(&model.ProjectGraphDomainDatasourceDO{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where(where, req.ProjectID, req.GraphID).Delete(&model.ProjectGraphNodeKusciaParamsDO{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&model.ProjectGraphDO{}, g.ID).Error
	})
}

// FullUpdateGraph mirrors GraphServiceImpl.fullUpdateGraph: nodes / edges are
// replaced only when present, maxParallelism and the datasource config are
// persisted when supplied.
func (s *GraphService) FullUpdateGraph(ctx context.Context, req *FullUpdateGraphRequest) error {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return err
	}
	if err := ownerCheck(ctx, g); err != nil {
		return err
	}
	if req.Edges != nil {
		g.Edges = graphjob.EdgesJSON(req.Edges)
	}
	if req.MaxParallelism != nil && *req.MaxParallelism > 0 {
		g.MaxParallelism = *req.MaxParallelism
	}
	// Validate the datasource config before writing anything.
	dsUpdates, err := s.validateDataSourceConfig(ctx, req.ProjectID, req.GraphID, req.DataSourceConfig)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.Nodes != nil {
			if err := tx.Unscoped().Where("project_id = ? AND graph_id = ?", req.ProjectID, req.GraphID).
				Delete(&model.ProjectGraphNodeDO{}).Error; err != nil {
				return err
			}
			for i := range req.Nodes {
				if req.Nodes[i].GraphNodeID == "" {
					continue
				}
				if err := tx.Create(nodeDOFromInfo(req.ProjectID, req.GraphID, &req.Nodes[i])).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Save(g).Error; err != nil {
			return err
		}
		for i := range dsUpdates {
			if err := saveGraphDatasource(tx, &dsUpdates[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateGraphMeta mirrors GraphServiceImpl.updateGraphMeta.
func (s *GraphService) UpdateGraphMeta(ctx context.Context, req *UpdateGraphMetaRequest) error {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return err
	}
	op := OperatorFrom(ctx)
	if strings.EqualFold(op.OwnerType, "P2P") && g.OwnerID != "" && !strings.EqualFold(op.OwnerID, g.OwnerID) {
		return padErr(errcode.GraphNotOwnerCannotUpdate)
	}
	g.Name = req.Name
	return s.graphRepo.Update(ctx, g)
}

// UpdateGraphNode mirrors GraphServiceImpl.updateGraphNode (whole node replace).
func (s *GraphService) UpdateGraphNode(ctx context.Context, req *UpdateGraphNodeRequest) error {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return err
	}
	if err := ownerCheck(ctx, g); err != nil {
		return err
	}
	if req.Node == nil || req.Node.GraphNodeID == "" {
		return padErr(errcode.GraphNodeNotExists)
	}
	existing, err := s.graphNodeRepo.FindByGraphNodeID(ctx, req.ProjectID, req.GraphID, req.Node.GraphNodeID)
	if err != nil || existing == nil {
		return padErr(errcode.GraphNodeNotExists, req.Node.GraphNodeID)
	}
	n := nodeDOFromInfo(req.ProjectID, req.GraphID, req.Node)
	n.BaseDO = existing.BaseDO
	return s.graphNodeRepo.Update(ctx, n)
}

// RefreshNodeMaxIndex mirrors GraphServiceImpl.refreshNodeMaxIndex.
func (s *GraphService) RefreshNodeMaxIndex(ctx context.Context, req *RefreshNodeMaxIndexRequest) (*GraphNodeMaxIndexVO, error) {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}
	index := g.NodeMaxIndex
	if req.CurrentIndex != nil && *req.CurrentIndex > index {
		g.NodeMaxIndex = *req.CurrentIndex + 1
		index = *req.CurrentIndex
	} else {
		g.NodeMaxIndex = index + 1
	}
	if err := s.graphRepo.Update(ctx, g); err != nil {
		return nil, err
	}
	return &GraphNodeMaxIndexVO{MaxIndex: index}, nil
}

// --- status / logs / stop / output ---

// ListNodeStatus mirrors GraphServiceImpl.listGraphNodeStatus.
func (s *GraphService) ListNodeStatus(ctx context.Context, req *ListGraphNodeStatusRequest) (*GraphStatusVO, error) {
	if _, err := s.loadGraph(ctx, req.ProjectID, req.GraphID); err != nil {
		return nil, err
	}
	nodes, err := s.loadGraphNodes(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}
	return s.latestTaskStatus(ctx, req.ProjectID, nodes)
}

// latestTaskStatus mirrors GraphServiceImpl.getLatestTaskStatus.
func (s *GraphService) latestTaskStatus(ctx context.Context, projectID string, nodes []model.ProjectGraphNodeDO) (*GraphStatusVO, error) {
	vo := &GraphStatusVO{Finished: true, Nodes: make([]GraphNodeStatusVO, 0, len(nodes))}
	jobIDs := map[string]bool{}
	for _, n := range nodes {
		st := GraphNodeStatusVO{GraphNodeID: n.GraphNodeID, Status: graphjob.TaskStaging}
		t, err := latestTask(ctx, s.db, projectID, n.GraphNodeID)
		if err != nil {
			return nil, err
		}
		if t != nil {
			st.Status = graphjob.NormalizeTaskStatus(t.Status)
			st.TaskID = t.TaskID
			st.JobID = t.JobID
			st.Parties = nodeSimpleInfos(ctx, s.db, taskParties(t))
			st.Progress = taskProgress(t)
			jobIDs[t.JobID] = true
		}
		vo.Nodes = append(vo.Nodes, st)
	}
	if len(jobIDs) > 0 {
		ids := make([]string, 0, len(jobIDs))
		for id := range jobIDs {
			ids = append(ids, id)
		}
		var jobs []model.ProjectJobDO
		if err := s.db.WithContext(ctx).Where("project_id = ? AND job_id IN ?", projectID, ids).Find(&jobs).Error; err != nil {
			return nil, err
		}
		for _, j := range jobs {
			if !graphjob.IsJobFinished(j.Status) {
				vo.Finished = false
				break
			}
		}
	}
	return vo, nil
}

// GetNodeLogs mirrors GraphServiceImpl.getGraphNodeLogs.
func (s *GraphService) GetNodeLogs(ctx context.Context, req *GraphNodeLogsRequest) (*GraphNodeTaskLogsVO, error) {
	t, err := latestTask(ctx, s.db, req.ProjectID, req.GraphNodeID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, padErr(errcode.ProjectJobTaskNotExists)
	}
	logs, err := taskLogLines(ctx, s.db, t.JobID, t.TaskID)
	if err != nil {
		return nil, err
	}
	logs = graphjob.Dedupe(logs)
	if len(logs) == 0 {
		info := s.taskNodeInfo(ctx, t, "")
		if info.CodeName == graphjob.CompReadDataDatatable {
			logs = []string{
				graphjob.TaskStartLog(t.GmtCreate, t.JobID, t.TaskID),
				graphjob.TaskSucceedLog(t.GmtCreate, t.JobID, t.TaskID),
			}
		}
	}
	prefix := "INFO the jobId=" + t.JobID + ", taskId=" + t.JobID + "-" + req.GraphNodeID
	logs = graphjob.DistinctSpecifyLogs(logs, prefix+" start")
	logs = graphjob.DistinctSpecifyLogs(logs, prefix+" succeed")
	return &GraphNodeTaskLogsVO{Status: graphjob.NormalizeTaskStatus(t.Status), Logs: logs}, nil
}

// taskLogLines loads task log lines in insertion order (Java findAllByJobTaskId).
func taskLogLines(ctx context.Context, db *gorm.DB, jobID, taskID string) ([]string, error) {
	var rows []model.ProjectJobTaskLogDO
	if err := db.WithContext(ctx).Where("job_id = ? AND task_id = ?", jobID, taskID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Log)
	}
	return out, nil
}

// StopGraph mirrors GraphServiceImpl.stopGraphNode: without graphNodeId every
// RUNNING job of the graph is stopped, otherwise the jobs running the node.
func (s *GraphService) StopGraph(ctx context.Context, req *StopGraphRequest) error {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		if req.JobID == "" {
			return err
		}
		// Legacy clients stop by job id only.
		return stopProjectJob(ctx, s.db, s.kuscia, req.ProjectID, req.JobID, false)
	}
	if err := ownerCheck(ctx, g); err != nil {
		return err
	}
	jobIDs := []string{}
	switch {
	case req.JobID != "":
		jobIDs = append(jobIDs, req.JobID)
	case req.GraphNodeID == "":
		var jobs []model.ProjectJobDO
		if err := s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ? AND status = ?", req.ProjectID, req.GraphID, graphjob.JobRunning).
			Find(&jobs).Error; err != nil {
			return err
		}
		for _, j := range jobs {
			jobIDs = append(jobIDs, j.JobID)
		}
	default:
		var tasks []model.ProjectJobTaskDO
		if err := s.db.WithContext(ctx).Where("project_id = ? AND graph_node_id = ? AND status = ?", req.ProjectID, req.GraphNodeID, graphjob.TaskRunning).
			Find(&tasks).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, t := range tasks {
			if !seen[t.JobID] {
				seen[t.JobID] = true
				jobIDs = append(jobIDs, t.JobID)
			}
		}
	}
	for _, id := range jobIDs {
		if err := stopProjectJob(ctx, s.db, s.kuscia, req.ProjectID, id, true); err != nil {
			return err
		}
	}
	return nil
}

// GetNodeOutput mirrors GraphServiceImpl.getGraphNodeOutput.
func (s *GraphService) GetNodeOutput(ctx context.Context, req *GraphNodeOutputRequest) (*GraphNodeOutputVO, error) {
	t, err := latestTask(ctx, s.db, req.ProjectID, req.GraphNodeID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, padErr(errcode.GraphNodeOutputNotExists)
	}
	r := newOutputRenderer(s.db, s.kuscia)
	r.channel = s.kusciaFor
	return r.taskOutput(ctx, t, req.OutputID)
}

// --- graph datasource bindings (Java ProjectGraphDomainDatasourceServiceImpl) ---

// controlNodeIDs mirrors getControlNodeIds: the project nodes the operator
// may configure (CENTER / EDGE: every project node; P2P: its own node).
func controlNodeIDs(ctx context.Context, db *gorm.DB, projectID string) []string {
	var pns []model.ProjectNodeDO
	db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").Find(&pns)
	nodes := make([]string, 0, len(pns))
	for _, pn := range pns {
		nodes = append(nodes, pn.NodeID)
	}
	op := OperatorFrom(ctx)
	switch strings.ToUpper(op.OwnerType) {
	case "P2P", "AUTONOMY":
		for _, n := range nodes {
			if n == op.OwnerID {
				return []string{n}
			}
		}
	}
	return nodes
}

func boolPtr(b bool) *bool { return &b }

func (s *GraphService) initDefaultDataSources(ctx context.Context, tx *gorm.DB, projectID, graphID string) error {
	for _, nodeID := range controlNodeIDs(ctx, tx, projectID) {
		if err := saveGraphDatasource(tx, &model.ProjectGraphDomainDatasourceDO{
			ProjectID:      projectID,
			GraphID:        graphID,
			DomainID:       nodeID,
			DatasourceID:   graphjob.DefaultDataSourceID,
			DatasourceName: graphjob.DefaultDataSourceID,
			EditEnable:     boolPtr(true),
		}); err != nil {
			return err
		}
	}
	return nil
}

func saveGraphDatasource(tx *gorm.DB, d *model.ProjectGraphDomainDatasourceDO) error {
	var existing model.ProjectGraphDomainDatasourceDO
	err := tx.Where("project_id = ? AND graph_id = ? AND domain_id = ?", d.ProjectID, d.GraphID, d.DomainID).First(&existing).Error
	if err == nil {
		return tx.Model(&existing).Updates(map[string]any{
			"datasource_id":   d.DatasourceID,
			"datasource_name": d.DatasourceName,
			"edit_enable":     d.EditEnable,
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(d).Error
}

// ProjectDataSourceItem mirrors ProjectGraphDomainDataSourceVO.DataSource.
type ProjectDataSourceItem struct {
	NodeID         string `json:"nodeId"`
	Type           string `json:"type"`
	DataSourceID   string `json:"dataSourceId"`
	DataSourceName string `json:"dataSourceName"`
}

// domainDataSources mirrors getDomainDataSourcesByProjectAndComputeMode: the
// Kuscia datasources of a node (TEE projects: LOCAL only).
func domainDataSources(ctx context.Context, db *gorm.DB, k GraphKusciaAPI, projectID, nodeID string) ([]ProjectDataSourceItem, error) {
	out := []ProjectDataSourceItem{}
	if k == nil {
		return out, errors.New("kuscia client unavailable")
	}
	list, err := k.ListDomainDataSource(ctx, nodeID)
	if err != nil {
		return out, err
	}
	tee := false
	var p model.ProjectDO
	if db.WithContext(ctx).Where("project_id = ?", projectID).First(&p).Error == nil {
		tee = strings.EqualFold(p.ComputeMode, "TEE")
	}
	for _, ds := range list {
		t := kusciaDatasourceTypeToPlatform(ds.Type)
		if tee && t != "LOCAL" {
			continue
		}
		out = append(out, ProjectDataSourceItem{NodeID: nodeID, Type: t, DataSourceID: ds.DatasourceID, DataSourceName: ds.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DataSourceID < out[j].DataSourceID })
	return out, nil
}

// validateDataSourceConfig mirrors updateProjectGraphDomainDatasourceDOByFullUpdateGraphRequest.
func (s *GraphService) validateDataSourceConfig(ctx context.Context, projectID, graphID string, cfgs []GraphDataSourceConfig) ([]model.ProjectGraphDomainDatasourceDO, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}
	control := map[string]bool{}
	for _, n := range controlNodeIDs(ctx, s.db, projectID) {
		control[n] = true
	}
	out := make([]model.ProjectGraphDomainDatasourceDO, 0, len(cfgs))
	for _, c := range cfgs {
		if !control[c.NodeID] {
			return nil, padErr(errcode.SystemValidationError, "unauthorized access to node "+c.NodeID+", by project "+projectID)
		}
		var existing model.ProjectGraphDomainDatasourceDO
		err := s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ? AND domain_id = ?", projectID, graphID, c.NodeID).First(&existing).Error
		if err != nil {
			return nil, padErr(errcode.SystemValidationError, "current node dataSources is empty")
		}
		if existing.EditEnable != nil && !*existing.EditEnable {
			if existing.DatasourceID == c.DataSourceID {
				continue // unchanged, locked binding
			}
			return nil, padErr(errcode.SystemValidationError, "saved node dataSources are not editable "+existing.DatasourceID)
		}
		name := c.DataSourceID
		if s.kuscia != nil {
			dss, err := domainDataSources(ctx, s.db, s.kuscia, projectID, c.NodeID)
			if err != nil {
				return nil, padErr(errcode.SystemValidationError, "failed to query domain data sources "+err.Error())
			}
			found := false
			for _, ds := range dss {
				if ds.DataSourceID == c.DataSourceID {
					found, name = true, ds.DataSourceName
					break
				}
			}
			if !found {
				return nil, padErr(errcode.SystemValidationError, "can't find data source "+c.DataSourceID)
			}
		}
		out = append(out, model.ProjectGraphDomainDatasourceDO{
			ProjectID: projectID, GraphID: graphID, DomainID: c.NodeID,
			DatasourceID: c.DataSourceID, DatasourceName: name, EditEnable: boolPtr(false),
		})
	}
	return out, nil
}

// graphDataSourceConfig mirrors convertToGraphDetailVODataSourceConfig.
func (s *GraphService) graphDataSourceConfig(ctx context.Context, projectID, graphID string) []GraphDataSourceConfigVO {
	control := map[string]bool{}
	for _, n := range controlNodeIDs(ctx, s.db, projectID) {
		control[n] = true
	}
	var rows []model.ProjectGraphDomainDatasourceDO
	s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", projectID, graphID).Order("id ASC").Find(&rows)
	if len(rows) == 0 {
		return nil
	}
	out := make([]GraphDataSourceConfigVO, 0, len(rows))
	for _, r := range rows {
		if !control[r.DomainID] {
			continue
		}
		edit := r.EditEnable == nil || *r.EditEnable
		name := r.DatasourceName
		if name == "" {
			name = r.DatasourceID
		}
		out = append(out, GraphDataSourceConfigVO{
			EditEnable: edit, NodeID: r.DomainID, NodeName: nodeName(ctx, s.db, r.DomainID),
			DataSourceName: name, DataSourceID: r.DatasourceID,
		})
	}
	return out
}

// graphDatasourceFor returns the datasource bound to (project, graph, party)
// or the default datasource (KusciaJobConverter.buildDatasourceConfig).
func graphDatasourceFor(ctx context.Context, db *gorm.DB, projectID, graphID, party string) string {
	var d model.ProjectGraphDomainDatasourceDO
	if projectID != "" && graphID != "" &&
		db.WithContext(ctx).Where("project_id = ? AND graph_id = ? AND domain_id = ?", projectID, graphID, party).First(&d).Error == nil &&
		d.DatasourceID != "" {
		return d.DatasourceID
	}
	return graphjob.DefaultDataSourceID
}

// timeNow is overridable in tests.
var timeNow = time.Now
