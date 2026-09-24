package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// Model export (pack) — Java ModelExportServiceImpl + JobManager.syncModelExportJob.
// The Java MODEL_EXPORT_CACHE is replaced by the project_model_export table.

// Model export task statuses (Java GraphNodeTaskStatus names).
const (
	ModelExportInitialized = "INITIALIZED"
	ModelExportRunning     = "RUNNING"
	ModelExportSucceed     = "SUCCEED"
	ModelExportFailed      = "FAILED"
)

// modelExportRetention is how long finished export records are kept so that
// model/status keeps answering after the job finished.
const modelExportRetention = 24 * time.Hour

// ModelComponent is Java ModelComponent.
type ModelComponent struct {
	GraphNodeID string `json:"graphNodeId"`
	Domain      string `json:"domain"`
	Name        string `json:"name"`
	Version     string `json:"version"`
}

// ModelPartyConfig is Java ModelPartyConfig.
type ModelPartyConfig struct {
	ModelParty      string `json:"modelParty"`
	ModelDataSource string `json:"modelDataSource"`
	ModelDataName   string `json:"modelDataName"`
}

// ModelExportPackageRequest is Java ModelExportPackageRequest.
type ModelExportPackageRequest struct {
	ProjectID         string             `json:"projectId"`
	GraphID           string             `json:"graphId"`
	TrainID           string             `json:"trainId"`
	ModelName         string             `json:"modelName"`
	ModelDesc         string             `json:"modelDesc"`
	GraphNodeOutPutID string             `json:"graphNodeOutPutId"`
	ModelPartyConfig  []ModelPartyConfig `json:"modelPartyConfig"`
	ModelComponent    []ModelComponent   `json:"modelComponent"`
}

// ModelExportPackageResponse is Java ModelExportPackageResponse.
type ModelExportPackageResponse struct {
	JobID string `json:"jobId"`
}

// ModelExportDTO is Java ModelExportDTO (model/status response).
type ModelExportDTO struct {
	ProjectID        string            `json:"projectId"`
	GraphID          string            `json:"graphId"`
	Initiator        string            `json:"initiator"`
	ModelID          string            `json:"modelId"`
	ModelName        string            `json:"modelName"`
	ModelDesc        string            `json:"modelDesc"`
	SampleTables     string            `json:"sampleTables"`
	ModelList        []string          `json:"modelList"`
	JobID            string            `json:"jobId"`
	TaskID           string            `json:"taskId"`
	TrainID          string            `json:"trainId"`
	GraphDetail      string            `json:"graphDetail"`
	ModelReportID    string            `json:"modelReportId"`
	PartyDataSources []PartyDataSource `json:"partyDataSources"`
	Status           string            `json:"status"`
	ErrMsg           string            `json:"errMsg"`
	// Msg duplicates ErrMsg (status VO convenience).
	Msg string `json:"msg,omitempty"`
}

// ModelExportStatusRequest is Java ModelExportStatusRequest.
type ModelExportStatusRequest struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
}

// ModelPartyPathRequest is Java ModelPartyPathRequest (graphId accepted as a Go extension).
type ModelPartyPathRequest struct {
	ProjectID         string `json:"projectId"`
	GraphID           string `json:"graphId"`
	GraphNodeID       string `json:"graphNodeId"`
	GraphNodeOutPutID string `json:"graphNodeOutPutId"`
}

// ModelPartyDataSourceVO is Java ProjectGraphDomainDataSourceVO.DataSource.
type ModelPartyDataSourceVO struct {
	DataSourceID   string `json:"dataSourceId"`
	DataSourceName string `json:"dataSourceName"`
	NodeID         string `json:"nodeId"`
	Type           string `json:"type"`
}

// ModelPartyPathVO is Java ModelPartyPathResponse.
type ModelPartyPathVO struct {
	NodeID      string                   `json:"nodeId"`
	NodeName    string                   `json:"nodeName"`
	DataSources []ModelPartyDataSourceVO `json:"dataSources"`
}

