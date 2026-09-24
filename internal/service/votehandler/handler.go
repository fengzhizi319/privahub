package votehandler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Handler is the Go counterpart of Java VoteTypeHandler.
type Handler interface {
	// Type returns the supported vote type (Java supportTypes()).
	Type() string
	CreateApproval(ctx context.Context, initiatorID string, cfg *VoteConfig) (string, error)
	GetVoteMessageDetail(ctx context.Context, isInitiator bool, ownerID, voteID string) (interface{}, error)
	GetMessageListNecessaryInfo(ctx context.Context, voteID string) (interface{}, error)
	GetPartyStatusByVoteID(ctx context.Context, voteID string) ([]PartyVoteStatus, error)
	DoCallBackApproved(ctx context.Context, vr *model.VoteRequestDO)
	DoCallBackRejected(ctx context.Context, vr *model.VoteRequestDO)
	FlushVoteStatus(ctx context.Context, voteID string) error
	// GetVoter returns the node whose certificate signs a reply of voteParticipantID.
	GetVoter(voteParticipantID string) string
}

// approvalSpec holds the type-specific hooks of the Java
// AbstractVoteTypeHandler.createApproval template. All hooks run inside the
// createApproval transaction.
type approvalSpec interface {
	Type() string
	preCheck(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) error
	createVoteConfig(ctx context.Context, tx *gorm.DB, voteID, initiator string, cfg *VoteConfig) error
	voters(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) ([]string, error)
	executors(majorNodeID string, cfg *VoteConfig) []string
	approvedAction(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) (string, error)
	rejectAction(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) (string, error)
	requestDesc(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) (string, error)
	inviteDesc(ctx context.Context, tx *gorm.DB, initiator string, cfg *VoteConfig) (string, error)
	// signAtCreate: true → the request is signed at creation with the
	// platform node's cert and body.executors=[platformNode] (PROJECT_*);
	// false → unsigned, body.executors=[initiator], signed later by the
	// VoteRequestStatusMonitor (NODE_ROUTE / TEE_DOWNLOAD).
	signAtCreate() bool
}

// base implements the shared parts of Java AbstractVoteTypeHandler.
type base struct {
	d *Deps
}

// NewVoteID mirrors Java UUIDUtils.newUUID (32 hex chars).
func NewVoteID() string { return strings.ReplaceAll(uuid.New().String(), "-", "") }

// createApproval is the Java AbstractVoteTypeHandler.createApproval template:
// preCheck → voteId → createVoteConfig → request message → invites → request.
func (b *base) createApproval(ctx context.Context, spec approvalSpec, initiator string, cfg *VoteConfig) (string, error) {
	if cfg == nil {
		cfg = &VoteConfig{}
	}
	voteID := NewVoteID()
	err := b.d.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := spec.preCheck(ctx, tx, initiator, cfg); err != nil {
			return err
		}
		if err := spec.createVoteConfig(ctx, tx, voteID, initiator, cfg); err != nil {
			return err
		}
		voters, err := spec.voters(ctx, tx, initiator, cfg)
		if err != nil {
			return err
		}
		approved, err := spec.approvedAction(ctx, tx, initiator, cfg)
		if err != nil {
			return err
		}
		rejected, err := spec.rejectAction(ctx, tx, initiator, cfg)
		if err != nil {
			return err
		}
		major := b.d.platformNodeID()
		body := VoteRequestBody{
			VoteRequestID:     voteID,
			Type:              spec.Type(),
			Initiator:         initiator,
			VoteCounter:       initiator,
			Voters:            voters,
			ApprovedThreshold: len(voters),
			ApprovedAction:    approved,
			RejectedAction:    rejected,
		}
		var msg *VoteRequestMessage
		if spec.signAtCreate() {
			body.Executors = []string{major}
			msg, err = b.signedRequestMessage(ctx, major, &body)
			if err != nil {
				return err
			}
		} else {
			body.Executors = []string{initiator}
			msg = &VoteRequestMessage{Body: b64(toJSON(body))}
		}
		voteMsg := toJSON(msg)

		inviteDesc, err := spec.inviteDesc(ctx, tx, initiator, cfg)
		if err != nil {
			return err
		}
		for _, voter := range voters {
			if err := tx.Create(&model.VoteInviteDO{
				VoteID: voteID, Initiator: initiator, VoteParticipantID: voter, Type: spec.Type(),
				VoteMsg: voteMsg, Action: ActionReviewing, Description: inviteDesc,
			}).Error; err != nil {
				return err
			}
		}
		requestDesc, err := spec.requestDesc(ctx, tx, initiator, cfg)
		if err != nil {
			return err
		}
		parties := make([]PartyVoteInfo, 0, len(voters)+1)
		for _, v := range voters {
			parties = append(parties, PartyVoteInfo{PartyID: v, Action: ActionReviewing})
		}
		parties = upsertPartyVoteInfo(parties, PartyVoteInfo{PartyID: initiator, Action: ActionApproved})
		return tx.Create(&model.VoteRequestDO{
			VoteID:            voteID,
			Initiator:         initiator,
			Type:              spec.Type(),
			Voters:            toJSON(nonNil(voters)),
			VoteCounter:       initiator,
			Executors:         toJSON(nonNil(spec.executors(major, cfg))),
			ApprovedThreshold: len(voters),
			RequestMsg:        voteMsg,
			Status:            StatusReviewing,
			ExecuteStatus:     ExecCommitted,
			Description:       requestDesc,
			PartyVoteInfo:     toJSON(parties),
		}).Error
	})
	if err != nil {
		return "", err
	}
	return voteID, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// signedRequestMessage signs the base64 body with the key issued for nodeID.
