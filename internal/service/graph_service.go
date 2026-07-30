package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/google/uuid"
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
	GraphNodeID string          `json:"graph_node_id"`
	CodeName    string          `json:"code_name"`
	Label       string          `json:"label"`
	X           int             `json:"x"`
	Y           int             `json:"y"`
	Inputs      []string        `json:"inputs"`
	Outputs     []string        `json:"outputs"`
	NodeDef     json.RawMessage `json:"node_def"`
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
// Inputs/Outputs/NodeDef arrive as JSON array/object from the frontend (and as
// JSON-encoded strings from legacy clients); they are normalized to strings
// before storage via rawJSONString.
type GraphNodeReq struct {
	GraphNodeID    string          `json:"graph_node_id"`
	GraphNodeIDAlt string          `json:"graphNodeId"`
	CodeName       string          `json:"code_name"`
	Label          string          `json:"label"`
	X              int             `json:"x"`
	Y              int             `json:"y"`
	Inputs         json.RawMessage `json:"inputs"`
	Outputs        json.RawMessage `json:"outputs"`
	NodeDef        json.RawMessage `json:"node_def"`
}

// rawJSONString normalizes a raw JSON value to the string form stored in the
// database: a JSON string is used verbatim, anything else is compact-marshaled.
func rawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// storedNodeDef mirrors the nodeDef JSON kept on each graph node.
type storedNodeDef struct {
	Domain    string            `json:"domain"`
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	AttrPaths []string          `json:"attrPaths"`
	Attrs     []json.RawMessage `json:"attrs"`
}

func parseStoredNodeDef(raw string) *storedNodeDef {
	if raw == "" {
		return nil
	}
	var nd storedNodeDef
	if json.Unmarshal([]byte(raw), &nd) != nil {
		return nil
	}
	return &nd
}

// datatableIDFromNodeDef extracts the DomainData ID referenced by a
// read_data/datatable node's datatable_selected attribute.
func datatableIDFromNodeDef(raw string) string {
	nd := parseStoredNodeDef(raw)
	if nd == nil {
		return ""
	}
	for i, p := range nd.AttrPaths {
		if p == "datatable_selected" && i < len(nd.Attrs) {
			var attr struct {
				S string `json:"s"`
			}
			if json.Unmarshal(nd.Attrs[i], &attr) == nil {
				return attr.S
			}
		}
	}
	return ""
}

// upstreamNodeID strips the -output-{i} anchor suffix from an edge endpoint.
func upstreamNodeID(anchor string) string {
	if i := strings.LastIndex(anchor, "-output-"); i > 0 {
		return anchor[:i]
	}
	return anchor
}

// nodeStatusString maps internal task statuses to the Java SecretPad-style
// graph node status strings expected by the frontend and API clients.
func nodeStatusString(status string) string {
	if status == "SUCCEEDED" {
		return "SUCCEED"
	}
	return status
}

// --- SecretFlow task_input_config structures ---

type sfDatasourceRef struct {
	ID string `json:"id"`
}

type sfDeviceDesc struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Parties []string `json:"parties"`
	Config  string   `json:"config"`
}

type sfClusterDesc struct {
	Parties      []string          `json:"parties"`
	Devices      []sfDeviceDesc    `json:"devices"`
	RayFedConfig map[string]string `json:"ray_fed_config"`
}

type sfNodeEvalParam struct {
	Version       string            `json:"version"`
	CompID        string            `json:"comp_id"`
	AttrPaths     []string          `json:"attr_paths"`
	Attrs         []json.RawMessage `json:"attrs"`
	CheckpointURI string            `json:"checkpoint_uri,omitempty"`
}

type sfTaskInputConfig struct {
	DatasourceConfig map[string]sfDatasourceRef `json:"sf_datasource_config"`
	ClusterDesc      sfClusterDesc              `json:"sf_cluster_desc"`
	NodeEvalParam    sfNodeEvalParam            `json:"sf_node_eval_param"`
	InputIDs         []string                   `json:"sf_input_ids,omitempty"`
	OutputIDs        []string                   `json:"sf_output_ids,omitempty"`
	OutputURIs       []string                   `json:"sf_output_uris,omitempty"`
}