// exportInitiator mirrors ModelExportServiceImpl.getInitiator.
func (s *ModelService) exportInitiator(parties []string) string {
	if s.isAutonomy() {
		return s.localParty(parties)
	}
	return s.platformNodeID()
}

// ExportModel mirrors ModelExportServiceImpl.exportModel: submits a
// model/model_export Kuscia job and records it for status sync.
func (s *ModelService) ExportModel(ctx context.Context, req *ModelExportPackageRequest) (*ModelExportPackageResponse, error) {
	if req.ProjectID == "" || req.GraphID == "" || req.TrainID == "" || req.ModelName == "" {
		return nil, padErr(errcode.SystemValidationError, "projectId, graphId, trainId and modelName are required")
	}
	if len(req.ModelPartyConfig) == 0 || len(req.ModelComponent) == 0 {
		return nil, padErr(errcode.SystemValidationError, "modelPartyConfig and modelComponent are required")
	}
	parties := make([]string, 0, len(req.ModelPartyConfig))
	seen := map[string]bool{}
	for _, m := range req.ModelPartyConfig {
		if m.ModelParty == "" {
			return nil, padErr(errcode.SystemValidationError, "modelParty is required")
		}
		if !seen[m.ModelParty] {
			seen[m.ModelParty] = true
			parties = append(parties, m.ModelParty)
		}
	}
	modelDataName := strings.ToLower(graphjob.RandomLetters(2)) + "_" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	for i := range req.ModelPartyConfig {
		req.ModelPartyConfig[i].ModelDataName = modelDataName
	}
	initiator := s.exportInitiator(parties)
	if s.isAutonomy() && initiator == "" {
		return nil, padErr(errcode.ModelExportFailed, "initiator must be one of party")
	}
	task, err := latestTask(ctx, s.db, req.ProjectID, req.TrainID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, padErr(errcode.GraphNodeOutputNotExists)
	}
	graphDetail := ""
	if s.graph != nil {
		if gd, err := s.graph.GetGraphDetail(ctx, &GetGraphRequest{ProjectID: req.ProjectID, GraphID: req.GraphID}); err == nil {
			b, _ := json.Marshal(gd)
			graphDetail = string(b)
		} else {
			return nil, err
		}
	}
	modelList := make([]string, 0, len(req.ModelComponent))
	for _, c := range req.ModelComponent {
		modelList = append(modelList, c.GraphNodeID)
	}
	pds := make([]PartyDataSource, 0, len(req.ModelPartyConfig))
	for _, m := range req.ModelPartyConfig {
		pds = append(pds, PartyDataSource{PartyID: m.ModelParty, Datasource: m.ModelDataSource + "/" + modelDataName})
	}
	dto := &ModelExportDTO{
		ProjectID:        req.ProjectID,
		GraphID:          req.GraphID,
		ModelName:        req.ModelName,
		ModelDesc:        req.ModelDesc,
		GraphDetail:      graphDetail,
		PartyDataSources: pds,
		ModelList:        modelList,
		TrainID:          task.JobID + "-" + req.GraphNodeOutPutID,
		Initiator:        initiator,
		Status:           ModelExportInitialized,
	}
	jobReq, err := s.buildModelExportJob(ctx, req, parties, dto)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(dto)
	rec := &model.ProjectModelExportDO{JobID: dto.JobID, ProjectID: req.ProjectID, Status: dto.Status, Payload: string(payload)}
	if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
		return nil, err
	}
	c := s.kc(jobReq.Initiator)
	if c == nil {
		s.db.WithContext(ctx).Unscoped().Delete(rec)
		return nil, padErr(errcode.ModelExportFailed, "kuscia client not configured")
	}
	if _, err := c.CreateJob(ctx, jobReq); err != nil {
		s.db.WithContext(ctx).Unscoped().Delete(rec)
		return nil, padErr(errcode.ModelExportFailed, err.Error())
	}
	return &ModelExportPackageResponse{JobID: dto.JobID}, nil
}

