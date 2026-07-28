package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Graph service errors.
var (
	ErrGraphNotFound = errors.New("graph not found")
)

// GraphService handles DAG graph management.
type GraphService struct {
	graphRepo     repository.GraphRepository
	graphNodeRepo repository.GraphNodeRepository
	jobRepo       repository.JobRepository
	taskRepo      repository.TaskRepository
	taskLogRepo   repository.TaskLogRepository
	kusciaClient  *kuscia.Client
}

// NewGraphService creates a new GraphService.
func NewGraphService(
	graphRepo repository.GraphRepository,
	graphNodeRepo repository.GraphNodeRepository,
	jobRepo repository.JobRepository,
	taskRepo repository.TaskRepository,
	taskLogRepo repository.TaskLogRepository,
	kusciaClient *kuscia.Client,
) *GraphService {
	return &GraphService{
		graphRepo:     graphRepo,
		graphNodeRepo: graphNodeRepo,
		jobRepo:       jobRepo,
		taskRepo:      taskRepo,
		taskLogRepo:   taskLogRepo,
		kusciaClient:  kusciaClient,
	}
}

// --- Request / Response DTOs ---

// CreateGraphRequest represents a graph creation request.
type CreateGraphRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	Name         string `json:"name"`
}

// CreateGraphVO is the response for graph creation.
type CreateGraphVO struct {
	GraphID   string `json:"graph_id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

// ListGraphRequest represents a graph list request.
type ListGraphRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
}

// GraphMetaVO is a graph summary.
type GraphMetaVO struct {
	GraphID   string `json:"graph_id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	GmtCreate string `json:"gmt_create"`
}

// GetGraphRequest represents a graph detail request.
type GetGraphRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
}

