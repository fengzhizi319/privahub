package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/gorm"
)

// Graph execution chain (Java GraphServiceImpl.startGraph + JobChain:
// JobPersistentHandler -> JobRenderHandler -> JobSubmittedHandler).

// StartGraphOptions tunes a graph run.
type StartGraphOptions struct {
	// Scheduled renders the job without persisting project_job / tasks and
	// without submitting it to Kuscia: the caller (scheduler) receives the
	// DOs and the pre-rendered Kuscia request in StartGraphResult.
	Scheduled bool
	// ScheduleExpectStartDate (e.g. "20260924") is appended to output URIs
	// and used by ODPS partition rules in scheduled mode.
	ScheduleExpectStartDate string
	// JobID forces the job id (scheduled mode); random when empty.
	JobID string
}

// StartGraphResult is the outcome of a graph run.
type StartGraphResult struct {
	JobID string
	// Status is the job status after the chain (RUNNING, or SUCCEED when only
	// platform components were selected).
	Status string
	Job    *model.ProjectJobDO
	Tasks  []model.ProjectJobTaskDO
	Logs   []model.ProjectJobTaskLogDO
	// Request is the rendered Kuscia CreateJob request (nil for pad-only jobs).
	Request *kuscia.CreateJobRequest
}

// projectJob mirrors Java ProjectJob.
type projectJob struct {
	ProjectID      string
	GraphID        string
	Name           string
	JobID          string
	MaxParallelism int
	FullNodes      []graphjob.GraphNodeInfo
	Edges          []graphjob.GraphEdge
	Tasks          []*jobTask
}

// jobTask mirrors ProjectJob.JobTask.
type jobTask struct {
	TaskID       string
	Parties      []string
	Status       string
	Dependencies []string
	Node         graphjob.GraphNodeInfo
}

func (j *projectJob) clone() *projectJob {
	c := *j
	c.FullNodes = make([]graphjob.GraphNodeInfo, len(j.FullNodes))
	for i := range j.FullNodes {
		c.FullNodes[i] = j.FullNodes[i].Clone()
	}
	c.Tasks = make([]*jobTask, len(j.Tasks))
	for i, t := range j.Tasks {
		ct := *t
		ct.Parties = append([]string(nil), t.Parties...)
		ct.Dependencies = append([]string(nil), t.Dependencies...)
		ct.Node = t.Node.Clone()
		c.Tasks[i] = &ct
	}
	return &c
}

// partitionInfo mirrors GraphContext.PartitionInfo.
type partitionInfo struct {
	ReadRule         string
	TableName        string
	PartitionColumns map[string]bool
}

// chainContext mirrors GraphContext (per-run state instead of a ThreadLocal).
type chainContext struct {
	project    *model.ProjectDO
	opts       StartGraphOptions
	breakpoint bool
	teeNodeID  string
	tableAttrs []graphjob.TableAttr
	partitions map[string]partitionInfo
	result     *StartGraphResult
	persisted  bool
}

// jobChainHandler is one link of the job chain.
type jobChainHandler interface {
	handle(ctx context.Context, cc *chainContext, job *projectJob) (*projectJob, error)
}

// StartGraph mirrors GraphServiceImpl.startGraph and submits the job.
func (s *GraphService) StartGraph(ctx context.Context, req *StartGraphRequest) (*StartGraphVO, error) {
	res, err := s.RunGraphJob(ctx, req, StartGraphOptions{})
	if err != nil {
		return nil, err
	}
	return &StartGraphVO{JobID: res.JobID}, nil
}

// SubmitRenderedJob submits a pre-rendered Kuscia request (scheduled mode).
func (s *GraphService) SubmitRenderedJob(ctx context.Context, req *kuscia.CreateJobRequest) error {
	if s.kuscia == nil {
		return padErr(errcode.ProjectJobCreateError, "kuscia client unavailable")
	}
	if _, err := s.kuscia.CreateJob(ctx, req); err != nil {
		return padErr(errcode.ProjectJobCreateError, err.Error())
	}
	return nil
}