// buildSFTaskInputConfig renders the task_input_config JSON consumed by
// `python -m secretflow.kuscia.entry` inside the SecretFlow container.
func buildSFTaskInputConfig(nodeDefRaw string, inputIDs, outputIDs, outputURIs []string) string {
	nd := parseStoredNodeDef(nodeDefRaw)
	if nd == nil {
		nd = &storedNodeDef{}
	}
	parties := []string{"alice", "bob"}
	checkpointURI := ""
	if len(outputIDs) > 0 {
		checkpointURI = "ck" + outputIDs[0]
	}
	cfg := sfTaskInputConfig{
		DatasourceConfig: map[string]sfDatasourceRef{
			"alice": {ID: "default-data-source"},
			"bob":   {ID: "default-data-source"},
		},
		ClusterDesc: sfClusterDesc{
			Parties: parties,
			Devices: []sfDeviceDesc{
				{
					Name:    "spu",
					Type:    "spu",
					Parties: parties,
					Config:  `{"runtime_config":{"protocol":"SEMI2K","field":"FM128"},"link_desc":{"connect_retry_times":60,"connect_retry_interval_ms":1000,"brpc_channel_protocol":"http","brpc_channel_connection_type":"pooled","recv_timeout_ms":1200000,"http_timeout_ms":1200000}}`,
				},
				{
					Name:    "heu",
					Type:    "heu",
					Parties: parties,
					Config:  `{"mode":"PHEU","schema":"paillier","key_size":2048}`,
				},
			},
			RayFedConfig: map[string]string{"cross_silo_comm_backend": "brpc_link"},
		},
		NodeEvalParam: sfNodeEvalParam{
			Version:       "1.0.0",
			CompID:        nd.Domain + "/" + nd.Name + ":" + nd.Version,
			AttrPaths:     nd.AttrPaths,
			Attrs:         nd.Attrs,
			CheckpointURI: checkpointURI,
		},
		InputIDs:   inputIDs,
		OutputIDs:  outputIDs,
		OutputURIs: outputURIs,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(raw)
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
	ProjectID      string          `json:"project_id"`
	ProjectIDAlt   string          `json:"projectId"`
	GraphID        string          `json:"graph_id"`
	GraphIDAlt     string          `json:"graphId"`
	GraphNodeID    string          `json:"graph_node_id"`
	GraphNodeIDAlt string          `json:"graphNodeId"`
	NodeDef        json.RawMessage `json:"node_def"`
	// Node carries the Java SecretPad / frontend shape:
	// {projectId, graphId, node: {graphNodeId, codeName, ..., nodeDef}}.
	Node *GraphNodeReq `json:"node"`
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
			Inputs:      splitOutputs(n.Inputs),
			Outputs:     splitOutputs(n.Outputs),
			NodeDef:     rawJSONMessage(n.NodeDef),
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

// rawJSONMessage converts a stored JSON string to json.RawMessage so it is embedded
// as a parsed object/array in the response. Empty or invalid input yields null.
func rawJSONMessage(raw string) json.RawMessage {
	if raw == "" || !json.Valid([]byte(raw)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(raw)
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
			Inputs:      rawJSONString(n.Inputs),
			Outputs:     rawJSONString(n.Outputs),
			NodeDef:     rawJSONString(n.NodeDef),
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
	if req.ProjectID == "" {
		req.ProjectID = req.ProjectIDAlt
	}
	if req.GraphID == "" {
		req.GraphID = req.GraphIDAlt
	}
	if req.GraphNodeID == "" {
		req.GraphNodeID = req.GraphNodeIDAlt
	}
	if req.GraphNodeID == "" && req.Node != nil {
		req.GraphNodeID = firstNonEmpty(req.Node.GraphNodeID, req.Node.GraphNodeIDAlt)
	}

	node, err := s.graphNodeRepo.FindByGraphNodeID(ctx, req.ProjectID, req.GraphID, req.GraphNodeID)
	if err != nil {
		return ErrGraphNotFound
	}

	if req.Node != nil {
		n := req.Node
		if n.CodeName != "" {
			node.CodeName = n.CodeName
		}
		if n.Label != "" {
			node.Label = n.Label
		}
		node.X = n.X
		node.Y = n.Y
		if len(n.Inputs) > 0 {
			node.Inputs = rawJSONString(n.Inputs)
		}
		if len(n.Outputs) > 0 {
			node.Outputs = rawJSONString(n.Outputs)
		}
		if len(n.NodeDef) > 0 {
			node.NodeDef = rawJSONString(n.NodeDef)
		}
	} else if len(req.NodeDef) > 0 {
		node.NodeDef = rawJSONString(req.NodeDef)
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
	OutputID       string `json:"output_id"`
	OutputIDAlt    string `json:"outputId"`
}

// GraphNodeOutputVO represents a node's output.
type GraphNodeOutputVO struct {
	GraphNodeID string          `json:"graph_node_id"`
	Outputs     []NodeOutput    `json:"outputs"`
	Type        string          `json:"type,omitempty"`
	Meta        map[string]any  `json:"meta,omitempty"`
	Tabs        json.RawMessage `json:"tabs,omitempty"`
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
	nodeByID := make(map[string]model.ProjectGraphNodeDO, len(nodes))
	for _, n := range nodes {
		nodeByID[n.GraphNodeID] = n
	}
	runSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if len(req.Nodes) > 0 && !containsStr(req.Nodes, n.GraphNodeID) {
			continue
		}
		runSet[n.GraphNodeID] = true
	}

	var kusciaTasks []kuscia.TaskConfig
	for _, n := range nodes {
		if !runSet[n.GraphNodeID] {
			continue
		}
		// Kuscia task IDs follow the SecretPad convention {jobId}-{graphNodeId}
		// so output DomainData IDs and status sync line up.
		taskID := jobID + "-" + n.GraphNodeID

		// read_data/datatable is a virtual node: no Kuscia task is created and
		// downstream components reference its datatable ID directly.
		if n.CodeName == "read_data/datatable" {
			task := &model.ProjectJobTaskDO{
				ProjectID:   req.ProjectID,
				JobID:       jobID,
				TaskID:      taskID,
				GraphNodeID: n.GraphNodeID,
				Status:      "SUCCEEDED",
				Parties:     "[\"alice\",\"bob\"]",
			}
			_ = s.taskRepo.Create(ctx, task)
			continue
		}

		task := &model.ProjectJobTaskDO{
			ProjectID:   req.ProjectID,
			JobID:       jobID,
			TaskID:      taskID,
			GraphNodeID: n.GraphNodeID,
			Status:      "PENDING",
			Parties:     "[\"alice\",\"bob\"]",
		}
		_ = s.taskRepo.Create(ctx, task)

		// Resolve inputs: an anchor produced by a read_data/datatable node maps
		// to that datatable's DomainData ID; anything else maps to the upstream
		// task output ID ({jobId}-{outputAnchor}) and adds a task dependency.
		inputIDs := make([]string, 0)
		dependencies := make([]string, 0)
		for _, anchor := range splitOutputs(n.Inputs) {
			upID := upstreamNodeID(anchor)
			up, ok := nodeByID[upID]
			if ok && up.CodeName == "read_data/datatable" {
				if dtID := datatableIDFromNodeDef(up.NodeDef); dtID != "" {
					inputIDs = append(inputIDs, dtID)
					continue
				}
			}
			if ok && runSet[upID] {
				dependencies = append(dependencies, jobID+"-"+upID)
			}
			inputIDs = append(inputIDs, jobID+"-"+anchor)
		}

		// Outputs: DomainData ID {jobId}-{outputAnchor}, URI with '-' -> '_'.
		outputAnchors := splitOutputs(n.Outputs)
		outputIDs := make([]string, 0, len(outputAnchors))
		outputURIs := make([]string, 0, len(outputAnchors))
		for _, anchor := range outputAnchors {
			outputID := jobID + "-" + anchor
			outputIDs = append(outputIDs, outputID)
			outputURIs = append(outputURIs, strings.ReplaceAll(outputID, "-", "_"))
		}

		// Build Kuscia task config with the SecretFlow task_input_config.
		kusciaTasks = append(kusciaTasks, kuscia.TaskConfig{
			AppImage:     "secretflow-image",
			Alias:        n.GraphNodeID,
			TaskID:       taskID,
			Dependencies: dependencies,
			InputConfig:  buildSFTaskInputConfig(n.NodeDef, inputIDs, outputIDs, outputURIs),
			Parties: []kuscia.Party{
				{DomainID: "alice", Role: "guest"},
				{DomainID: "bob", Role: "host"},
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
		// Java SecretPad semantics: without an explicit job id, report the
		// status of the latest job launched for this graph.
		req.JobID = s.latestJobIDForGraph(ctx, req.ProjectID, req.GraphID)
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
			// Java SecretPad uses SUCCEED for graph nodes; keep that contract.
			Status: nodeStatusString(t.Status),
			ErrMsg: t.ErrMsg,
		})
	}

	return &GraphStatusVO{
		JobID:  req.JobID,
		Status: job.Status,
		Nodes:  nodeStatuses,
	}, nil
}

// GetNodeOutput retrieves output of a specific graph node.
// When the output DomainData is resolvable in Kuscia it returns the actual
// output metadata (Java SecretPad-compatible type/meta/tabs); otherwise it
// falls back to the node's declared outputs.
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
	if req.OutputID == "" {
		req.OutputID = req.OutputIDAlt
	}
	if req.JobID == "" {
		req.JobID = s.latestJobIDForGraph(ctx, req.ProjectID, req.GraphID)
	}
	node, err := s.graphNodeRepo.FindByGraphNodeID(ctx, req.ProjectID, req.GraphID, req.GraphNodeID)
	if err != nil {
		return nil, ErrGraphNotFound
	}

	vo := &GraphNodeOutputVO{
		GraphNodeID: req.GraphNodeID,
		Outputs:     make([]NodeOutput, 0),
	}

	// Resolve the actual output DomainData ({jobId}-{outputAnchor}).
	if s.kusciaClient != nil && req.JobID != "" && req.OutputID != "" {
		outputDataID := req.JobID + "-" + req.OutputID
		if resp, err := s.kusciaClient.QueryDomainData(ctx, "alice", outputDataID); err == nil {
			vo.Outputs = append(vo.Outputs, NodeOutput{Name: resp.Data.Name, Type: resp.Data.Type})
			vo.Type = resp.Data.Type
			switch resp.Data.Type {
			case "table":
				cols := make([]map[string]string, 0, len(resp.Data.Columns))
				for _, c := range resp.Data.Columns {
					cols = append(cols, map[string]string{"name": c.Name, "type": c.Type})
				}
				vo.Meta = map[string]any{
					"rows": []map[string]string{{
						"tableId":      resp.Data.RelativeURI,
						"domainDataId": resp.Data.DomainDataID,
					}},
					"columns":      cols,
					"datasourceId": resp.Data.DatasourceID,
				}
			case "report":
				// The report content lives in attributes["dist_data"] as a
				// DistData JSON whose packed meta carries the Report tabs.
				if raw := resp.Data.Attributes["dist_data"]; raw != "" {
					var dd struct {
						Meta struct {
							Tabs json.RawMessage `json:"tabs"`
						} `json:"meta"`
					}
					if json.Unmarshal([]byte(raw), &dd) == nil && len(dd.Meta.Tabs) > 0 {
						vo.Tabs = dd.Meta.Tabs
					}
				}
			default:
				vo.Meta = map[string]any{
					"name":         resp.Data.Name,
					"datasourceId": resp.Data.DatasourceID,
				}
			}
			return vo, nil
		}
	}

	// Fallback: return the node's declared outputs
	if node.Outputs != "" {
		var outNames []string
		if json.Unmarshal([]byte(node.Outputs), &outNames) == nil {
			for _, name := range outNames {
				vo.Outputs = append(vo.Outputs, NodeOutput{Name: name, Type: "table"})
			}
		}
	}

	return vo, nil
}

// latestJobIDForGraph returns the most recent job launched for a graph, or ""
// when the graph has never run.
func (s *GraphService) latestJobIDForGraph(ctx context.Context, projectID, graphID string) string {
	jobs, _ := s.jobRepo.FindByProjectID(ctx, projectID)
	for _, j := range jobs {
		if j.GraphID == graphID {
			return j.JobID
		}
	}
	return ""
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