// Without Kuscia (offline mode) the message is left unsigned.
func (b *base) signedRequestMessage(ctx context.Context, nodeID string, body *VoteRequestBody) (*VoteRequestMessage, error) {
	if b.d.Kuscia != nil {
		body.SignMeta = newSignMeta()
	}
	msg := &VoteRequestMessage{Body: b64(toJSON(body))}
	if b.d.Kuscia == nil {
		b.d.log().Warn("kuscia not configured, vote request left unsigned", zap.String("vote", body.VoteRequestID))
		return msg, nil
	}
	kc, err := b.d.Kuscia.GenerateKeyCerts(ctx, nodeID)
	if err != nil {
		return nil, bizWrap(errcode.KusciaRpcError, err, "")
	}
	sig, err := SignSHA256withRSA([]byte(msg.Body), kc.Key)
	if err != nil {
		return nil, bizWrap(errcode.SystemSignatureError, err, "")
	}
	msg.VoteRequestSignature = sig
	msg.CertChain = kc.CertChain
	return msg, nil
}

// parseRequest decodes vote_request.request_msg and its body.
func parseRequest(requestMsg string) (*VoteRequestMessage, *VoteRequestBody, error) {
	var msg VoteRequestMessage
	if err := json.Unmarshal([]byte(requestMsg), &msg); err != nil {
		return nil, nil, bizWrap(errcode.VoteCheckFailed, err, "invalid vote request message")
	}
	raw, err := unb64(msg.Body)
	if err != nil {
		return nil, nil, bizWrap(errcode.VoteCheckFailed, err, "invalid vote request body")
	}
	var body VoteRequestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, nil, bizWrap(errcode.VoteCheckFailed, err, "invalid vote request body")
	}
	return &msg, &body, nil
}

// --- shared lookups ---

func (b *base) findRequest(ctx context.Context, tx *gorm.DB, voteID string) (*model.VoteRequestDO, error) {
	if tx == nil {
		tx = b.d.DB.WithContext(ctx)
	}
	var vr model.VoteRequestDO
	if err := tx.Where("vote_id = ?", voteID).First(&vr).Error; err != nil {
		return nil, bizErr(errcode.VoteNotExists, "")
	}
	return &vr, nil
}

func (b *base) findInvite(ctx context.Context, voteID, participant string) (*model.VoteInviteDO, error) {
	var inv model.VoteInviteDO
	if err := b.d.DB.WithContext(ctx).Where("vote_id = ? AND vote_participant_id = ?", voteID, participant).
		First(&inv).Error; err != nil {
		return nil, bizErr(errcode.VoteNotExists, "")
	}
	return &inv, nil
}

func (b *base) openProject(ctx context.Context, tx *gorm.DB, projectID string) (*model.ProjectDO, error) {
	if tx == nil {
		tx = b.d.DB.WithContext(ctx)
	}
	var p model.ProjectDO
	if projectID == "" || tx.Where("project_id = ?", projectID).First(&p).Error != nil {
		return nil, bizErr(errcode.ProjectNotExists, "")
	}
	return &p, nil
}

func (b *base) nodeNames(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var nodes []model.NodeDO
	b.d.DB.WithContext(ctx).Where("node_id IN ?", ids).Find(&nodes)
	for _, n := range nodes {
		out[n.NodeID] = n.Name
	}
	return out
}