// buildModelExportJob mirrors ModelExportServiceImpl.buildKusciaParams.
func (s *ModelService) buildModelExportJob(ctx context.Context, req *ModelExportPackageRequest, parties []string, dto *ModelExportDTO) (*kuscia.CreateJobRequest, error) {
	st := graphjob.GetSettings()
	taskID := graphjob.RandomLetters(4) + "-model-export"
	jobID := graphjob.RandomLetters(4)
	id := jobID + "-" + taskID
	outputs := []string{id, id + "-report"}
	outputURIs := []string{req.ModelPartyConfig[0].ModelDataName, id + "-report"}

	eval, err := s.buildModelExportEvalParam(ctx, req)
	if err != nil {
		return nil, err
	}
	dsCfg := make(map[string]graphjob.DatasourceRef, len(parties))
	for _, p := range parties {
		dsCfg[p] = graphjob.DatasourceRef{ID: graphDatasourceFor(ctx, s.db, req.ProjectID, req.GraphID, p)}
	}
	for _, m := range req.ModelPartyConfig {
		if _, ok := dsCfg[m.ModelParty]; ok && m.ModelDataSource != "" {
			dsCfg[m.ModelParty] = graphjob.DatasourceRef{ID: m.ModelDataSource}
		}
	}
	cfg := &graphjob.TaskInputConfig{
		DatasourceConfig: dsCfg,
		ClusterDesc:      graphjob.BuildClusterDesc(st, parties),
		NodeEvalParam:    eval,
		OutputIDs:        outputs,
		OutputURIs:       outputURIs,
	}
	inputConfig := cfg.Marshal()

	// sample tables: read tables among the train node's root ancestors, owned by the top parties.
	var g model.ProjectGraphDO
	if s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", req.ProjectID, req.GraphID).First(&g).Error != nil {
		return nil, padErr(errcode.GraphNotExists)
	}
	var nodes []model.ProjectGraphNodeDO
	s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ?", req.ProjectID, req.GraphID).Find(&nodes)
	topNodes := graphjob.FindTopNodes(graphjob.ParseEdges(g.Edges), req.TrainID)
	topParties := map[string]bool{}
	readTables := map[string]bool{}
	hasReadTable := false
	for i := range nodes {
		n := &nodes[i]
		info := nodeInfoFromDO(n)
		dtID := info.Def().DatatableID()
		if topNodes[n.GraphNodeID] && dtID != "" {
			var dts []model.ProjectDatatableDO
			s.db.WithContext(ctx).Where("project_id = ? AND datatable_id = ?", req.ProjectID, dtID).Find(&dts)
			for _, d := range dts {
				topParties[d.NodeID] = true
			}
		}
		if n.CodeName == graphjob.CompReadDataDatatable {
			hasReadTable = true
			if topNodes[n.GraphNodeID] {
				readTables[dtID] = true
			}
		}
	}
	if !hasReadTable {
		return nil, padErr(errcode.ModelExportFailed, "read table not found")
	}
	sample := map[string]string{}
	var pdts []model.ProjectDatatableDO
	s.db.WithContext(ctx).Where("project_id = ?", req.ProjectID).Order("id ASC").Find(&pdts)
	for _, p := range pdts {
		if readTables[p.DatatableID] && topParties[p.NodeID] {
			sample[p.NodeID] = p.DatatableID
		}
	}
	sampleJSON, _ := json.Marshal(sample)
	dto.SampleTables = string(sampleJSON)
	dto.JobID = jobID
	dto.TaskID = taskID
	dto.ModelID = id
	dto.ModelReportID = id + "-report"
	s.log.Info("model export job", zap.String("job_id", jobID), zap.String("sample_tables", dto.SampleTables))

	initiator := parties[0]
	if !s.isCenter() {
		initiator = s.exportInitiator(parties)
		if initiator == "" {
			initiator = parties[0]
		}
	}
	taskParties := make([]kuscia.Party, 0, len(parties))
	for _, p := range parties {
		taskParties = append(taskParties, kuscia.Party{DomainID: p})
	}
	maxPar := st.MaxParallelism
	if maxPar <= 0 {
		maxPar = 1
	}
	return &kuscia.CreateJobRequest{
		JobID:          jobID,
		Initiator:      initiator,
		MaxParallelism: int32(maxPar),
		Tasks: []kuscia.TaskConfig{{
			TaskID:      taskID,
			Alias:       taskID,
			AppImage:    st.AppImage,
			Parties:     taskParties,
			InputConfig: inputConfig,
		}},
	}, nil
}

