package votehandler

import (
	"context"
	"encoding/json"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/graphjob"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TeeDownloadHandler is Java TeeDownLoadMessageHandler: downloading a TEE
// result requires the approval of every other project node.
type TeeDownloadHandler struct{ base }

// NewTeeDownloadHandler creates the TEE_DOWNLOAD handler.
func NewTeeDownloadHandler(d *Deps) *TeeDownloadHandler { return &TeeDownloadHandler{base{d}} }

func (h *TeeDownloadHandler) Type() string       { return VoteTypeTeeDownload }
func (h *TeeDownloadHandler) signAtCreate() bool { return false }

func (h *TeeDownloadHandler) CreateApproval(ctx context.Context, initiator string, cfg *VoteConfig) (string, error) {
	return h.createApproval(ctx, h, initiator, cfg)
}

// GetVoter: TEE replies are signed with the voting node's own cert.
func (h *TeeDownloadHandler) GetVoter(nodeID string) string { return nodeID }

func (h *TeeDownloadHandler) preCheck(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) error {
	if !contains(h.projectNodeIDs(ctx, tx, cfg.ProjectID), initiator) {
		return bizErr(errcode.ProjectInstNotExists, "")
	}
	return nil
}

func (h *TeeDownloadHandler) createVoteConfig(ctx context.Context, tx *gorm.DB, voteID, _ string, cfg *VoteConfig) error {
	return tx.Create(&model.TeeDownloadApprovalConfigDO{
		VoteID: voteID, TaskID: cfg.TaskID, JobID: cfg.JobID, ResourceID: cfg.ResourceID,
		ResourceType: cfg.ResourceType, ProjectID: cfg.ProjectID, GraphID: cfg.GraphID,
		AllParticipants: toJSON(nonNil(h.projectNodeIDs(ctx, tx, cfg.ProjectID))),
	}).Error
}

func (h *TeeDownloadHandler) voters(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) ([]string, error) {
	return without(h.projectNodeIDs(ctx, tx, cfg.ProjectID), initiator), nil
}

func (h *TeeDownloadHandler) executors(major string, _ *VoteConfig) []string { return []string{major} }

func (h *TeeDownloadHandler) approvedAction(_ context.Context, _ *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	return "TEE_DOWNLOAD," + cfg.ResourceID, nil
}

func (h *TeeDownloadHandler) rejectAction(context.Context, *gorm.DB, string, *VoteConfig) (string, error) {
	return "NONE", nil
}

func (h *TeeDownloadHandler) requestDesc(_ context.Context, _ *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	return cfg.ResourceID, nil
}

func (h *TeeDownloadHandler) inviteDesc(ctx context.Context, tx *gorm.DB, i string, cfg *VoteConfig) (string, error) {
	return h.requestDesc(ctx, tx, i, cfg)
}

func (h *TeeDownloadHandler) config(ctx context.Context, voteID string) (*model.TeeDownloadApprovalConfigDO, error) {
	var c model.TeeDownloadApprovalConfigDO
	if err := h.d.DB.WithContext(ctx).Where("vote_id = ?", voteID).First(&c).Error; err != nil {
		return nil, bizErr(errcode.VoteNotExists, "")
	}
	return &c, nil
}

// GetVoteMessageDetail mirrors TeeDownLoadMessageHandler.getVoteMessageDetail.
func (h *TeeDownloadHandler) GetVoteMessageDetail(ctx context.Context, _ bool, nodeID, voteID string) (interface{}, error) {
	vr, err := h.findRequest(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	cfg, err := h.config(ctx, voteID)
	if err != nil {
		return nil, err
	}
	db := h.d.DB.WithContext(ctx)
	fedTableID := vr.Description
	var columns []TableColumnVO
	if cfg.ResourceType == resultKindFedTable {
		var task model.ProjectJobTaskDO
		if err := db.Where("project_id = ? AND job_id = ? AND task_id = ?", cfg.ProjectID, cfg.JobID, cfg.TaskID).
			First(&task).Error; err != nil {
			return nil, bizErr(errcode.ProjectJobNotExists, "")
		}
		parties := parseStringList(task.Parties)
		if len(parties) != 1 {
			return nil, bizErr(errcode.SystemError, "tee task parties must single party")
		}
		nodeID = parties[0]
		var pd model.ProjectDatatableDO
		if err := db.Where("project_id = ? AND node_id = ? AND datatable_id = ?", cfg.ProjectID, nodeID, fedTableID).
			First(&pd).Error; err != nil {
			return nil, bizErr(errcode.ProjectDatatableNotExists, "fedTable not exists")
		}
		columns = []TableColumnVO{}
		for _, c := range graphjob.ParseColumnConfigs(pd.TableConfigs) {
			columns = append(columns, TableColumnVO{ColName: c.ColName, ColType: c.ColType, ColComment: c.ColComment})
		}
	}
	p, err := h.openProject(ctx, nil, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	// Java: graphDetailVO = projectService.getProjectJob(projectId, jobId).getGraph()
	// (fails with PROJECT_JOB_NOT_EXISTS when the job is gone); graphName is its name.
	job, err := service.NewJobService(nil, nil, nil, nil, nil, nil, h.d.DB).
		GetJob(ctx, &service.GetJobRequest{ProjectID: cfg.ProjectID, JobID: cfg.JobID})
	if err != nil {
		return nil, err
	}
	graph := job.Graph
	parties, err := h.GetPartyStatusByVoteID(ctx, voteID)
	if err != nil {
		return nil, err
	}
	out := &TeeDownloadMessageDetail{
		PartyVoteStatuses: parties,
		GraphName:         graph.Name,
		GraphDetailVO:     graph,
		TableColumns:      columns,
		Project: ProjectSimpleVO{ProjectName: p.Name, GmtCreated: rfc3339(p.GmtCreate),
			ComputeMode: p.ComputeMode, Description: p.Description},
		TaskID:     cfg.TaskID,
		ResourceID: cfg.ResourceID,
		Parties:    []string{nodeID},
	}
	out.MessageName = fedTableID
	out.Status = VoteStatusName(vr.Status)
	out.Type = VoteTypeTeeDownload
	return out, nil
}

func (h *TeeDownloadHandler) GetMessageListNecessaryInfo(ctx context.Context, voteID string) (interface{}, error) {
	cfg, err := h.config(ctx, voteID)
	if err != nil {
		return nil, err
	}
	return &TeeListCustomizedMessage{ProjectID: cfg.ProjectID}, nil
}

func (h *TeeDownloadHandler) GetPartyStatusByVoteID(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	return h.partyStatusByInvites(ctx, voteID)
}

func (h *TeeDownloadHandler) DoCallBackRejected(ctx context.Context, vr *model.VoteRequestDO) {
	h.doCallBackRejectedDefault(ctx, vr)
}

// DoCallBackApproved builds the VoteResult proof and, on CENTER, pulls the
// TEE result (Java doDownLoad); other platforms only record success.
func (h *TeeDownloadHandler) DoCallBackApproved(ctx context.Context, vr *model.VoteRequestDO) {
	var reqMsg VoteRequestMessage
	_ = json.Unmarshal([]byte(vr.RequestMsg), &reqMsg)
	var invites []model.VoteInviteDO
	h.d.DB.WithContext(ctx).Where("vote_id = ?", vr.VoteID).Order("id").Find(&invites)
	result := VoteResult{VoteRequest: reqMsg, VoteInvite: []VoteReplyMessage{}}
	for _, inv := range invites {
		var r VoteReplyMessage
		_ = json.Unmarshal([]byte(inv.VoteMsg), &r)
		result.VoteInvite = append(result.VoteInvite, r)
	}
	cfg, err := h.config(ctx, vr.VoteID)
	if err != nil {
		h.failed(ctx, vr, "tee download approval config not found")
		return
	}
	if !h.d.isCenter() || h.d.TeePuller == nil {
		if h.d.isCenter() {
			h.d.log().Info("tee result puller not configured, approval recorded only", zap.String("vote", vr.VoteID))
		}
		h.success(ctx, vr)
		return
	}
	if err := h.d.TeePuller.PullResultFromTeeNode(ctx, vr.Initiator, cfg.ResourceID, toJSON(result),
		cfg.ProjectID, cfg.JobID, cfg.TaskID, cfg.ResourceType); err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	h.success(ctx, vr)
}
