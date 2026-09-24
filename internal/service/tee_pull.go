package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// TEE result pull and datatable authorization jobs (Java
// DatatableServiceImpl.pullResultFromTeeNode and the TEE branch of
// ProjectServiceImpl.addDatatableToProject).

// systemDomainID is Java SYSTEM_DOMAIN_ID (the Kuscia master domain).
const systemDomainID = "kuscia-system"

// graphjobResultKind maps a TEE download resource type (a DomainData type
// such as "table", or a ResultKind name) to a project_result kind.
func graphjobResultKind(resourceType string) string {
	if k := graphjob.ResultKindFromDatatable(resourceType); k != "" {
		return k
	}
	for _, k := range []string{graphjob.ResultFedTable, graphjob.ResultModel, graphjob.ResultRule, graphjob.ResultReport, graphjob.ResultReadData} {
		if strings.EqualFold(k, resourceType) {
			return k
		}
	}
	return graphjob.ResultFedTable
}

// TeePullRequest is the input of TeePullService.PullResult (Java
// pullResultFromTeeNode arguments).
type TeePullRequest struct {
	// NodeID is the receiver (vote initiator) node.
	NodeID string
	// ResourceID is the TEE side result id ("{jobId}-{output}").
	ResourceID string
	// TeeNodeID defaults to the configured tee.domain-id.
	TeeNodeID string
	// DatasourceID defaults to default-data-source.
	DatasourceID string
	// RelativeURI defaults to the pulled datatable id.
	RelativeURI string
	// VoteResult is the JSON approval result checked by the TEE capsule manager.
	VoteResult   string
	ProjectID    string
	ProjectJobID string
	TaskID       string
	// ResourceType is the result type (DomainData type / ResultKind).
	ResourceType string
}

// TeePullService pulls TEE results once a TEE_DOWNLOAD approval passes. It
// implements votehandler.TeeResultPuller (PullResultFromTeeNode).
type TeePullService struct {
	ds *DatatableService
}

// NewTeePullService creates a TeePullService on top of the datatable service
// (TEE converter, Kuscia pool, tee.domain-id).
func NewTeePullService(ds *DatatableService) *TeePullService {
	return &TeePullService{ds: ds}
}