// buildModelExportEvalParam mirrors ModelExportServiceImpl.buildNodeEvalParam
// (comp_id form of the bundled SecretFlow).
func (s *ModelService) buildModelExportEvalParam(ctx context.Context, req *ModelExportPackageRequest) (graphjob.NodeEvalParam, error) {
	inputs, outputs, params := []string{}, []string{}, []string{}
	for _, c := range req.ModelComponent {
		var kp model.ProjectGraphNodeKusciaParamsDO
		if s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ? AND graph_node_id = ?", req.ProjectID, req.GraphID, c.GraphNodeID).
			First(&kp).Error != nil {
			return graphjob.NodeEvalParam{}, padErr(errcode.ModelExportFailed, "component needs to have been successfully run: "+c.GraphNodeID)
		}
		if isIOComponent(kp.NodeEvalParam) {
			continue
		}
		var in, out []string
		_ = json.Unmarshal([]byte(kp.Inputs), &in)
		_ = json.Unmarshal([]byte(kp.Outputs), &out)
		inputs = append(inputs, in...)
		outputs = append(outputs, out...)
		params = append(params, base64.StdEncoding.EncodeToString([]byte(stripCheckpoint(kp.NodeEvalParam))))
	}
	if len(params) == 0 {
		return graphjob.NodeEvalParam{}, padErr(errcode.ModelExportFailed, "no exportable model component")
	}
	attr := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	return graphjob.NodeEvalParam{
		Version:   "1.0.0",
		CompID:    "model/model_export:1.0.0",
		AttrPaths: []string{"model_name", "model_desc", "input_datasets", "output_datasets", "component_eval_params", "he_mode"},
		Attrs: []json.RawMessage{
			attr(map[string]string{"s": req.ModelName}),
			attr(map[string]string{"s": req.ModelDesc}),
			attr(map[string][]string{"ss": inputs}),
			attr(map[string][]string{"ss": outputs}),
			attr(map[string][]string{"ss": params}),
			attr(map[string]bool{"b": false}),
		},
	}, nil
}

// isIOComponent reports io/read_data, io/identity and io/write_data eval params.
func isIOComponent(evalParam string) bool {
	var p struct {
		Name   string `json:"name"`
		CompID string `json:"comp_id"`
	}
	_ = json.Unmarshal([]byte(evalParam), &p)
	name := p.Name
	if name == "" && p.CompID != "" {
		id := p.CompID
		if i := strings.Index(id, ":"); i >= 0 {
			id = id[:i]
		}
		if i := strings.LastIndex(id, "/"); i >= 0 {
			id = id[i+1:]
		}
		name = id
	}
	return name == "read_data" || name == "identity" || name == "write_data"
}

// stripCheckpoint removes checkpoint_uri from a persisted eval param JSON.
func stripCheckpoint(evalParam string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(evalParam), &m) != nil {
		return evalParam
	}
	if _, ok := m["checkpoint_uri"]; !ok {
		return evalParam
	}
	delete(m, "checkpoint_uri")
	b, err := json.Marshal(m)
	if err != nil {
		return evalParam
	}
	return string(b)
}

// --- model/status + job sync ---