// GraphNodeVO represents a graph node in detail response.
type GraphNodeVO struct {
	GraphNodeID string `json:"graph_node_id"`
	CodeName    string `json:"code_name"`
	Label       string `json:"label"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Inputs      string `json:"inputs"`
	Outputs     string `json:"outputs"`
	NodeDef     string `json:"node_def"`
}

// GraphDetailVO is the full graph detail.
type GraphDetailVO struct {
	GraphID        string          `json:"graph_id"`
	ProjectID      string          `json:"project_id"`
	Name           string          `json:"name"`
	Edges          json.RawMessage `json:"edges"`
	Nodes          []GraphNodeVO   `json:"nodes"`
	NodeMaxIndex   int             `json:"node_max_index"`
	MaxParallelism int             `json:"max_parallelism"`
}

// DeleteGraphRequest represents a graph deletion request.
type DeleteGraphRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
}

// FullUpdateGraphRequest represents a full graph update (nodes + edges).
type FullUpdateGraphRequest struct {
	ProjectID    string          `json:"project_id"`
	ProjectIDAlt string          `json:"projectId"`
	GraphID      string          `json:"graph_id"`
	GraphIDAlt   string          `json:"graphId"`
	Edges        json.RawMessage `json:"edges"`
	Nodes        []GraphNodeReq  `json:"nodes"`
}

// GraphNodeReq represents a node in a full update request.
type GraphNodeReq struct {
	GraphNodeID    string `json:"graph_node_id"`
	GraphNodeIDAlt string `json:"graphNodeId"`
	CodeName       string `json:"code_name"`
	Label          string `json:"label"`
	X              int    `json:"x"`
	Y              int    `json:"y"`
	Inputs         string `json:"inputs"`
	Outputs        string `json:"outputs"`
	NodeDef        string `json:"node_def"`
}

// UpdateGraphMetaRequest updates graph name only.
type UpdateGraphMetaRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
	Name         string `json:"name"`
}

// UpdateGraphNodeRequest updates a single graph node.
type UpdateGraphNodeRequest struct {
	ProjectID      string `json:"project_id"`
	ProjectIDAlt   string `json:"projectId"`
	GraphID        string `json:"graph_id"`
	GraphIDAlt     string `json:"graphId"`
	GraphNodeID    string `json:"graph_node_id"`
	GraphNodeIDAlt string `json:"graphNodeId"`
	NodeDef        string `json:"node_def"`
}

// --- Service Methods ---

// CreateGraph creates a new DAG graph.
func (s *GraphService) CreateGraph(ctx context.Context, req *CreateGraphRequest) (*CreateGraphVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	graphID := uuid.New().String()[:8]

	graph := &model.ProjectGraphDO{
		ProjectID:      req.ProjectID,
		GraphID:        graphID,
		Name:           req.Name,
		Edges:          "[]",
		NodeMaxIndex:   0,
		MaxParallelism: 1,
	}

	if err := s.graphRepo.Create(ctx, graph); err != nil {
		return nil, err
	}

	return &CreateGraphVO{
		GraphID:   graphID,
		ProjectID: req.ProjectID,
		Name:      req.Name,
	}, nil
}

// ListGraph lists all graphs for a project.
func (s *GraphService) ListGraph(ctx context.Context, req *ListGraphRequest) ([]GraphMetaVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	graphs, err := s.graphRepo.FindByProjectID(ctx, req.ProjectID)
	if err != nil {
		return nil, err
	}

	result := make([]GraphMetaVO, 0, len(graphs))
	for _, g := range graphs {
		result = append(result, GraphMetaVO{
			GraphID:   g.GraphID,
			ProjectID: g.ProjectID,
			Name:      g.Name,
			GmtCreate: g.GmtCreate.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// GetGraphDetail retrieves full graph detail including nodes.
func (s *GraphService) GetGraphDetail(ctx context.Context, req *GetGraphRequest) (*GraphDetailVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, ErrGraphNotFound
	}

	nodes, err := s.graphNodeRepo.FindByGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}

	nodeVOs := make([]GraphNodeVO, 0, len(nodes))
	for _, n := range nodes {
		nodeVOs = append(nodeVOs, GraphNodeVO{
			GraphNodeID: n.GraphNodeID,
			CodeName:    n.CodeName,
			Label:       n.Label,
			X:           n.X,
			Y:           n.Y,
			Inputs:      n.Inputs,
			Outputs:     n.Outputs,
			NodeDef:     n.NodeDef,
		})
	}

	return &GraphDetailVO{
		GraphID:        graph.GraphID,
		ProjectID:      graph.ProjectID,
		Name:           graph.Name,
		Edges:          json.RawMessage(graph.Edges),
		Nodes:          nodeVOs,
		NodeMaxIndex:   graph.NodeMaxIndex,
		MaxParallelism: graph.MaxParallelism,
	}, nil
}

// DeleteGraph deletes a graph and all its nodes.
func (s *GraphService) DeleteGraph(ctx context.Context, req *DeleteGraphRequest) error {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return ErrGraphNotFound
	}

	// Delete all nodes first
	if err := s.graphNodeRepo.DeleteByGraphID(ctx, req.ProjectID, req.GraphID); err != nil {
		return err
	}

	return s.graphRepo.Delete(ctx, graph.ID)
}

// FullUpdateGraph replaces all nodes and edges in a graph.
func (s *GraphService) FullUpdateGraph(ctx context.Context, req *FullUpdateGraphRequest) error {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return ErrGraphNotFound
	}

	// Update edges
	graph.Edges = string(req.Edges)
	if len(req.Nodes) > graph.NodeMaxIndex {
		graph.NodeMaxIndex = len(req.Nodes)
	}
	if err := s.graphRepo.Update(ctx, graph); err != nil {
		return err
	}

	// Delete old nodes and insert new ones
	if err := s.graphNodeRepo.DeleteByGraphID(ctx, req.ProjectID, req.GraphID); err != nil {
		return err
	}

	for _, n := range req.Nodes {
		node := &model.ProjectGraphNodeDO{
			ProjectID:   req.ProjectID,
			GraphID:     req.GraphID,
			GraphNodeID: n.GraphNodeID,
			CodeName:    n.CodeName,
			Label:       n.Label,
			X:           n.X,
			Y:           n.Y,
			Inputs:      n.Inputs,
			Outputs:     n.Outputs,
			NodeDef:     n.NodeDef,
		}
		if err := s.graphNodeRepo.Create(ctx, node); err != nil {
			return err
		}
	}

	return nil
}

// UpdateGraphMeta updates graph name.
func (s *GraphService) UpdateGraphMeta(ctx context.Context, req *UpdateGraphMetaRequest) error {
	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return ErrGraphNotFound
	}

	graph.Name = req.Name
	return s.graphRepo.Update(ctx, graph)
}

// UpdateGraphNode updates a single graph node's definition.
func (s *GraphService) UpdateGraphNode(ctx context.Context, req *UpdateGraphNodeRequest) error {
	node, err := s.graphNodeRepo.FindByGraphNodeID(ctx, req.ProjectID, req.GraphID, req.GraphNodeID)
	if err != nil {
		return ErrGraphNotFound
	}

	if req.NodeDef != "" {
		node.NodeDef = req.NodeDef
	}
	return s.graphNodeRepo.Update(ctx, node)
}

// --- Graph Execution DTOs ---

// StartGraphRequest represents a graph start request.
type StartGraphRequest struct {
	ProjectID    string   `json:"project_id"`
	ProjectIDAlt string   `json:"projectId"`
	GraphID      string   `json:"graph_id"`
	GraphIDAlt   string   `json:"graphId"`
	Nodes        []string `json:"nodes"` // optional: specific nodes to run
}

// StartGraphVO is the response for graph start.
type StartGraphVO struct {
	JobID   string `json:"job_id"`
	GraphID string `json:"graph_id"`
	Status  string `json:"status"`
}

// StopGraphRequest represents a graph stop request.
type StopGraphRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
	JobID        string `json:"job_id"`
	JobIDAlt     string `json:"jobId"`
	TaskID       string `json:"task_id"`
	TaskIDAlt    string `json:"taskId"`
}

// ListGraphNodeStatusRequest represents a node status request.
type ListGraphNodeStatusRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
	JobID        string `json:"job_id"`
	JobIDAlt     string `json:"jobId"`
}

// GraphNodeStatusVO represents a single node's status.
type GraphNodeStatusVO struct {
	GraphNodeID string `json:"graph_node_id"`
	Status      string `json:"status"`
	ErrMsg      string `json:"err_msg,omitempty"`
}

// GraphStatusVO represents the overall graph status.
type GraphStatusVO struct {
	JobID  string              `json:"job_id,omitempty"`
	Status string              `json:"status"`
	Nodes  []GraphNodeStatusVO `json:"nodes"`
}

// GraphNodeOutputRequest represents a node output request.
type GraphNodeOutputRequest struct {
	ProjectID      string `json:"project_id"`
	ProjectIDAlt   string `json:"projectId"`
	GraphID        string `json:"graph_id"`
	GraphIDAlt     string `json:"graphId"`
	GraphNodeID    string `json:"graph_node_id"`
	GraphNodeIDAlt string `json:"graphNodeId"`
	JobID          string `json:"job_id"`
	JobIDAlt       string `json:"jobId"`
}

// GraphNodeOutputVO represents a node's output.
type GraphNodeOutputVO struct {
	GraphNodeID string       `json:"graph_node_id"`
	Outputs     []NodeOutput `json:"outputs"`
}

// NodeOutput represents a single output of a node.
type NodeOutput struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

// GraphNodeLogsRequest represents a node logs request.
type GraphNodeLogsRequest struct {
	ProjectID      string `json:"project_id"`
	ProjectIDAlt   string `json:"projectId"`
	GraphID        string `json:"graph_id"`
	GraphIDAlt     string `json:"graphId"`
	GraphNodeID    string `json:"graph_node_id"`
	GraphNodeIDAlt string `json:"graphNodeId"`
	JobID          string `json:"job_id"`
	JobIDAlt       string `json:"jobId"`
	TaskID         string `json:"task_id"`
	TaskIDAlt      string `json:"taskId"`
}

// GraphNodeLogsVO represents a node's task logs.
type GraphNodeLogsVO struct {
	GraphNodeID string   `json:"graph_node_id"`
	TaskID      string   `json:"task_id,omitempty"`
	Logs        []string `json:"logs"`
}

// RefreshNodeMaxIndexRequest represents a max index refresh request.
type RefreshNodeMaxIndexRequest struct {
	ProjectID    string `json:"project_id"`
	ProjectIDAlt string `json:"projectId"`
	GraphID      string `json:"graph_id"`
	GraphIDAlt   string `json:"graphId"`
}

// --- Graph Execution Methods ---

// StartGraph starts graph execution by creating a job.
func (s *GraphService) StartGraph(ctx context.Context, req *StartGraphRequest) (*StartGraphVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, ErrGraphNotFound
	}

	// Create a job for this graph execution
	jobID := uuid.New().String()[:8]
	job := &model.ProjectJobDO{
		ProjectID: req.ProjectID,
		JobID:     jobID,
		Name:      graph.Name + "_run",
		Status:    "RUNNING",
		GraphID:   req.GraphID,
		Edges:     graph.Edges,
	}
	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	// Create tasks for each graph node
	nodes, _ := s.graphNodeRepo.FindByGraphID(ctx, req.ProjectID, req.GraphID)
	var kusciaTasks []kuscia.TaskConfig
	for _, n := range nodes {
		if len(req.Nodes) > 0 && !containsStr(req.Nodes, n.GraphNodeID) {
			continue
		}
		taskID := uuid.New().String()[:8]
		task := &model.ProjectJobTaskDO{
			ProjectID:   req.ProjectID,
			JobID:       jobID,
			TaskID:      taskID,
			GraphNodeID: n.GraphNodeID,
			Status:      "PENDING",
			Parties:     "[\"alice\",\"bob\"]",
		}
		_ = s.taskRepo.Create(ctx, task)

		// Build Kuscia task config
		kusciaTasks = append(kusciaTasks, kuscia.TaskConfig{
			AppImage: "secretflow",
			Alias:    n.GraphNodeID,
			TaskID:   taskID,
			Parties: []kuscia.Party{
				{Name: "alice", Role: "guest"},
				{Name: "bob", Role: "host"},
			},
		})
	}

	// Submit to Kuscia if client is available
	if s.kusciaClient != nil && len(kusciaTasks) > 0 {
		kusciaReq := &kuscia.CreateJobRequest{
			JobID:          jobID,
			Initiator:      "alice",
			MaxParallelism: 1,
			Tasks:          kusciaTasks,
			CustomFields: map[string]string{
				"project_id": req.ProjectID,
				"graph_id":   req.GraphID,
			},
		}
		if _, err := s.kusciaClient.CreateJob(ctx, kusciaReq); err != nil {
			// Kuscia unreachable — keep job in RUNNING, log warning
			_ = s.jobRepo.UpdateStatus(ctx, jobID, "RUNNING", "kuscia unreachable: "+err.Error())
		}
	}

	return &StartGraphVO{
		JobID:   jobID,
		GraphID: req.GraphID,
		Status:  "RUNNING",
	}, nil
}

// StopGraph stops a running graph execution.
func (s *GraphService) StopGraph(ctx context.Context, req *StopGraphRequest) error {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	if req.TaskID == "" {
		req.TaskID = req.TaskIDAlt
	}
	if req.JobID == "" {
		return ErrGraphNotFound
	}

	job, err := s.jobRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		return ErrJobNotFound
	}

	// Stop in Kuscia first (best-effort)
	if s.kusciaClient != nil {
		_ = s.kusciaClient.StopJob(ctx, req.JobID)
	}

	if err := s.jobRepo.UpdateStatus(ctx, job.JobID, "STOPPED", "stopped by user"); err != nil {
		return err
	}

	// Stop all running tasks
	tasks, _ := s.taskRepo.FindByJobID(ctx, req.JobID)
	for _, t := range tasks {
		if t.Status == "RUNNING" || t.Status == "PENDING" {
			_ = s.taskRepo.UpdateStatus(ctx, t.TaskID, "STOPPED", "stopped by user")
		}
	}

	return nil
}

// ListNodeStatus retrieves status of all nodes in a graph execution.
func (s *GraphService) ListNodeStatus(ctx context.Context, req *ListGraphNodeStatusRequest) (*GraphStatusVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	if req.JobID == "" {
		// No job running - all nodes idle
		nodes, _ := s.graphNodeRepo.FindByGraphID(ctx, req.ProjectID, req.GraphID)
		nodeStatuses := make([]GraphNodeStatusVO, 0, len(nodes))
		for _, n := range nodes {
			nodeStatuses = append(nodeStatuses, GraphNodeStatusVO{
				GraphNodeID: n.GraphNodeID,
				Status:      "IDLE",
			})
		}
		return &GraphStatusVO{Status: "IDLE", Nodes: nodeStatuses}, nil
	}

	job, err := s.jobRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	if err != nil {
		return nil, ErrJobNotFound
	}

	tasks, _ := s.taskRepo.FindByProjectAndJobID(ctx, req.ProjectID, req.JobID)
	nodeStatuses := make([]GraphNodeStatusVO, 0, len(tasks))
	for _, t := range tasks {
		nodeStatuses = append(nodeStatuses, GraphNodeStatusVO{
			GraphNodeID: t.GraphNodeID,
			Status:      t.Status,
			ErrMsg:      t.ErrMsg,
		})
	}

	return &GraphStatusVO{
		JobID:  req.JobID,
		Status: job.Status,
		Nodes:  nodeStatuses,
	}, nil
}

// GetNodeOutput retrieves output of a specific graph node.
// Queries Kuscia DomainData for actual output when available, falls back to declared outputs.
func (s *GraphService) GetNodeOutput(ctx context.Context, req *GraphNodeOutputRequest) (*GraphNodeOutputVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	if req.GraphNodeID == "" {
		req.GraphNodeID = req.GraphNodeIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	node, err := s.graphNodeRepo.FindByGraphNodeID(ctx, req.ProjectID, req.GraphID, req.GraphNodeID)
	if err != nil {
		return nil, ErrGraphNotFound
	}

	outputs := make([]NodeOutput, 0)

	// Try querying Kuscia DomainData for actual output data
	if s.kusciaClient != nil && req.JobID != "" {
		// Output data is typically registered as DomainData with job-scoped naming
		outputDataID := req.JobID + "-" + req.GraphNodeID + "-output"
		resp, err := s.kusciaClient.QueryDomainData(ctx, "alice", outputDataID)
		if err == nil {
			outputs = append(outputs, NodeOutput{
				Name: resp.Data.Name,
				Type: resp.Data.Type,
			})
			return &GraphNodeOutputVO{
				GraphNodeID: req.GraphNodeID,
				Outputs:     outputs,
			}, nil
		}
	}

	// Fallback: return the node's declared outputs
	if node.Outputs != "" {
		var outNames []string
		if json.Unmarshal([]byte(node.Outputs), &outNames) == nil {
			for _, name := range outNames {
				outputs = append(outputs, NodeOutput{Name: name, Type: "table"})
			}
		}
	}

	return &GraphNodeOutputVO{
		GraphNodeID: req.GraphNodeID,
		Outputs:     outputs,
	}, nil
}

// GetNodeLogs retrieves logs for a specific graph node.
func (s *GraphService) GetNodeLogs(ctx context.Context, req *GraphNodeLogsRequest) (*GraphNodeLogsVO, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	if req.GraphNodeID == "" {
		req.GraphNodeID = req.GraphNodeIDAlt
	}
	if req.JobID == "" {
		req.JobID = req.JobIDAlt
	}
	if req.TaskID == "" {
		req.TaskID = req.TaskIDAlt
	}
	if req.TaskID == "" && req.JobID != "" {
		// Find task by graph node ID
		tasks, _ := s.taskRepo.FindByJobID(ctx, req.JobID)
		for _, t := range tasks {
			if t.GraphNodeID == req.GraphNodeID {
				req.TaskID = t.TaskID
				break
			}
		}
	}

	logs := make([]string, 0)
	if req.TaskID != "" {
		taskLogs, err := s.taskLogRepo.FindByTaskID(ctx, req.ProjectID, req.JobID, req.TaskID)
		if err == nil {
			for _, l := range taskLogs {
				logs = append(logs, l.Log)
			}
		}
	}

	return &GraphNodeLogsVO{
		GraphNodeID: req.GraphNodeID,
		TaskID:      req.TaskID,
		Logs:        logs,
	}, nil
}

// RefreshNodeMaxIndex recalculates and updates the node max index.
func (s *GraphService) RefreshNodeMaxIndex(ctx context.Context, req *RefreshNodeMaxIndexRequest) (int, error) {
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	graph, err := s.graphRepo.FindByProjectAndGraphID(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return 0, ErrGraphNotFound
	}

	nodes, _ := s.graphNodeRepo.FindByGraphID(ctx, req.ProjectID, req.GraphID)
	if len(nodes) > graph.NodeMaxIndex {
		graph.NodeMaxIndex = len(nodes)
		if err := s.graphRepo.Update(ctx, graph); err != nil {
			return graph.NodeMaxIndex, err
		}
	}

	return graph.NodeMaxIndex, nil
}

// containsStr checks if a string slice contains a value.
func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