func (b *base) nodeName(ctx context.Context, tx *gorm.DB, id string) string {
	if tx == nil {
		tx = b.d.DB.WithContext(ctx)
	}
	var n model.NodeDO
	if tx.Where("node_id = ?", id).First(&n).Error == nil && n.Name != "" {
		return n.Name
	}
	return id
}

func (b *base) instNames(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var insts []model.InstDO
	b.d.DB.WithContext(ctx).Where("inst_id IN ?", ids).Find(&insts)
	for _, i := range insts {
		out[i.InstID] = i.Name
	}
	return out
}

func (b *base) projectNodeIDs(ctx context.Context, tx *gorm.DB, projectID string) []string {
	if tx == nil {
		tx = b.d.DB.WithContext(ctx)
	}
	var ids []string
	tx.Model(&model.ProjectNodeDO{}).Where("project_id = ?", projectID).Order("id").Pluck("node_id", &ids)
	return ids
}

// partyStatusByInvites is the default getPartyStatusByVoteID (invites + node names).
func (b *base) partyStatusByInvites(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	var invites []model.VoteInviteDO
	if err := b.d.DB.WithContext(ctx).Where("vote_id = ?", voteID).Order("id").Find(&invites).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(invites))
	for _, i := range invites {
		ids = append(ids, i.VoteParticipantID)
	}
	names := b.nodeNames(ctx, ids)
	out := make([]PartyVoteStatus, 0, len(invites))
	for _, i := range invites {
		out = append(out, PartyVoteStatus{
			ParticipantID: i.VoteParticipantID, ParticipantName: names[i.VoteParticipantID],
			Action: i.Action, Reason: i.Reason,
		})
	}
	return out, nil
}

// partyStatusByPartyInfos is the AUTONOMY getPartyStatusByVoteID
// (vote_request.party_vote_info + inst names).
func (b *base) partyStatusByPartyInfos(ctx context.Context, voteID string) ([]PartyVoteStatus, error) {
	vr, err := b.findRequest(ctx, nil, voteID)
	if err != nil {
		return nil, err
	}
	infos := parsePartyVoteInfos(vr.PartyVoteInfo)
	ids := make([]string, 0, len(infos))
	for _, i := range infos {
		ids = append(ids, i.PartyID)
	}
	names := b.instNames(ctx, ids)
	out := make([]PartyVoteStatus, 0, len(infos))
	for _, i := range infos {
		out = append(out, PartyVoteStatus{ParticipantID: i.PartyID, ParticipantName: names[i.PartyID], Action: i.Action, Reason: i.Reason})
	}
	return out, nil
}

// --- execute status (Java success/failed/observer) ---

func (b *base) setExecute(ctx context.Context, vr *model.VoteRequestDO, status, msg string) {
	vr.ExecuteStatus = status
	updates := map[string]interface{}{"execute_status": status}
	if msg != "" {
		vr.Msg = msg
		updates["msg"] = msg
	}
	if err := b.d.DB.WithContext(ctx).Model(&model.VoteRequestDO{}).Where("vote_id = ?", vr.VoteID).
		Updates(updates).Error; err != nil {
		b.d.log().Error("update vote execute status failed", zap.String("vote", vr.VoteID), zap.Error(err))
	}
}

func (b *base) success(ctx context.Context, vr *model.VoteRequestDO) {
	b.setExecute(ctx, vr, ExecSuccess, "")
}

func (b *base) failed(ctx context.Context, vr *model.VoteRequestDO, msg string) {
	b.d.log().Warn("vote callback failed", zap.String("vote", vr.VoteID), zap.String("msg", msg))
	b.setExecute(ctx, vr, ExecFailed, msg)
}

func (b *base) observer(ctx context.Context, vr *model.VoteRequestDO) {
	b.setExecute(ctx, vr, ExecObserver, "")
}

// Default hook implementations shared by handlers.

func (b *base) doCallBackRejectedDefault(ctx context.Context, vr *model.VoteRequestDO) {
	b.success(ctx, vr)
}

// GetVoter default: the platform node signs replies (Java AbstractVoteTypeHandler.getVoter).
func (b *base) GetVoter(string) string { return b.d.platformNodeID() }

// FlushVoteStatus default is a no-op.
func (b *base) FlushVoteStatus(context.Context, string) error { return nil }

// currentUser returns the login user of ctx (nil when none).
func currentUser(ctx context.Context) *auth.UserContext {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil
	}
	return u
}