// PullResult mirrors DatatableServiceImpl.pullResultFromTeeNode: it saves a
// RUNNING Pull record ("{nodeId}-{resourceId}") and creates the TEE download
// job; SyncTeeJobs later updates the status and writes project_result.
func (p *TeePullService) PullResult(ctx context.Context, req *TeePullRequest) (*model.TeeNodeDatatableManagementDO, error) {
	s := p.ds
	if s == nil {
		return nil, bizErr(errcode.KusciaRpcError, "datatable service not configured")
	}
	if req.NodeID == "" || req.ResourceID == "" {
		return nil, bizErr(errcode.ParamError, "nodeId/resourceId cannot be blank")
	}
	if err := s.requireKuscia(); err != nil {
		return nil, err
	}
	teeDomain := s.teeDomain(req.TeeNodeID)
	dsID := firstNonEmpty(req.DatasourceID, DefaultLocalDatasourceID)
	datatableID := BuildTeeDatatableID(req.NodeID, req.ResourceID)
	relativeURI := firstNonEmpty(req.RelativeURI, datatableID)
	info, _ := json.Marshal(map[string]string{
		teeOperateRelativeURI:   relativeURI,
		teeOperateVoteResult:    req.VoteResult,
		teeOperateProjectID:     req.ProjectID,
		teeOperateProjectJobID:  req.ProjectJobID,
		teeOperateProjectTaskID: req.TaskID,
		teeOperateResultType:    req.ResourceType,
	})
	pull := &model.TeeNodeDatatableManagementDO{
		NodeID: req.NodeID, DatatableID: datatableID, TeeNodeID: teeDomain, JobID: randomLetters(4),
		DatasourceID: dsID, Status: TeeJobStatusRunning, Kind: TeeJobKindPull, OperateInfo: string(info),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := saveTeeManagement(tx, pull); err != nil {
			return err
		}
		job := GenTeeJob(pull, []string{req.NodeID, teeDomain}, "", nil, nil)
		if _, err := s.kc(req.NodeID).CreateJob(ctx, s.teeConverter.Convert(job)); err != nil {
			return bizWrap(errcode.DatatableCreateTeeJobFailed, err, "")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("tee pull job created", zap.String("node", req.NodeID), zap.String("datatable", datatableID), zap.String("job", pull.JobID))
	return pull, nil
}

// PullResultFromTeeNode is the votehandler.TeeResultPuller entry used by the
// TEE_DOWNLOAD approved callback (Java TeeDownLoadMessageHandler.doDownLoad).
func (p *TeePullService) PullResultFromTeeNode(ctx context.Context, nodeID, resourceID, voteResult, projectID, jobID, taskID, resourceType string) error {
	_, err := p.PullResult(ctx, &TeePullRequest{
		NodeID: nodeID, ResourceID: resourceID, VoteResult: voteResult,
		ProjectID: projectID, ProjectJobID: jobID, TaskID: taskID, ResourceType: resourceType,
	})
	return err
}

// TeePullStatus returns the latest Pull record of nodeID for a pulled
// datatable id (NodeServiceImpl.getPullFromTeeInfos); nil when none.
func TeePullStatus(ctx context.Context, db *gorm.DB, nodeID, teeNodeID, datatableID string) *model.TeeNodeDatatableManagementDO {
	q := db.WithContext(ctx).Where("node_id = ? AND datatable_id = ? AND kind = ?", nodeID, datatableID, TeeJobKindPull)
	if teeNodeID != "" {
		q = q.Where("tee_node_id = ?", teeNodeID)
	}
	var m model.TeeNodeDatatableManagementDO
	if q.Order("gmt_create DESC").Order("id DESC").First(&m).Error != nil {
		return nil
	}
	return &m
}

// AuthDatatableToTee mirrors the TEE branch of ProjectServiceImpl.addDatatableToProject:
// check the two-way route with the TEE node, push the datatable when it was
// never pushed (or the last push failed), then save a RUNNING Auth record and
// create the TEE "authorize" job granting the project scope to kuscia-system.
func (s *DatatableService) AuthDatatableToTee(ctx context.Context, projectID, nodeID, datatableID, teeNodeID, datasourceID string) (*model.TeeNodeDatatableManagementDO, error) {
	if err := s.requireKuscia(); err != nil {
		return nil, err
	}
	teeDomain := s.teeDomain(teeNodeID)
	dsID := firstNonEmpty(datasourceID, DefaultLocalDatasourceID)
	if !s.routeExists(ctx, nodeID, teeDomain) {
		return nil, bizErr(errcode.NodeRouteNotExistError, fmt.Sprintf("route not exist %s->%s", nodeID, teeDomain))
	}
	if !s.routeExists(ctx, teeDomain, nodeID) {
		return nil, bizErr(errcode.NodeRouteNotExistError, fmt.Sprintf("route not exist %s->%s", teeDomain, nodeID))
	}
	teeDatatableID := BuildTeeDatatableID(teeDomain, datatableID)
	var push model.TeeNodeDatatableManagementDO
	err := s.db.WithContext(ctx).Where("node_id = ? AND tee_node_id = ? AND datatable_id = ? AND kind = ?",
		nodeID, teeDomain, teeDatatableID, TeeJobKindPush).Order("id DESC").First(&push).Error
	if err != nil || push.Status == TeeJobStatusFailed {
		if perr := s.PushDatatableToTee(ctx, &PushDatatableToTeeRequest{NodeID: nodeID, DatatableID: datatableID}); perr != nil {
			return nil, perr
		}
	}
	info, _ := json.Marshal(map[string]string{teeOperateProjectID: projectID})
	auth := &model.TeeNodeDatatableManagementDO{
		NodeID: nodeID, DatatableID: teeDatatableID, TeeNodeID: teeDomain, JobID: randomLetters(4),
		DatasourceID: dsID, Status: TeeJobStatusRunning, Kind: TeeJobKindAuth, OperateInfo: string(info),
	}
	systemCert := ""
	if resp, qerr := s.kc(systemDomainID).QueryDomain(ctx, systemDomainID); qerr == nil && resp != nil {
		systemCert = resp.Data.Cert
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := saveTeeManagement(tx, auth); err != nil {
			return err
		}
		job := GenTeeJob(auth, []string{nodeID}, "", []string{systemDomainID}, []string{systemCert})
		if _, err := s.kc(nodeID).CreateJob(ctx, s.teeConverter.Convert(job)); err != nil {
			return bizWrap(errcode.DatatableCreateTeeJobFailed, err, "")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return auth, nil
}

// TeeDatatableAuthorizer is the TEE hook of ProjectService.AddDatatable.
type TeeDatatableAuthorizer interface {
	AuthDatatableToTee(ctx context.Context, projectID, nodeID, datatableID, teeNodeID, datasourceID string) (*model.TeeNodeDatatableManagementDO, error)
}

var _ TeeDatatableAuthorizer = (*DatatableService)(nil)