// RunGraphJob runs the whole chain and returns the rendered artifacts.
func (s *GraphService) RunGraphJob(ctx context.Context, req *StartGraphRequest, opts StartGraphOptions) (*StartGraphResult, error) {
	g, err := s.loadGraph(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}
	if err := ownerCheck(ctx, g); err != nil {
		return nil, err
	}
	nodeDOs, err := s.loadGraphNodes(ctx, req.ProjectID, req.GraphID)
	if err != nil {
		return nil, err
	}
	if len(nodeDOs) == 0 {
		return nil, padErr(errcode.GraphNodeNotExists)
	}
	fullNodes := make([]graphjob.GraphNodeInfo, 0, len(nodeDOs))
	byID := map[string]graphjob.GraphNodeInfo{}
	for i := range nodeDOs {
		info := nodeInfoFromDO(&nodeDOs[i])
		fullNodes = append(fullNodes, info)
		byID[info.GraphNodeID] = info
	}
	selected := make([]graphjob.GraphNodeInfo, 0)
	if len(req.Nodes) == 0 {
		selected = append(selected, fullNodes...)
	} else {
		for _, id := range req.Nodes {
			n, ok := byID[id]
			if !ok {
				return nil, padErr(errcode.GraphNodeNotExists, id)
			}
			selected = append(selected, n)
		}
	}
	var project model.ProjectDO
	if err := s.db.WithContext(ctx).Where("project_id = ?", req.ProjectID).First(&project).Error; err != nil {
		return nil, padErrBase(ErrProjectNotFound, errcode.ProjectNotExists)
	}
	cc := &chainContext{project: &project, opts: opts, breakpoint: req.Breakpoint, partitions: map[string]partitionInfo{}}
	if strings.EqualFold(project.ComputeMode, "TEE") {
		// GraphContext.getTeeNodeId: the project's TEE domain (tee.domain-id fallback).
		cc.teeNodeID = projectTeeNodeID(&project)
		if cc.teeNodeID == "" {
			cc.teeNodeID = s.teeConfig().DomainID
		}
	}

	edges := graphjob.ParseEdges(g.Edges)
	parties, err := s.findParties(ctx, cc, req.ProjectID, edges, byID, selected)
	if err != nil {
		return nil, err
	}
	if cc.teeNodeID != "" {
		// TEE mode: every selected component runs on the TEE node only.
		for id := range parties {
			parties[id] = []string{cc.teeNodeID}
		}
	}
	all := map[string]bool{}
	for _, ps := range parties {
		for _, p := range ps {
			all[p] = true
		}
	}
	if err := s.verifyNodeAndRouteHealthy(ctx, req.ProjectID, all); err != nil {
		return nil, err
	}

	jobID := opts.JobID
	if jobID == "" {
		jobID = s.newJobID(ctx)
	}
	maxPar := g.MaxParallelism
	if maxPar <= 0 {
		maxPar = s.settings().MaxParallelism
	}
	job := &projectJob{
		ProjectID: req.ProjectID, GraphID: req.GraphID, Name: g.Name, JobID: jobID,
		MaxParallelism: maxPar, FullNodes: fullNodes, Edges: edges,
	}
	for _, n := range selected {
		job.Tasks = append(job.Tasks, &jobTask{
			TaskID:  graphjob.GenTaskID(jobID, n.GraphNodeID),
			Parties: parties[n.GraphNodeID],
			Node:    n.Clone(),
		})
	}
	cc.result = &StartGraphResult{JobID: jobID}

	chain := []jobChainHandler{&jobPersistentHandler{s}, &jobRenderHandler{s}, &jobSubmittedHandler{s}}
	cur := job
	for _, h := range chain {
		next, err := h.handle(ctx, cc, cur)
		if err != nil {
			if cc.persisted && !opts.Scheduled {
				s.rollbackJob(ctx, req.ProjectID, jobID)
			}
			return nil, err
		}
		cur = next
	}
	return cc.result, nil
}

func projectTeeNodeID(p *model.ProjectDO) string {
	var info struct {
		TeeDomainID string `json:"teeDomainId"`
	}
	if json.Unmarshal([]byte(p.ProjectInfo), &info) == nil {
		return info.TeeDomainID
	}
	return ""
}