func (s *ModelService) loadExport(ctx context.Context, jobID string) (*model.ProjectModelExportDO, *ModelExportDTO, error) {
	var rec model.ProjectModelExportDO
	if err := s.db.WithContext(ctx).Where("job_id = ?", jobID).First(&rec).Error; err != nil {
		return nil, nil, err
	}
	var dto ModelExportDTO
	if err := json.Unmarshal([]byte(rec.Payload), &dto); err != nil {
		return nil, nil, err
	}
	dto.Status, dto.ErrMsg = rec.Status, rec.ErrMsg
	return &rec, &dto, nil
}

func isExportFinished(status string) bool {
	return status == ModelExportSucceed || status == ModelExportFailed
}

// QueryModelExport mirrors ModelExportServiceImpl.queryModel.
func (s *ModelService) QueryModelExport(ctx context.Context, req *ModelExportStatusRequest) (*ModelExportDTO, error) {
	rec, dto, err := s.loadExport(ctx, req.JobID)
	if err != nil || (req.ProjectID != "" && rec.ProjectID != req.ProjectID) {
		return nil, padErr(errcode.ModelExportFailed, "job not found.")
	}
	if !isExportFinished(rec.Status) {
		// Refresh eagerly so the status does not depend on the sync loop cadence.
		if c := s.kc(dto.Initiator); c != nil {
			if resp, err := c.QueryJobDetail(ctx, req.JobID); err == nil && resp != nil && resp.Data.Status != nil {
				s.HandleJob(ctx, req.JobID, resp.Data.Status)
				if rec2, dto2, err := s.loadExport(ctx, req.JobID); err == nil {
					rec, dto = rec2, dto2
				}
			}
		}
	}
	if rec.Status == ModelExportSucceed {
		if err := s.savePack(ctx, dto); err != nil {
			return nil, err
		}
	}
	dto.Msg = dto.ErrMsg
	return dto, nil
}

// PendingJobIDs implements JobSyncHandler: unfinished model export jobs.
func (s *ModelService) PendingJobIDs(ctx context.Context) []string {
	// Drop finished records past retention (the Java cache eviction).
	s.db.WithContext(ctx).Unscoped().Where("status IN ? AND gmt_modified < ?",
		[]string{ModelExportSucceed, ModelExportFailed}, time.Now().Add(-modelExportRetention)).
		Delete(&model.ProjectModelExportDO{})
	var recs []model.ProjectModelExportDO
	s.db.WithContext(ctx).Where("status NOT IN ?", []string{ModelExportSucceed, ModelExportFailed}).Find(&recs)
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.JobID)
	}
	sort.Strings(ids)
	return ids
}

// HandleJob implements JobSyncHandler (Java JobManager.syncModelExportJob):
// SUCCEED writes the project_model_pack row, FAILED finishes the record.
func (s *ModelService) HandleJob(ctx context.Context, jobID string, detail *kuscia.JobStatusDetailInfo) bool {
	rec, dto, err := s.loadExport(ctx, jobID)
	if err != nil {
		return false
	}
	if isExportFinished(rec.Status) || detail == nil {
		return true
	}
	status, msg := rec.Status, rec.ErrMsg
	if !(graphjob.IsKusciaFinishedState(detail.State) && detail.EndTime == "") {
		if len(detail.Tasks) > 0 {
			status = exportStatusFromKuscia(detail.Tasks[0].State)
			msg = detail.Tasks[0].ErrMsg
		} else {
			status = exportStatusFromKuscia(detail.State)
			msg = detail.ErrMsg
		}
	}
	if status == ModelExportFailed && msg == "" {
		msg = detail.ErrMsg
	}
	if status == rec.Status && msg == rec.ErrMsg {
		return true
	}
	dto.Status, dto.ErrMsg = status, msg
	payload, _ := json.Marshal(dto)
	if err := s.db.WithContext(ctx).Model(&model.ProjectModelExportDO{}).Where("id = ?", rec.ID).
		Updates(map[string]any{"status": status, "err_msg": msg, "payload": string(payload)}).Error; err != nil {
		s.log.Warn("update model export status failed", zap.String("job_id", jobID), zap.Error(err))
		return true
	}
	if status == ModelExportSucceed {
		if err := s.savePack(ctx, dto); err != nil {
			s.log.Warn("save model pack failed", zap.String("job_id", jobID), zap.Error(err))
		}
	}
	return true
}

