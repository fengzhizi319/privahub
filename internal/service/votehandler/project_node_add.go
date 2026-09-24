package votehandler

import (
	"context"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"gorm.io/gorm"
)

// ProjectNodeAddHandler handles PROJECT_NODE_ADD. Java ships only an empty
// stub (every hook returns null); the Go version implements the obvious
// semantics: voteConfig {projectId, inviteNodeId}; the other project
// parties (institutions in AUTONOMY, nodes otherwise) vote; on approval the
// node (and in AUTONOMY its institution) joins the project.
type ProjectNodeAddHandler struct{ base }

// NewProjectNodeAddHandler creates the PROJECT_NODE_ADD handler.
func NewProjectNodeAddHandler(d *Deps) *ProjectNodeAddHandler { return &ProjectNodeAddHandler{base{d}} }

func (h *ProjectNodeAddHandler) Type() string       { return VoteTypeProjectNodeAdd }
func (h *ProjectNodeAddHandler) signAtCreate() bool { return false }

func (h *ProjectNodeAddHandler) CreateApproval(ctx context.Context, initiator string, cfg *VoteConfig) (string, error) {
	return h.createApproval(ctx, h, initiator, cfg)
}

func (h *ProjectNodeAddHandler) preCheck(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) error {
	if _, err := h.openProject(ctx, tx, cfg.ProjectID); err != nil {
		return err
	}
	if cfg.InviteNodeID == "" {
		return bizErr(errcode.ParamError, "inviteNodeId is required")
	}
	var n int64
	tx.Model(&model.NodeDO{}).Where("node_id = ?", cfg.InviteNodeID).Count(&n)
	if n == 0 {
		return bizErr(errcode.NodeNotExistError, cfg.InviteNodeID)
	}
	return nil
}

func (h *ProjectNodeAddHandler) createVoteConfig(_ context.Context, tx *gorm.DB, voteID, initiator string, cfg *VoteConfig) error {
	return tx.Create(&model.ProjectApprovalConfigDO{
		VoteID: voteID, Initiator: initiator, Type: VoteTypeProjectNodeAdd,
		ProjectID: cfg.ProjectID, InviteNodeID: cfg.InviteNodeID,
	}).Error
}

func (h *ProjectNodeAddHandler) voters(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) ([]string, error) {
	if h.d.isAutonomy() {
		var insts []string
		tx.Model(&model.ProjectInstDO{}).Where("project_id = ?", cfg.ProjectID).Order("id").Pluck("inst_id", &insts)
		return without(insts, initiator), nil
	}
	return without(h.projectNodeIDs(ctx, tx, cfg.ProjectID), initiator), nil
}

func (h *ProjectNodeAddHandler) executors(major string, _ *VoteConfig) []string {
	return []string{major}
}

func (h *ProjectNodeAddHandler) approvedAction(_ context.Context, _ *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	return VoteTypeProjectNodeAdd + "," + cfg.ProjectID + "," + cfg.InviteNodeID, nil
}

func (h *ProjectNodeAddHandler) rejectAction(context.Context, *gorm.DB, string, *VoteConfig) (string, error) {
	return "NONE", nil
}

func (h *ProjectNodeAddHandler) requestDesc(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	p, err := h.openProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return "", err
	}
	return p.Name, nil
}

func (h *ProjectNodeAddHandler) inviteDesc(ctx context.Context, tx *gorm.DB, i string, cfg *VoteConfig) (string, error) {
	return h.requestDesc(ctx, tx, i, cfg)
}

func (h *ProjectNodeAddHandler) config(ctx context.Context, voteID string) (*model.ProjectApprovalConfigDO, error) {
	var c model.ProjectApprovalConfigDO
	if err := h.d.DB.WithContext(ctx).Where("vote_id = ?", voteID).First(&c).Error; err != nil {
		return nil, bizErr(errcode.VoteNotExists, "")
	}
	return &c, nil
}

func (h *ProjectNodeAddHandler) GetVoteMessageDetail(ctx context.Context, _ bool, _, voteID string) (interface{}, error) {
	vr, err := h.findRequest(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	cfg, err := h.config(ctx, voteID)
	if err != nil {
		return nil, err
	}
	p, err := h.openProject(ctx, nil, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	statuses, err := h.GetPartyStatusByVoteID(ctx, voteID)
	if err != nil {
		return nil, err
	}
	out := &ProjectCreateMessageDetail{
		InitiatorID: vr.Initiator, ProjectName: p.Name, PartyVoteStatuses: statuses,
		ComputeMode: p.ComputeMode, ComputeFunc: p.ComputeFunc, ProjectDesc: p.Description,
		GmtCreated: rfc3339(p.GmtCreate),
		ParticipantNodeInstVOS: []ParticipantNodeInstVO{{InitiatorNodeID: vr.Initiator,
			Invitees: []NodeInstVO{{InviteeID: cfg.InviteNodeID, InviteeName: h.nodeName(ctx, nil, cfg.InviteNodeID)}}}},
	}
	out.MessageName = p.Name
	out.Type = VoteTypeProjectNodeAdd
	out.Status = VoteStatusName(vr.Status)
	return out, nil
}

func (h *ProjectNodeAddHandler) GetMessageListNecessaryInfo(ctx context.Context, voteID string) (interface{}, error) {
	cfg, err := h.config(ctx, voteID)
	if err != nil {
		return nil, err
	}
	p, err := h.openProject(ctx, nil, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	return &ProjectApprovalCustomizedMessage{ProjectID: p.ProjectID, ComputeFunc: p.ComputeFunc, ComputeMode: p.ComputeMode}, nil
}

func (h *ProjectNodeAddHandler) GetPartyStatusByVoteID(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	if h.d.isAutonomy() {
		return h.partyStatusByPartyInfos(ctx, voteID)
	}
	return h.partyStatusByInvites(ctx, voteID)
}

func (h *ProjectNodeAddHandler) DoCallBackRejected(ctx context.Context, vr *model.VoteRequestDO) {
	h.doCallBackRejectedDefault(ctx, vr)
}

// DoCallBackApproved adds the invited node (and its inst in AUTONOMY) to the project.
func (h *ProjectNodeAddHandler) DoCallBackApproved(ctx context.Context, vr *model.VoteRequestDO) {
	cfg, err := h.config(ctx, vr.VoteID)
	if err != nil {
		h.failed(ctx, vr, "project node add approval config not found")
		return
	}
	err = h.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a := &ProjectCallBackAction{ProjectNodeDOS: []callbackProjectNode{{ProjectID: cfg.ProjectID, NodeID: cfg.InviteNodeID}}}
		if h.d.isAutonomy() {
			var n model.NodeDO
			if tx.Where("node_id = ?", cfg.InviteNodeID).First(&n).Error == nil && n.InstID != "" {
				var cnt int64
				tx.Model(&model.ProjectInstDO{}).Where("project_id = ? AND inst_id = ?", cfg.ProjectID, n.InstID).Count(&cnt)
				if cnt == 0 {
					a.ProjectInstDOS = []callbackProjectInst{{ProjectID: cfg.ProjectID, InstID: n.InstID}}
				}
			}
		}
		return upsertProjectMembers(tx, a)
	})
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	h.success(ctx, vr)
}
