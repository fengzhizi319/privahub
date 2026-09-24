package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

// TEE data-management job constants (Java TeeJobConstants / JobConstants).
const (
	TeeJobKindPushAuth   = "PushAuth"
	TeeJobKindPush       = "Push"
	TeeJobKindAuth       = "Auth"
	TeeJobKindDelete     = "Delete"
	TeeJobKindPull       = "Pull"
	TeeJobKindCancelAuth = "CancelAuth"

	TeeJobStatusRunning = "RUNNING"
	TeeJobStatusSuccess = "SUCCESS"
	TeeJobStatusFailed  = "FAILED"

	teeImage                 = "tee-dm-image"
	teeDataManagement        = "data_management"
	teeVersion               = "0.0.1"
	teePushName              = "upload"
	teeDeleteName            = "delete"
	teePushAttrPathDomain    = "uploader/domain_id"
	teePushAttrPathCert      = "uploader/cert_chain"
	teeDeleteAttrPathDomain  = "deleter/domain_id"
	teePushInputName         = "uploader_input"
	teeDeleteInputName       = "deleter_input"
	teePushInputType         = "sf.table.individual"
	teeDataRef               = "dm://input?id="
	teeOutputDatasourceRef   = "dm://output?datasource_id="
	teeOutputIDRef           = "&id="
	teeOutputRelativeURIRef  = "&uri="
	teeDefaultDS             = "default-data-source"
	teeOperateRelativeURI    = "relativeUri"
	teeOperateGrantID        = "domaindatagrant_id"
	teeOperatePushToTeeJobID = "pushToTeeJobId"
	teeOperateProjectID      = "project_id"
	teeOperateVoteResult     = "vote_result"

	teeAuthName              = "authorize"
	teePullName              = "download"
	teeAuthAttrOwnerDomain   = "owner/domain_id"
	teeAuthAttrAuthDomains   = "authorization_info/domain_ids"
	teeAuthAttrRootCerts     = "authorization_info/root_certs"
	teeAuthAttrColumns       = "authorization_info/columns"
	teeAuthAttrProjectID     = "authorization_info/project_id"
	teePullAttrPathDomain    = "receiver/domain_id"
	teePullAttrVoteResult    = "receiver/vote_result"
	teeAuthInputName         = "authorize_input"
	teePullInputName         = "downloader_input"
	teeOperateProjectJobID   = "projectJobId"
	teeOperateProjectTaskID  = "projectJobTaskId"
	teeOperateResultType     = "resultType"
	teeOperateAuthProjectJob = "authToTeeProjectJobId"
)

// TeeJob is Java TeeJob: everything needed to render a TEE data-management
// Kuscia job.
type TeeJob struct {
	NodeID        string
	NodeCert      string
	AuthNodeIDs   []string
	AuthNodeCerts []string
	TeeNodeID     string
	DatatableID   string
	JobID         string
	Kind          string
	ProjectID     string
	DatasourceID  string
	RelativeURI   string
	VoteResult    string
	Parties       []string
}

// TeeJobConverter mirrors Java KusciaTeeDataManagerConverter.
type TeeJobConverter struct {
	// CapsuleManager is tee.capsule-manager; '#' is replaced by the tee domain.
	CapsuleManager string
	MaxParallelism int32
}

// NewTeeJobConverter creates a converter with Java defaults.
func NewTeeJobConverter(capsuleManager string, maxParallelism int32) *TeeJobConverter {
	if capsuleManager == "" {
		capsuleManager = "capsule-manager.#.svc"
	}
	if maxParallelism <= 0 {
		maxParallelism = 1
	}
	return &TeeJobConverter{CapsuleManager: capsuleManager, MaxParallelism: maxParallelism}
}

// BuildTeeDatatableID mirrors Java buildTeeDatatableId: "<nodeId>-<datatableId>".
func BuildTeeDatatableID(nodeID, datatableID string) string {
	return nodeID + "-" + datatableID
}