// exportStatusFromKuscia mirrors GraphNodeTaskStatus.formKusciaTaskStatus.
func exportStatusFromKuscia(state string) string {
	switch state {
	case "Succeeded":
		return ModelExportSucceed
	case "Failed":
		return ModelExportFailed
	case "Running":
		return ModelExportRunning
	default:
		return ModelExportInitialized
	}
}

// savePack mirrors ModelExportDTO.of + projectModelPackRepository.save (idempotent).
func (s *ModelService) savePack(ctx context.Context, dto *ModelExportDTO) error {
	var existing model.ProjectModelPackDO
	err := s.db.WithContext(ctx).Unscoped().Where("model_id = ?", dto.ModelID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	modelList, _ := json.Marshal(nonNilStrings(dto.ModelList))
	pds := dto.PartyDataSources
	if pds == nil {
		pds = []PartyDataSource{}
	}
	pdsJSON, _ := json.Marshal(pds)
	return s.db.WithContext(ctx).Create(&model.ProjectModelPackDO{
		ModelID:         dto.ModelID,
		Initiator:       dto.Initiator,
		ProjectID:       dto.ProjectID,
		ModelName:       dto.ModelName,
		ModelDesc:       dto.ModelDesc,
		ModelStats:      model.ModelStatsInit,
		SampleTables:    firstNonEmpty(dto.SampleTables, "{}"),
		TrainID:         dto.TrainID,
		ModelList:       string(modelList),
		GraphDetail:     dto.GraphDetail,
		ModelReportID:   dto.ModelReportID,
		ModelDatasource: string(pdsJSON),
	}).Error
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// --- model/modelPartyPath ---

// ModelPartyPath mirrors ModelExportServiceImpl.modelPartyPath.
func (s *ModelService) ModelPartyPath(ctx context.Context, req *ModelPartyPathRequest) ([]ModelPartyPathVO, error) {
	task, err := latestTask(ctx, s.db, req.ProjectID, req.GraphNodeID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, padErr(errcode.GraphNodeOutputNotExists)
	}
	parties := taskParties(task)
	if len(parties) == 0 {
		return nil, padErr(errcode.ModelExportFailed, "modelPartyPath result not found")
	}
	var job model.ProjectJobDO
	if s.db.WithContext(ctx).Where("job_id = ?", task.JobID).First(&job).Error != nil {
		return nil, padErr(errcode.ModelExportFailed, "job not found")
	}
	graphID := firstNonEmpty(job.GraphID, req.GraphID)
	names := s.nodeNames(ctx, parties)
	out := make([]ModelPartyPathVO, 0, len(parties))
	for _, p := range parties {
		name, ok := names[p]
		if !ok {
			return nil, padErr(errcode.ModelExportFailed, "node not found")
		}
		dsID, dsName := graphjob.DefaultDataSourceID, graphjob.DefaultDataSourceID
		var d model.ProjectGraphDomainDatasourceDO
		if s.db.WithContext(ctx).Where("project_id = ? AND graph_id = ? AND domain_id = ?", req.ProjectID, graphID, p).
			First(&d).Error == nil && d.DatasourceID != "" {
			dsID, dsName = d.DatasourceID, d.DatasourceName
		}
		out = append(out, ModelPartyPathVO{
			NodeID:   p,
			NodeName: name,
			DataSources: []ModelPartyDataSourceVO{{
				DataSourceID: dsID, DataSourceName: dsName, NodeID: p, Type: "LOCAL",
			}},
		})
	}
	return out, nil
}

var _ JobSyncHandler = (*ModelService)(nil)