// newJobID generates a unique 4-letter job id (UUIDUtils.random(4)).
func (s *GraphService) newJobID(ctx context.Context) string {
	for i := 0; i < 20; i++ {
		id := graphjob.RandomLetters(4)
		var cnt int64
		s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).Where("job_id = ?", id).Count(&cnt)
		if cnt == 0 {
			var scnt int64
			s.db.WithContext(ctx).Model(&model.ProjectScheduleJobDO{}).Where("job_id = ?", id).Count(&scnt)
			if scnt == 0 {
				return id
			}
		}
	}
	return graphjob.RandomLetters(8)
}

// rollbackJob removes everything persisted for a job whose submission failed
// (Java: @Transactional rollback of startGraph).
func (s *GraphService) rollbackJob(ctx context.Context, projectID, jobID string) {
	_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Unscoped().Where("project_id = ? AND job_id = ?", projectID, jobID).Delete(&model.ProjectJobTaskLogDO{})
		tx.Unscoped().Where("project_id = ? AND job_id = ?", projectID, jobID).Delete(&model.ProjectJobTaskDO{})
		tx.Unscoped().Where("project_id = ? AND job_id = ?", projectID, jobID).Delete(&model.ProjectJobDO{})
		tx.Unscoped().Where("project_id = ? AND job_id = ?", projectID, jobID).Delete(&model.ProjectGraphNodeKusciaParamsDO{})
		return nil
	})
}

// findParties mirrors findTopNodes + findParties: a node's parties are the
// owners of the datatables read by its root ancestors (plus the hidden party
// of unbalance_psi_cache); table attrs come from the project datatable config.
func (s *GraphService) findParties(ctx context.Context, cc *chainContext, projectID string, edges []graphjob.GraphEdge,
	byID map[string]graphjob.GraphNodeInfo, selected []graphjob.GraphNodeInfo) (map[string][]string, error) {
	type entry struct {
		id   string
		tops map[string]bool
	}
	entries := make([]entry, 0, len(selected))
	for _, n := range selected {
		entries = append(entries, entry{n.GraphNodeID, graphjob.FindTopNodes(edges, n.GraphNodeID)})
	}
	sort.SliceStable(entries, func(i, j int) bool { return len(entries[i].tops) < len(entries[j].tops) })

	attrSeen := map[string]bool{}
	result := make(map[string][]string, len(entries))
	for _, e := range entries {
		set := map[string]bool{}
		topIDs := make([]string, 0, len(e.tops))
		for id := range e.tops {
			topIDs = append(topIDs, id)
		}
		sort.Strings(topIDs)
		for _, topID := range topIDs {
			top, ok := byID[topID]
			if !ok {
				continue
			}
			dtID := top.Def().DatatableID()
			if dtID == "" {
				continue
			}
			var dts []model.ProjectDatatableDO
			if err := s.db.WithContext(ctx).Where("project_id = ? AND datatable_id = ?", projectID, dtID).Order("id ASC").Find(&dts).Error; err != nil {
				return nil, err
			}
			for _, dt := range dts {
				set[dt.NodeID] = true
			}
			if len(dts) > 0 && !attrSeen[dtID] {
				attrSeen[dtID] = true
				cc.tableAttrs = append(cc.tableAttrs, graphjob.BuildTableAttr(dtID, graphjob.ParseColumnConfigs(dts[0].TableConfigs)))
			}
		}
		if cur, ok := byID[e.id]; ok && strings.EqualFold(cur.CodeName, graphjob.DataPrepUnbalancePSICache) {
			if p := cur.Def().HiddenPartyID(); p != "" {
				set[p] = true
			}
		}
		parties := make([]string, 0, len(set))
		for p := range set {
			parties = append(parties, p)
		}
		sort.Strings(parties)
		result[e.id] = parties
	}
	// Components without data input have no natural initiator: fall back to
	// the first project node.
	var defaultInitiator string
	var pn model.ProjectNodeDO
	if s.db.WithContext(ctx).Where("project_id = ?", projectID).Order("id ASC").First(&pn).Error == nil {
		defaultInitiator = pn.NodeID
	}
	for id, ps := range result {
		if len(ps) == 0 && defaultInitiator != "" {
			result[id] = []string{defaultInitiator}
		}
	}
	return result, nil
}