// operateInfoMap decodes TeeNodeDatatableManagementDO.operate_info.
func operateInfoMap(s string) map[string]interface{} {
	m := map[string]interface{}{}
	if strings.TrimSpace(s) == "" {
		return m
	}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

func operateInfoString(s, key string) string {
	if v, ok := operateInfoMap(s)[key]; ok && v != nil {
		if str, ok := v.(string); ok {
			return str
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	return ""
}

// GenTeeJob mirrors Java TeeJob.genTeeJob.
func GenTeeJob(m *model.TeeNodeDatatableManagementDO, parties []string, cert string, authNodeIDs, authNodeCerts []string) *TeeJob {
	return &TeeJob{
		ProjectID:     operateInfoString(m.OperateInfo, teeOperateProjectID),
		NodeID:        m.NodeID,
		NodeCert:      cert,
		AuthNodeIDs:   authNodeIDs,
		AuthNodeCerts: authNodeCerts,
		DatatableID:   m.DatatableID,
		TeeNodeID:     m.TeeNodeID,
		JobID:         m.JobID,
		Kind:          m.Kind,
		DatasourceID:  m.DatasourceID,
		RelativeURI:   operateInfoString(m.OperateInfo, teeOperateRelativeURI),
		VoteResult:    operateInfoString(m.OperateInfo, teeOperateVoteResult),
		Parties:       parties,
	}
}

// secretflow spec JSON shapes (proto field names, as Java ProtoUtils.toJsonString).
type teeAttr struct {
	S  string   `json:"s,omitempty"`
	SS []string `json:"ss,omitempty"`
}

type teeDataRefJSON struct {
	URI string `json:"uri"`
}

type teeDistData struct {
	Name     string           `json:"name"`
	Type     string           `json:"type"`
	DataRefs []teeDataRefJSON `json:"data_refs"`
}

type teeNodeEvalParam struct {
	Version    string        `json:"version"`
	CompID     string        `json:"comp_id"`
	AttrPaths  []string      `json:"attr_paths,omitempty"`
	Attrs      []teeAttr     `json:"attrs,omitempty"`
	Inputs     []teeDistData `json:"inputs,omitempty"`
	OutputURIs []string      `json:"output_uris,omitempty"`
}

type teeTaskConfig struct {
	Scope                  string `json:"scope,omitempty"`
	CapsuleManagerEndpoint string `json:"capsule_manager_endpoint,omitempty"`
}

type teeDatasourceConfig struct {
	ID string `json:"id"`
}

type teeTaskInputConfig struct {
	SfDatasourceConfig map[string]teeDatasourceConfig `json:"sf_datasource_config,omitempty"`
	SfNodeEvalParam    *teeNodeEvalParam              `json:"sf_node_eval_param,omitempty"`
	TeeTaskConfig      teeTaskConfig                  `json:"tee_task_config"`
}

func teeBuildURI(datatableID, prefix string) string {
	if prefix == "" {
		return teeDataRef + datatableID
	}
	return teeDataRef + strings.ReplaceAll(datatableID, prefix, "")
}

func teeBuildOutputURI(j *TeeJob) string {
	s := teeOutputDatasourceRef + j.DatasourceID + teeOutputIDRef + j.DatatableID
	if strings.TrimSpace(j.RelativeURI) != "" {
		s += teeOutputRelativeURIRef + j.RelativeURI
	}
	return s
}

// RenderTaskInputConfig renders the TEE task_input_config JSON (Java renderTaskInputConfig).
func (c *TeeJobConverter) RenderTaskInputConfig(j *TeeJob) string {
	cfg := teeTaskInputConfig{
		SfDatasourceConfig: map[string]teeDatasourceConfig{},
		TeeTaskConfig: teeTaskConfig{
			CapsuleManagerEndpoint: strings.ReplaceAll(c.CapsuleManager, "#", j.TeeNodeID),
		},
	}
	for _, p := range j.Parties {
		cfg.SfDatasourceConfig[p] = teeDatasourceConfig{ID: teeDefaultDS}
	}
	compID := func(name string) string { return teeDataManagement + "/" + name + ":" + teeVersion }
	switch j.Kind {
	case TeeJobKindPush:
		cfg.SfNodeEvalParam = &teeNodeEvalParam{
			Version:   teeVersion,
			CompID:    compID(teePushName),
			AttrPaths: []string{teePushAttrPathDomain, teePushAttrPathCert},
			Attrs:     []teeAttr{{S: j.NodeID}, {}},
			Inputs: []teeDistData{{
				Name: teePushInputName, Type: teePushInputType,
				DataRefs: []teeDataRefJSON{{URI: teeBuildURI(j.DatatableID, j.TeeNodeID+"-")}},
			}},
			OutputURIs: []string{teeBuildOutputURI(j)},
		}
	case TeeJobKindAuth:
		authIDs := j.AuthNodeIDs
		if authIDs == nil {
			authIDs = []string{}
		}
		authCerts := j.AuthNodeCerts
		if authCerts == nil {
			authCerts = []string{}
		}
		cfg.SfNodeEvalParam = &teeNodeEvalParam{
			Version:   teeVersion,
			CompID:    compID(teeAuthName),
			AttrPaths: []string{teeAuthAttrOwnerDomain, teeAuthAttrAuthDomains, teeAuthAttrRootCerts, teeAuthAttrColumns, teeAuthAttrProjectID},
			Attrs:     []teeAttr{{S: j.NodeID}, {SS: authIDs}, {SS: authCerts}, {SS: []string{"*"}}, {S: j.ProjectID}},
			Inputs: []teeDistData{{
				Name: teeAuthInputName, Type: teePushInputType,
				DataRefs: []teeDataRefJSON{{URI: teeBuildURI(j.DatatableID, "")}},
			}},
		}
		cfg.TeeTaskConfig.Scope = j.ProjectID
	case TeeJobKindPull:
		cfg.SfNodeEvalParam = &teeNodeEvalParam{
			Version:   teeVersion,
			CompID:    compID(teePullName),
			AttrPaths: []string{teePullAttrPathDomain, teePullAttrVoteResult},
			Attrs:     []teeAttr{{S: j.NodeID}, {S: j.VoteResult}},
			Inputs: []teeDistData{{
				Name: teePullInputName, Type: teePushInputType,
				DataRefs: []teeDataRefJSON{{URI: teeBuildURI(j.DatatableID, j.NodeID+"-")}},
			}},
			OutputURIs: []string{teeBuildOutputURI(j)},
		}
	case TeeJobKindDelete:
		cfg.SfNodeEvalParam = &teeNodeEvalParam{
			Version:   teeVersion,
			CompID:    compID(teeDeleteName),
			AttrPaths: []string{teeDeleteAttrPathDomain},
			Attrs:     []teeAttr{{S: j.NodeID}},
			Inputs: []teeDistData{{
				Name: teeDeleteInputName, Type: teePushInputType,
				DataRefs: []teeDataRefJSON{{URI: teeBuildURI(j.DatatableID, "")}},
			}},
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(cfg)
	return strings.TrimSpace(buf.String())
}

// Convert builds the Kuscia CreateJob request (Java converter(TeeJob)).
func (c *TeeJobConverter) Convert(j *TeeJob) *kuscia.CreateJobRequest {
	taskID := j.JobID + "-" + j.NodeID // JobUtils.genTaskId(jobId, nodeId)
	parties := make([]kuscia.Party, 0, len(j.Parties))
	for _, p := range j.Parties {
		parties = append(parties, kuscia.Party{DomainID: p})
	}
	initiator := ""
	if len(j.Parties) > 0 {
		initiator = j.Parties[0]
	}
	return &kuscia.CreateJobRequest{
		JobID:          j.JobID,
		Initiator:      initiator,
		MaxParallelism: c.MaxParallelism,
		Tasks: []kuscia.TaskConfig{{
			TaskID:      taskID,
			Alias:       taskID,
			AppImage:    teeImage,
			Parties:     parties,
			InputConfig: c.RenderTaskInputConfig(j),
		}},
	}
}

// --- TEE job status sync (Java JobManager.syncTeeJob, polling instead of watch) ---

// SyncTeeJobs polls Kuscia for RUNNING TEE data-management jobs and updates
// tee_node_datatable_management. A successful Delete job soft-deletes the
// Push record it removed (Java behaviour).
func (s *DatatableService) SyncTeeJobs(ctx context.Context) {
	if s.kusciaClient == nil || s.db == nil {
		return
	}
	var running []model.TeeNodeDatatableManagementDO
	if err := s.db.WithContext(ctx).Where("status = ?", TeeJobStatusRunning).Find(&running).Error; err != nil {
		return
	}
	for i := range running {
		m := &running[i]
		resp, err := s.kusciaClient.QueryJob(ctx, m.JobID)
		if err != nil || resp == nil || resp.Data.Status == nil {
			continue
		}
		state := strings.ToLower(resp.Data.Status.State)
		switch state {
		case "succeeded":
			m.Status = TeeJobStatusSuccess
		case "failed", "cancelled":
			m.Status = TeeJobStatusFailed
			m.ErrMsg = resp.Data.Status.ErrMsg
			for _, t := range resp.Data.Status.Tasks {
				if t.ErrMsg != "" {
					m.ErrMsg = t.ErrMsg
				}
			}
		default:
			// Java syncTeeResult runs on every job event: a RUNNING pull
			// already exposes its (pending) result.
			if m.Kind == TeeJobKindPull {
				_ = SyncTeePullResult(ctx, s.db, m)
			}
			continue
		}
		if err := s.db.WithContext(ctx).Model(&model.TeeNodeDatatableManagementDO{}).Where("id = ?", m.ID).
			Updates(map[string]interface{}{"status": m.Status, "err_msg": m.ErrMsg}).Error; err != nil {
			continue
		}
		if m.Kind == TeeJobKindPull {
			if err := SyncTeePullResult(ctx, s.db, m); err != nil {
				s.log.Warn("sync tee pull result failed", zap.String("job_id", m.JobID), zap.Error(err))
			}
		}
		// Java syncTeeJob: a finished Delete / CancelAuth job removes the Push
		// / Auth record it undid (the Java code does not check the state).
		var undone string
		switch m.Kind {
		case TeeJobKindDelete:
			undone = operateInfoString(m.OperateInfo, teeOperatePushToTeeJobID)
		case TeeJobKindCancelAuth:
			undone = operateInfoString(m.OperateInfo, teeOperateAuthProjectJob)
		}
		if undone != "" && m.Status == TeeJobStatusSuccess {
			s.db.WithContext(ctx).Where("job_id = ?", undone).Delete(&model.TeeNodeDatatableManagementDO{})
		}
	}
}

// SyncTeePullResult mirrors JobManager.syncTeeResult: a Pull job writes the
// pulled DomainData into project_result (node = receiver, ref = pulled id) so
// it is listed in node/result/list and project results.
func SyncTeePullResult(ctx context.Context, db *gorm.DB, m *model.TeeNodeDatatableManagementDO) error {
	if m == nil || m.Kind != TeeJobKindPull {
		return nil
	}
	projectID := operateInfoString(m.OperateInfo, teeOperateProjectID)
	if projectID == "" {
		return nil
	}
	kind := graphjobResultKind(operateInfoString(m.OperateInfo, teeOperateResultType))
	return upsertResult(ctx, db, &model.ProjectResultDO{
		ProjectID: projectID, Kind: kind, NodeID: m.NodeID, RefID: m.DatatableID,
		JobID:  operateInfoString(m.OperateInfo, teeOperateProjectJobID),
		TaskID: operateInfoString(m.OperateInfo, teeOperateProjectTaskID),
	})
}

// StartTeeJobSync starts the background TEE job status poller.
func (s *DatatableService) StartTeeJobSync(interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	s.stopOnce = make(chan struct{})
	stop := s.stopOnce
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							s.log.Error("tee job sync panic", zap.Any("panic", r))
						}
					}()
					ctx, cancel := context.WithTimeout(context.Background(), interval)
					defer cancel()
					s.SyncTeeJobs(ctx)
				}()
			}
		}
	}()
}

// Stop stops the background TEE job poller.
func (s *DatatableService) Stop() {
	if s.stopOnce != nil {
		close(s.stopOnce)
		s.stopOnce = nil
	}
}

// saveTeeManagement persists a TEE management record (Java
// saveTeeNodeDatatableManagementOrPush): on EDGE the record is not written
// locally but pushed to the center through vote_sync
// (DbSyncUtil.dbDataSyncToCenter, syncDataType TEE_NODE_DATATABLE_MANAGEMENT);
// the center stores it and syncs it back over SSE. Elsewhere it is saved.
func saveTeeManagement(tx *gorm.DB, m *model.TeeNodeDatatableManagementDO) error {
	if GlobalEnv().IsEdge() {
		ctx := tx.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}
		now := time.Now()
		if m.GmtCreate.IsZero() {
			m.GmtCreate = now
		}
		if m.GmtModified.IsZero() {
			m.GmtModified = now
		}
		return CurrentDbSyncSender().Send(ctx, "", SyncData{
			TableName: "tee_node_datatable_management", Action: SyncActionCreate, Data: m,
		})
	}
	return tx.Create(m).Error
}
