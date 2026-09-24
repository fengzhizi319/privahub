package votehandler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// autonomyBase is Java AbstractAutonomyVoteTypeHandler (PROJECT_CREATE /
// PROJECT_ARCHIVE in AUTONOMY mode: parties are institutions).
type autonomyBase struct{ base }

func (b *autonomyBase) signAtCreate() bool { return true }

func (b *autonomyBase) executors(major string, _ *VoteConfig) []string { return []string{major} }

func (b *autonomyBase) inviteDesc(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	p, err := b.openProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return "", err
	}
	return p.Name, nil
}

func (b *autonomyBase) requestDesc(ctx context.Context, tx *gorm.DB, i string, cfg *VoteConfig) (string, error) {
	return b.inviteDesc(ctx, tx, i, cfg)
}

func (b *autonomyBase) approvalConfig(ctx context.Context, tx *gorm.DB, voteID string) (*model.ProjectApprovalConfigDO, error) {
	if tx == nil {
		tx = b.d.DB.WithContext(ctx)
	}
	var c model.ProjectApprovalConfigDO
	if err := tx.Where("vote_id = ?", voteID).First(&c).Error; err != nil {
		return nil, bizErr(errcode.VoteNotExists, "")
	}
	return &c, nil
}

func (b *autonomyBase) createConfigOfProject(ctx context.Context, tx *gorm.DB, projectID string) (*model.ProjectApprovalConfigDO, error) {
	if tx == nil {
		tx = b.d.DB.WithContext(ctx)
	}
	var c model.ProjectApprovalConfigDO
	if err := tx.Where("project_id = ? AND type = ?", projectID, VoteTypeProjectCreate).
		Order("id DESC").First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// fillParticipantNames resolves node / inst names of participantNodeInfo
// (Java getVoteMessageDetail / getProjectParticipants).
func (b *autonomyBase) fillParticipantNames(ctx context.Context, list []ParticipantNodeInstVO) ([]ParticipantNodeInstVO, error) {
	db := b.d.DB.WithContext(ctx)
	for i := range list {
		list[i].InitiatorNodeName = b.nodeName(ctx, nil, list[i].InitiatorNodeID)
		for j := range list[i].Invitees {
			inv := &list[i].Invitees[j]
			var n model.NodeDO
			if db.Where("node_id = ?", inv.InviteeID).First(&n).Error != nil {
				return nil, bizErr(errcode.NodeNotExistError, inv.InviteeID)
			}
			var inst model.InstDO
			if db.Where("inst_id = ?", n.InstID).First(&inst).Error != nil {
				return nil, bizErr(errcode.InstNotExists, "")
			}
			inv.InstID, inv.InstName, inv.InviteeName = inst.InstID, inst.Name, n.Name
		}
	}
	if list == nil {
		list = []ParticipantNodeInstVO{}
	}
	return list, nil
}

func (b *autonomyBase) instName(ctx context.Context, instID string) string {
	return b.instNames(ctx, []string{instID})[instID]
}

// verify checks the request signature and the signer's certificate chain
// (Java AbstractAutonomyVoteTypeHandler.verify). Skipped in offline mode.
func (b *autonomyBase) verify(ctx context.Context, msg *VoteRequestMessage, body *VoteRequestBody) error {
	if b.d.Kuscia == nil {
		return nil
	}
	if len(msg.CertChain) < 2 || len(body.Executors) == 0 {
		return bizErr(errcode.SystemVerifySignatureError, "vote request is not signed")
	}
	if err := VerifySHA256withRSA([]byte(msg.Body), msg.CertChain[0], msg.VoteRequestSignature); err != nil {
		return bizWrap(errcode.SystemVerifySignatureError, err, "")
	}
	cert := b.d.Kuscia.DomainCert(ctx, body.Executors[0], "")
	if !CompareCertPubKey(cert, msg.CertChain[1]) {
		return bizErr(errcode.SystemVerifySignatureError, "cert does not match")
	}
	if !ValidateCertChain(msg.CertChain) {
		return bizErr(errcode.SystemVerifySignatureError, "cert chain invalid")
	}
	return nil
}

// projectMessageDetail is Java AbstractAutonomyVoteTypeHandler.getVoteMessageDetail.
func (b *autonomyBase) projectMessageDetail(ctx context.Context, voteID string,
	partyStatus func(context.Context, string) ([]PartyVoteStatus, error)) (*ProjectCreateMessageDetail, error) {
	vr, err := b.findRequest(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	msg, body, err := parseRequest(vr.RequestMsg)
	if err != nil {
		return nil, err
	}
	if err := b.verify(ctx, msg, body); err != nil {
		return nil, err
	}
	if body.VoteRequestID != voteID {
		return nil, bizErr(errcode.VoteCheckFailed, "")
	}
	cfg, err := b.approvalConfig(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	participants, err := b.fillParticipantNames(ctx, parseParticipantNodeInfo(cfg.ParticipantNodeInfo))
	if err != nil {
		return nil, err
	}
	p, err := b.openProject(ctx, nil, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	statuses, err := partyStatus(ctx, voteID)
	if err != nil {
		return nil, err
	}
	out := &ProjectCreateMessageDetail{
		InitiatorID:            body.Initiator,
		InitiatorName:          b.instName(ctx, body.Initiator),
		ProjectName:            p.Name,
		PartyVoteStatuses:      statuses,
		ComputeMode:            p.ComputeMode,
		ComputeFunc:            p.ComputeFunc,
		ProjectDesc:            p.Description,
		GmtCreated:             rfc3339(p.GmtCreate),
		ParticipantNodeInstVOS: participants,
	}
	out.MessageName = p.Name
	out.Type = cfg.Type
	out.Status = VoteStatusName(vr.Status)
	return out, nil
}

func (b *autonomyBase) GetMessageListNecessaryInfo(ctx context.Context, voteID string) (interface{}, error) {
	cfg, err := b.approvalConfig(ctx, nil, voteID)
	if err != nil {
		return nil, bizErr(errcode.VoteProjectVoteNotExists, voteID)
	}
	p, err := b.openProject(ctx, nil, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	return &ProjectApprovalCustomizedMessage{ProjectID: p.ProjectID, ComputeFunc: p.ComputeFunc, ComputeMode: p.ComputeMode}, nil
}

// --- callback payload (Java ProjectCallBackAction) ---

type callbackProject struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Status    int8   `json:"status"`
}

type callbackProjectNode struct {
	ProjectID string `json:"projectId"`
	NodeID    string `json:"nodeId"`
}

type callbackProjectInst struct {
	ProjectID string `json:"projectId"`
	InstID    string `json:"instId"`
}

// ProjectCallBackAction is the payload behind "PROJECT_CREATE," / "PROJECT_ARCHIVE," actions.
type ProjectCallBackAction struct {
	ProjectDO      callbackProject       `json:"projectDO"`
	ProjectNodeDOS []callbackProjectNode `json:"projectNodeDOS"`
	ProjectInstDOS []callbackProjectInst `json:"projectInstDOS"`
}

func (a *ProjectCallBackAction) hasInst(instID string) bool {
	for _, i := range a.ProjectInstDOS {
		if i.InstID == instID {
			return true
		}
	}
	return false
}

func encodeAction(prefix string, a *ProjectCallBackAction) string { return prefix + "," + toJSON(a) }

func decodeAction(action string) (*ProjectCallBackAction, error) {
	idx := strings.Index(action, ",")
	if idx < 0 {
		return nil, bizErr(errcode.VoteCheckFailed, "invalid callback action")
	}
	var a ProjectCallBackAction
	if err := json.Unmarshal([]byte(action[idx+1:]), &a); err != nil {
		return nil, bizWrap(errcode.VoteCheckFailed, err, "invalid callback action")
	}
	return &a, nil
}

// deleteProjectMembers hard-deletes project_inst / project_node rows (unique
// keys must allow re-adding the same rows later).
func deleteProjectMembers(tx *gorm.DB, a *ProjectCallBackAction) error {
	for _, i := range a.ProjectInstDOS {
		if err := tx.Unscoped().Where("project_id = ? AND inst_id = ?", i.ProjectID, i.InstID).
			Delete(&model.ProjectInstDO{}).Error; err != nil {
			return err
		}
	}
	for _, n := range a.ProjectNodeDOS {
		if err := tx.Unscoped().Where("project_id = ? AND node_id = ?", n.ProjectID, n.NodeID).
			Delete(&model.ProjectNodeDO{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func upsertProjectMembers(tx *gorm.DB, a *ProjectCallBackAction) error {
	for _, i := range a.ProjectInstDOS {
		if err := tx.Unscoped().Where("project_id = ? AND inst_id = ?", i.ProjectID, i.InstID).
			Delete(&model.ProjectInstDO{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.ProjectInstDO{ProjectID: i.ProjectID, InstID: i.InstID}).Error; err != nil {
			return err
		}
	}
	for _, n := range a.ProjectNodeDOS {
		if err := tx.Unscoped().Where("project_id = ? AND node_id = ?", n.ProjectID, n.NodeID).
			Delete(&model.ProjectNodeDO{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.ProjectNodeDO{ProjectID: n.ProjectID, NodeID: n.NodeID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func setProjectStatus(tx *gorm.DB, projectID string, status int8) error {
	return tx.Model(&model.ProjectDO{}).Where("project_id = ?", projectID).Update("status", status).Error
}

// ============================ PROJECT_CREATE ============================

// ProjectCreateHandler is Java ProjectCreateMessageHandler.
type ProjectCreateHandler struct{ autonomyBase }

// NewProjectCreateHandler creates the PROJECT_CREATE handler.
func NewProjectCreateHandler(d *Deps) *ProjectCreateHandler {
	return &ProjectCreateHandler{autonomyBase{base{d}}}
}

func (h *ProjectCreateHandler) Type() string { return VoteTypeProjectCreate }

func (h *ProjectCreateHandler) CreateApproval(ctx context.Context, initiator string, cfg *VoteConfig) (string, error) {
	return h.createApproval(ctx, h, initiator, cfg)
}

func (h *ProjectCreateHandler) preCheck(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) error {
	if _, err := h.openProject(ctx, tx, cfg.ProjectID); err != nil {
		return err
	}
	if len(without(cfg.Participants, initiator)) == 0 {
		return bizErr(errcode.VoteParticipantNotExist, "")
	}
	seen := map[string]bool{}
	var initiatorNodes, invitees []string
	for _, p := range cfg.ParticipantNodeInstVOS {
		if seen[p.InitiatorNodeID] {
			return bizErr(errcode.VoteInitiatorNodeDuplicate, "")
		}
		seen[p.InitiatorNodeID] = true
		initiatorNodes = append(initiatorNodes, p.InitiatorNodeID)
		for _, inv := range p.Invitees {
			invitees = append(invitees, inv.InviteeID)
		}
	}
	// Java instService.checkNodesInInst(initiator, initiatorNodeIds)
	if len(initiatorNodes) > 0 {
		var n int64
		tx.Model(&model.NodeDO{}).Where("node_id IN ? AND inst_id = ?", initiatorNodes, initiator).Count(&n)
		if int(n) != len(initiatorNodes) {
			return bizErr(errcode.InstInitiatorInstNodeMismatch, "")
		}
	}
	if len(invitees) > 0 {
		var instIDs []string
		tx.Model(&model.NodeDO{}).Where("node_id IN ?", invitees).Pluck("inst_id", &instIDs)
		for _, id := range instIDs {
			if !contains(cfg.Participants, id) || id == initiator {
				return bizErr(errcode.InstInviteeInstNodeMismatch, "")
			}
		}
	}
	return nil
}

func (h *ProjectCreateHandler) createVoteConfig(_ context.Context, tx *gorm.DB, voteID, initiator string, cfg *VoteConfig) error {
	return tx.Create(&model.ProjectApprovalConfigDO{
		VoteID: voteID, Initiator: initiator, Type: VoteTypeProjectCreate,
		Parties: toJSON(nonNil(cfg.Participants)), ParticipantNodeInfo: toJSON(cfg.ParticipantNodeInstVOS),
		ProjectID: cfg.ProjectID,
	}).Error
}

func (h *ProjectCreateHandler) voters(_ context.Context, _ *gorm.DB, initiator string, cfg *VoteConfig) ([]string, error) {
	return without(cfg.Participants, initiator), nil
}

func (h *ProjectCreateHandler) callbackAction(ctx context.Context, tx *gorm.DB, cfg *VoteConfig, status int8) (*ProjectCallBackAction, error) {
	p, err := h.openProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return nil, err
	}
	a := &ProjectCallBackAction{ProjectDO: callbackProject{ProjectID: p.ProjectID, Name: p.Name, Status: status}}
	for _, inst := range cfg.Participants {
		a.ProjectInstDOS = append(a.ProjectInstDOS, callbackProjectInst{ProjectID: p.ProjectID, InstID: inst})
	}
	for _, pn := range cfg.ParticipantNodeInstVOS {
		a.ProjectNodeDOS = append(a.ProjectNodeDOS, callbackProjectNode{ProjectID: p.ProjectID, NodeID: pn.InitiatorNodeID})
		for _, inv := range pn.Invitees {
			a.ProjectNodeDOS = append(a.ProjectNodeDOS, callbackProjectNode{ProjectID: p.ProjectID, NodeID: inv.InviteeID})
		}
	}
	return a, nil
}

func (h *ProjectCreateHandler) approvedAction(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	a, err := h.callbackAction(ctx, tx, cfg, ProjectApproved)
	if err != nil {
		return "", err
	}
	return encodeAction(VoteTypeProjectCreate, a), nil
}

func (h *ProjectCreateHandler) rejectAction(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	a, err := h.callbackAction(ctx, tx, cfg, ProjectArchived)
	if err != nil {
		return "", err
	}
	return encodeAction(VoteTypeProjectArchive, a), nil
}

func (h *ProjectCreateHandler) GetVoteMessageDetail(ctx context.Context, _ bool, _, voteID string) (interface{}, error) {
	return h.projectMessageDetail(ctx, voteID, h.GetPartyStatusByVoteID)
}

func (h *ProjectCreateHandler) GetPartyStatusByVoteID(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	return h.partyStatusByPartyInfos(ctx, voteID)
}

// DoCallBackApproved: on the initiator, add all insts / nodes and mark the
// project APPROVED (Java ProjectCreateMessageHandler.doCallBackApproved).
func (h *ProjectCreateHandler) DoCallBackApproved(ctx context.Context, vr *model.VoteRequestDO) {
	if !h.d.isCurrentInst(ctx, vr.Initiator) {
		return
	}
	_, body, err := parseRequest(vr.RequestMsg)
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	a, err := decodeAction(body.ApprovedAction)
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	inst := h.d.currentInstID(ctx)
	if !a.hasInst(inst) {
		h.failed(ctx, vr, "doCallBackApproved error,voters does not has party : "+inst)
		return
	}
	p, err := h.openProject(ctx, nil, a.ProjectDO.ProjectID)
	if err != nil {
		h.failed(ctx, vr, "doCallBack create project Approved error,project ["+a.ProjectDO.Name+"] does not exist ")
		return
	}
	if p.Status != ProjectReviewing {
		h.failed(ctx, vr, "project ["+p.Name+"] status Is not reviewing,status Is ["+ProjectStatusName(p.Status)+"]")
		return
	}
	if err := h.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertProjectMembers(tx, a); err != nil {
			return err
		}
		return setProjectStatus(tx, p.ProjectID, ProjectApproved)
	}); err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	h.success(ctx, vr)
}

// DoCallBackRejected: archive the project and drop its members.
func (h *ProjectCreateHandler) DoCallBackRejected(ctx context.Context, vr *model.VoteRequestDO) {
	if !h.d.isCurrentInst(ctx, vr.Initiator) {
		return
	}
	_, body, err := parseRequest(vr.RequestMsg)
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	a, err := decodeAction(body.RejectedAction)
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	inst := h.d.currentInstID(ctx)
	if !a.hasInst(inst) {
		h.failed(ctx, vr, "doCallBackRejected error,voters does not has party : "+inst)
		return
	}
	if err := h.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := setProjectStatus(tx, a.ProjectDO.ProjectID, ProjectArchived); err != nil {
			return err
		}
		return deleteProjectMembers(tx, a)
	}); err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	h.d.log().Info("project archived after rejected create vote", zap.String("project", a.ProjectDO.ProjectID))
	h.success(ctx, vr)
}

// ============================ PROJECT_ARCHIVE ============================

// ProjectArchiveHandler is Java ProjectArchiveHandler.
type ProjectArchiveHandler struct{ autonomyBase }

// NewProjectArchiveHandler creates the PROJECT_ARCHIVE handler.
func NewProjectArchiveHandler(d *Deps) *ProjectArchiveHandler {
	return &ProjectArchiveHandler{autonomyBase{base{d}}}
}

func (h *ProjectArchiveHandler) Type() string { return VoteTypeProjectArchive }

func (h *ProjectArchiveHandler) CreateApproval(ctx context.Context, initiator string, cfg *VoteConfig) (string, error) {
	return h.createApproval(ctx, h, initiator, cfg)
}

func (h *ProjectArchiveHandler) preCheck(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) error {
	p, err := h.openProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return err
	}
	if p.Status != ProjectApproved {
		return bizErr(errcode.ProjectCanNotCreateArchiveVote, "")
	}
	var archives []model.ProjectApprovalConfigDO
	tx.Where("project_id = ? AND type = ?", cfg.ProjectID, VoteTypeProjectArchive).Find(&archives)
	for _, a := range archives {
		var vr model.VoteRequestDO
		if tx.Where("vote_id = ?", a.VoteID).First(&vr).Error == nil && vr.Status == StatusReviewing {
			return bizErr(errcode.VoteProjectArchiveVoteAlreadyExist, p.Name)
		}
	}
	if _, err := h.createConfigOfProject(ctx, tx, cfg.ProjectID); err != nil {
		return bizErr(errcode.VoteProjectVoteNotExists, p.Name)
	}
	return nil
}

func (h *ProjectArchiveHandler) createVoteConfig(ctx context.Context, tx *gorm.DB, voteID, initiator string, cfg *VoteConfig) error {
	createCfg, err := h.createConfigOfProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return bizErr(errcode.VoteProjectVoteNotExists, cfg.ProjectID)
	}
	list := parseParticipantNodeInfo(createCfg.ParticipantNodeInfo)
	instOf := map[string]string{}
	var nodes []model.NodeDO
	tx.Find(&nodes)
	for _, n := range nodes {
		instOf[n.NodeID] = n.InstID
	}
	// Java: the archive initiator's node becomes the initiator node of each
	// pair it was invited to.
	for i := range list {
		if list[i].InitiatorNodeID == initiator {
			break
		}
		for j := range list[i].Invitees {
			if instOf[list[i].Invitees[j].InviteeID] == initiator {
				invitee := list[i].Invitees[j].InviteeID
				list[i].Invitees[j].InviteeID = list[i].InitiatorNodeID
				list[i].InitiatorNodeID = invitee
			}
		}
	}
	return tx.Create(&model.ProjectApprovalConfigDO{
		VoteID: voteID, Initiator: initiator, Type: VoteTypeProjectArchive,
		ParticipantNodeInfo: toJSON(list), ProjectID: cfg.ProjectID, Parties: createCfg.Parties,
	}).Error
}

func (h *ProjectArchiveHandler) voters(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) ([]string, error) {
	createCfg, err := h.createConfigOfProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return nil, bizErr(errcode.VoteProjectVoteNotExists, cfg.ProjectID)
	}
	return without(parseStringList(createCfg.Parties), initiator), nil
}

func (h *ProjectArchiveHandler) approvedAction(ctx context.Context, tx *gorm.DB, _ string, cfg *VoteConfig) (string, error) {
	p, err := h.openProject(ctx, tx, cfg.ProjectID)
	if err != nil {
		return "", err
	}
	a := &ProjectCallBackAction{ProjectDO: callbackProject{ProjectID: p.ProjectID, Name: p.Name, Status: ProjectArchived}}
	var pns []model.ProjectNodeDO
	tx.Where("project_id = ?", p.ProjectID).Find(&pns)
	for _, n := range pns {
		a.ProjectNodeDOS = append(a.ProjectNodeDOS, callbackProjectNode{ProjectID: n.ProjectID, NodeID: n.NodeID})
	}
	var pis []model.ProjectInstDO
	tx.Where("project_id = ?", p.ProjectID).Find(&pis)
	for _, i := range pis {
		a.ProjectInstDOS = append(a.ProjectInstDOS, callbackProjectInst{ProjectID: i.ProjectID, InstID: i.InstID})
	}
	return encodeAction(VoteTypeProjectCreate, a), nil
}

func (h *ProjectArchiveHandler) rejectAction(context.Context, *gorm.DB, string, *VoteConfig) (string, error) {
	return "NONE", nil
}

func (h *ProjectArchiveHandler) GetVoteMessageDetail(ctx context.Context, _ bool, _, voteID string) (interface{}, error) {
	return h.projectMessageDetail(ctx, voteID, h.GetPartyStatusByVoteID)
}

// GetPartyStatusByVoteID adds each party's action on the project creation
// vote (Java ProjectArchivePartyVoteStatus).
func (h *ProjectArchiveHandler) GetPartyStatusByVoteID(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	statuses, err := h.partyStatusByPartyInfos(ctx, voteID)
	if err != nil {
		return nil, err
	}
	cfg, err := h.approvalConfig(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	createActions := map[string]string{}
	if createCfg, err := h.createConfigOfProject(ctx, nil, cfg.ProjectID); err == nil {
		if cvr, err := h.findRequest(ctx, nil, createCfg.VoteID); err == nil {
			for _, i := range parsePartyVoteInfos(cvr.PartyVoteInfo) {
				createActions[i.PartyID] = i.Action
			}
		}
	}
	for i := range statuses {
		statuses[i].ProjectCreateVoteAction = createActions[statuses[i].ParticipantID]
	}
	return statuses, nil
}

// DoCallBackApproved archives the project on the initiator side.
func (h *ProjectArchiveHandler) DoCallBackApproved(ctx context.Context, vr *model.VoteRequestDO) {
	if !h.d.isCurrentInst(ctx, vr.Initiator) {
		return
	}
	_, body, err := parseRequest(vr.RequestMsg)
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	a, err := decodeAction(body.ApprovedAction)
	if err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	p, err := h.openProject(ctx, nil, a.ProjectDO.ProjectID)
	if err != nil {
		h.failed(ctx, vr, "doCallBack archive project Approved error,project ["+a.ProjectDO.Name+"] does not exist ")
		return
	}
	inst := h.d.currentInstID(ctx)
	if !a.hasInst(inst) {
		h.failed(ctx, vr, "doCallBackApproved error,voters does not has party : "+inst)
		return
	}
	if err := h.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if p.Status == ProjectApproved {
			if err := deleteProjectMembers(tx, a); err != nil {
				return err
			}
		}
		return setProjectStatus(tx, p.ProjectID, ProjectArchived)
	}); err != nil {
		h.failed(ctx, vr, err.Error())
		return
	}
	h.success(ctx, vr)
}

func (h *ProjectArchiveHandler) DoCallBackRejected(ctx context.Context, vr *model.VoteRequestDO) {
	h.doCallBackRejectedDefault(ctx, vr)
}

// FlushVoteStatus marks the project ARCHIVED once every party approved (AUTONOMY reply).
func (h *ProjectArchiveHandler) FlushVoteStatus(ctx context.Context, voteID string) error {
	cfg, err := h.approvalConfig(ctx, nil, voteID)
	if err != nil {
		return err
	}
	return setProjectStatus(h.d.DB.WithContext(ctx), cfg.ProjectID, ProjectArchived)
}