// verifyNodeAndRouteHealthy mirrors GraphServiceImpl.verifyNodeAndRouteHealthy:
// AUTONOMY checks the routes towards the local institution's nodes (see
// verifyAutonomyRoutes); CENTER / EDGE require every party Ready and every
// route Succeeded.
func (s *GraphService) verifyNodeAndRouteHealthy(ctx context.Context, projectID string, parties map[string]bool) error {
	ids := make([]string, 0, len(parties))
	for p := range parties {
		ids = append(ids, p)
	}
	sort.Strings(ids)
	if GlobalEnv().IsAutonomy() {
		return s.verifyAutonomyRoutes(ctx, projectID, ids)
	}
	for _, id := range ids {
		var n model.NodeDO
		if err := s.db.WithContext(ctx).Where("node_id = ?", id).First(&n).Error; err != nil {
			return padErr(errcode.GraphNodeNotExists, id)
		}
	}
	if s.kuscia == nil || !s.settings().HealthCheck {
		return nil
	}
	for _, id := range ids {
		resp, err := s.kuscia.QueryDomain(ctx, id)
		if err != nil || resp == nil || !resp.Data.IsReady() {
			return padErr(errcode.GraphNodeNotExists, nodeName(ctx, s.db, id))
		}
	}
	for _, src := range ids {
		for _, dst := range ids {
			if src == dst {
				continue
			}
			resp, err := s.kuscia.QueryDomainRoute(ctx, src, dst)
			if err != nil || resp == nil || resp.Data.Status.Status != kuscia.RouteStatusSucceeded {
				return padErr(errcode.GraphNodeRouteNotExists, nodeName(ctx, s.db, src)+"->"+nodeName(ctx, s.db, dst))
			}
		}
	}
	return nil
}

// --- JobPersistentHandler ---

type jobPersistentHandler struct{ s *GraphService }

func (h *jobPersistentHandler) handle(ctx context.Context, cc *chainContext, job *projectJob) (*projectJob, error) {
	now := timeNow()
	jobDO := &model.ProjectJobDO{
		ProjectID: job.ProjectID,
		JobID:     job.JobID,
		Name:      truncate(job.Name, 40),
		Status:    graphjob.JobRunning,
		GraphID:   job.GraphID,
		Edges:     graphjob.EdgesJSON(job.Edges),
	}
	tasks := make([]model.ProjectJobTaskDO, 0, len(job.Tasks))
	logs := make([]model.ProjectJobTaskLogDO, 0)
	for _, t := range job.Tasks {
		t.Status = graphjob.TaskInitialized
		if t.Node.IsPad() {
			t.Status = graphjob.TaskSucceed
		}
		snapshot, _ := json.Marshal(t.Node)
		partiesJSON, _ := json.Marshal(t.Parties)
		tasks = append(tasks, model.ProjectJobTaskDO{
			ProjectID:   job.ProjectID,
			JobID:       job.JobID,
			TaskID:      t.TaskID,
			Parties:     string(partiesJSON),
			Status:      t.Status,
			GraphNodeID: t.Node.GraphNodeID,
			GraphNode:   string(snapshot),
		})
		if t.Status == graphjob.TaskSucceed {
			logs = append(logs,
				model.ProjectJobTaskLogDO{ProjectID: job.ProjectID, JobID: job.JobID, TaskID: t.TaskID, Log: graphjob.TaskStartLog(now, job.JobID, t.TaskID)},
				model.ProjectJobTaskLogDO{ProjectID: job.ProjectID, JobID: job.JobID, TaskID: t.TaskID, Log: graphjob.TaskSucceedLog(now, job.JobID, t.TaskID)},
			)
		}
	}
	cc.result.Job, cc.result.Tasks, cc.result.Logs = jobDO, tasks, logs
	cc.result.Status = graphjob.JobRunning
	if cc.opts.Scheduled {
		return job, nil
	}
	err := h.s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(jobDO).Error; err != nil {
			return err
		}
		if len(tasks) > 0 {
			if err := tx.Create(&tasks).Error; err != nil {
				return err
			}
		}
		if len(logs) > 0 {
			if err := tx.Create(&logs).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	cc.persisted = true
	return job, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// --- JobSubmittedHandler (+ KusciaJobConverter) ---

type jobSubmittedHandler struct{ s *GraphService }

func (h *jobSubmittedHandler) handle(ctx context.Context, cc *chainContext, job *projectJob) (*projectJob, error) {
	if len(job.Tasks) == 0 {
		// Only platform components selected: the job is done.
		now := timeNow()
		cc.result.Status = graphjob.JobSucceed
		cc.result.Job.Status = graphjob.JobSucceed
		cc.result.Job.FinishedTime = &now
		if !cc.opts.Scheduled {
			if err := h.s.db.WithContext(ctx).Model(&model.ProjectJobDO{}).
				Where("project_id = ? AND job_id = ?", job.ProjectID, job.JobID).
				Updates(map[string]any{"status": graphjob.JobSucceed, "finished_time": &now}).Error; err != nil {
				return nil, err
			}
		}
		return job, nil
	}
	var req *kuscia.CreateJobRequest
	var err error
	if cc.teeNodeID != "" {
		req, err = h.s.convertTrustedFlowJob(ctx, cc, job)
	} else {
		req, err = h.s.convertKusciaJob(ctx, cc, job)
	}
	if err != nil {
		return nil, err
	}
	cc.result.Request = req
	if cc.opts.Scheduled {
		return job, nil
	}
	if h.s.kuscia == nil {
		return nil, padErr(errcode.ProjectJobCreateError, "kuscia client unavailable")
	}
	if _, err := h.s.kuscia.CreateJob(ctx, req); err != nil {
		return nil, padErr(errcode.ProjectJobCreateError, err.Error())
	}
	return job, nil
}

// convertKusciaJob mirrors KusciaJobConverter.converter.
func (s *GraphService) convertKusciaJob(ctx context.Context, cc *chainContext, job *projectJob) (*kuscia.CreateJobRequest, error) {
	st := s.settings()
	initiator := ""
	tasks := make([]kuscia.TaskConfig, 0, len(job.Tasks))
	autonomy := GlobalEnv().IsAutonomy()
	for _, t := range job.Tasks {
		if len(t.Parties) > 0 {
			initiator = t.Parties[0]
			if autonomy {
				// Java: envService.findLocalNodeId(task) in AUTONOMY mode.
				if local := s.findLocalNodeID(ctx, job.ProjectID, t.Parties); local != "" {
					initiator = local
				}
			}
		}
		inputConfig, evalParam, err := s.renderTaskInputConfig(ctx, cc, job, t)
		if err != nil {
			return nil, err
		}
		inputsJSON, _ := json.Marshal(t.Node.Inputs)
		outputsJSON, _ := json.Marshal(t.Node.Outputs)
		if err := s.saveKusciaParams(ctx, &model.ProjectGraphNodeKusciaParamsDO{
			ProjectID: job.ProjectID, GraphID: job.GraphID, GraphNodeID: t.Node.GraphNodeID,
			JobID: job.JobID, TaskID: t.TaskID, Inputs: string(inputsJSON), Outputs: string(outputsJSON),
			NodeEvalParam: evalParam,
		}); err != nil {
			return nil, err
		}
		parties := make([]kuscia.Party, 0, len(t.Parties))
		for _, p := range t.Parties {
			parties = append(parties, kuscia.Party{DomainID: p})
		}
		appImage := st.AppImage
		if strings.EqualFold(t.Node.CodeName, graphjob.StatsSCQLAnalysis) {
			appImage = st.SCQLImage
		}
		tasks = append(tasks, kuscia.TaskConfig{
			AppImage:     appImage,
			Parties:      parties,
			Alias:        t.TaskID,
			TaskID:       t.TaskID,
			Dependencies: t.Dependencies,
			InputConfig:  inputConfig,
		})
	}
	return &kuscia.CreateJobRequest{
		JobID:          job.JobID,
		Initiator:      initiator,
		MaxParallelism: clampInt32(job.MaxParallelism),
		Tasks:          tasks,
	}, nil
}

func (s *GraphService) saveKusciaParams(ctx context.Context, p *model.ProjectGraphNodeKusciaParamsDO) error {
	var existing model.ProjectGraphNodeKusciaParamsDO
	err := s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ? AND graph_node_id = ?", p.ProjectID, p.GraphID, p.GraphNodeID).
		First(&existing).Error
	if err == nil {
		p.BaseDO = existing.BaseDO
		return s.db.WithContext(ctx).Save(p).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.WithContext(ctx).Create(p).Error
}

// renderTaskInputConfig mirrors KusciaJobConverter.renderTaskInputConfig.
func (s *GraphService) renderTaskInputConfig(ctx context.Context, cc *chainContext, job *projectJob, t *jobTask) (string, string, error) {
	st := s.settings()
	nd := t.Node.Def()
	checkpoint := ""
	if cc.breakpoint {
		prev := lastTwoTasks(ctx, s.db, job.ProjectID, t.Node.GraphNodeID)
		if len(prev) == 2 {
			pn := loadTaskNodeInfo(ctx, s.db, &prev[1], job.GraphID)
			if len(pn.Outputs) > 0 {
				checkpoint = graphjob.CheckpointPrefix + graphjob.GenTaskOutputID(prev[1].JobID, pn.Outputs[0])
			}
		}
	} else if len(t.Node.Outputs) > 0 && t.Node.Outputs[0] != "" {
		checkpoint = graphjob.CheckpointPrefix + t.Node.Outputs[0]
	}
	dsCfg := make(map[string]graphjob.DatasourceRef, len(t.Parties))
	for _, p := range t.Parties {
		dsCfg[p] = graphjob.DatasourceRef{ID: graphDatasourceFor(ctx, s.db, job.ProjectID, job.GraphID, p)}
	}
	if strings.EqualFold(t.Node.CodeName, graphjob.StatsSCQLAnalysis) {
		eval := graphjob.BuildEvalParam(nd, checkpoint)
		evalJSON, _ := json.Marshal(eval)
		cfg, err := s.renderScqlInputConfig(ctx, job, t, nd, dsCfg)
		if err != nil {
			return "", "", err
		}
		return cfg, string(evalJSON), nil
	}
	outputURIs := make([]string, 0, len(t.Node.Outputs))
	for _, o := range t.Node.Outputs {
		outputURIs = append(outputURIs, graphjob.OutputURI(o, cc.opts.ScheduleExpectStartDate))
	}
	partitions := make([]string, 0, len(t.Node.Inputs))
	for _, in := range t.Node.Inputs {
		pi, ok := cc.partitions[in]
		spec := ""
		if ok && pi.ReadRule != "" {
			v, err := s.partitionRule.ReadPartitionRuleAnalysis(pi.TableName, DataSourceTypeODPS,
				graphjob.PartitionRuleToDomainData(pi.ReadRule), cc.opts.ScheduleExpectStartDate, pi.PartitionColumns)
			if err != nil {
				return "", "", padErr(errcode.SystemValidationError, err.Error())
			}
			spec = v
		}
		partitions = append(partitions, spec)
	}
	eval := graphjob.BuildEvalParam(nd, checkpoint)
	cfg := &graphjob.TaskInputConfig{
		DatasourceConfig:    dsCfg,
		ClusterDesc:         graphjob.BuildClusterDesc(st, t.Parties),
		NodeEvalParam:       eval,
		InputIDs:            t.Node.Inputs,
		InputPartitionsSpec: partitions,
		OutputIDs:           t.Node.Outputs,
		OutputURIs:          outputURIs,
		TableAttrs:          cc.tableAttrs,
	}
	evalJSON, _ := json.Marshal(eval)
	return cfg.Marshal(), string(evalJSON), nil
}

// clampInt32 converts an int to int32, saturating at the int32 bounds so an
// oversized MaxParallelism cannot wrap to a negative/small value.
func clampInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
