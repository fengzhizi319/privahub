package votehandler

import (
	"context"
	"strings"
	"sync"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/sqlutil"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Service is the Go counterpart of Java MessageServiceImpl +
// ApprovalServiceImpl, dispatching to the per-type Handler.
type Service struct {
	d        *Deps
	handlers map[string]Handler

	syncMu sync.RWMutex
}

// NewService creates the vote service with all five handlers.
func NewService(d *Deps) *Service {
	s := &Service{d: d, handlers: map[string]Handler{}}
	for _, h := range []Handler{
		NewNodeRouteHandler(d), NewTeeDownloadHandler(d), NewProjectCreateHandler(d),
		NewProjectArchiveHandler(d), NewProjectNodeAddHandler(d),
	} {
		s.handlers[h.Type()] = h
	}
	return s
}

// Deps returns the shared dependencies.
func (s *Service) Deps() *Deps { return s.d }

// SetDbSyncSender installs the EDGE → center sync sender (Java DbSyncUtil).
func (s *Service) SetDbSyncSender(sender DbSyncSender) {
	s.syncMu.Lock()
	s.d.Sync = sender
	s.syncMu.Unlock()
}

// SetTeeResultPuller installs the TEE result puller used by TEE_DOWNLOAD callbacks.
func (s *Service) SetTeeResultPuller(p TeeResultPuller) {
	s.syncMu.Lock()
	s.d.TeePuller = p
	s.syncMu.Unlock()
}

// syncSender returns the explicit sender, else the process-wide one.
func (s *Service) syncSender() DbSyncSender {
	s.syncMu.RLock()
	defer s.syncMu.RUnlock()
	if s.d.Sync != nil {
		return s.d.Sync
	}
	return GlobalDbSyncSender{}
}

// Handler returns the handler of a vote type.
func (s *Service) Handler(voteType string) (Handler, error) {
	h, ok := s.handlers[strings.ToUpper(strings.TrimSpace(voteType))]
	if !ok {
		return nil, bizErr(errcode.ParamError, "unsupported vote type: "+voteType)
	}
	return h, nil
}

// identityVerification: in AUTONOMY the caller must act as its own institution.
func (s *Service) identityVerification(ctx context.Context, ownerID string) error {
	if !s.d.isAutonomy() {
		return nil
	}
	if u := currentUser(ctx); u == nil || u.OwnerID != ownerID {
		return bizErr(errcode.VoteCheckFailed, "")
	}
	return nil
}

// =============================== approval ===============================

// CreateApproval is Java ApprovalServiceImpl.createApproval. It returns the new vote id.
func (s *Service) CreateApproval(ctx context.Context, initiatorID, voteType string, cfg *VoteConfig) (string, error) {
	if err := s.identityVerification(ctx, initiatorID); err != nil {
		return "", err
	}
	h, err := s.Handler(voteType)
	if err != nil {
		return "", err
	}
	db := s.d.DB.WithContext(ctx)
	if s.d.isAutonomy() {
		var n int64
		db.Model(&model.InstDO{}).Where("inst_id = ?", initiatorID).Count(&n)
		if n == 0 {
			return "", bizErr(errcode.InstNotExists, "")
		}
	} else {
		var n int64
		db.Model(&model.NodeDO{}).Where("node_id = ?", initiatorID).Count(&n)
		if n == 0 {
			return "", bizErr(errcode.NodeNotExistError, "")
		}
	}
	return h.CreateApproval(ctx, initiatorID, cfg)
}

// PullStatus is Java ApprovalServiceImpl.pullStatus: the TEE_DOWNLOAD vote
// status of every project node for one result.
func (s *Service) PullStatus(ctx context.Context, projectID, jobID, taskID, resourceID, resourceType string) (*PullStatusVO, error) {
	db := s.d.DB.WithContext(ctx)
	var pns []model.ProjectNodeDO
	if err := db.Where("project_id = ?", projectID).Order("id").Find(&pns).Error; err != nil {
		return nil, err
	}
	nodeIDs := make([]string, 0, len(pns))
	for _, pn := range pns {
		nodeIDs = append(nodeIDs, pn.NodeID)
	}
	b := &base{s.d}
	names := b.nodeNames(ctx, nodeIDs)
	var job model.ProjectJobDO
	if err := db.Where("project_id = ? AND job_id = ?", projectID, jobID).First(&job).Error; err != nil {
		return nil, bizErr(errcode.ProjectJobNotExists, "")
	}
	if taskID == "" {
		// Java requires taskID; result-list rows have none, so resolve it from
		// the project result and return it for the follow-up approval/create.
		var r model.ProjectResultDO
		if db.Where("project_id = ? AND job_id = ? AND ref_id = ? AND task_id <> ''", projectID, jobID, resourceID).
			Order("id").Limit(1).Find(&r).RowsAffected > 0 {
			taskID = r.TaskID
		}
	}
	out := &PullStatusVO{TaskID: taskID, JobID: jobID, GraphID: job.GraphID, ResourceType: resourceType,
		ResourceID: resourceID, Parties: []Participant{}}

	q := db.Model(&model.TeeDownloadApprovalConfigDO{}).Where("resource_id = ? AND resource_type = ?", resourceID, resourceType)
	if taskID != "" {
		q = q.Where("task_id = ?", taskID)
	}
	var voteIDs []string
	if err := q.Pluck("vote_id", &voteIDs).Error; err != nil {
		return nil, err
	}
	for _, party := range nodeIDs {
		p := Participant{NodeID: party, NodeName: names[party], Status: ActionNotInitiated}
		if len(voteIDs) > 0 {
			var vr model.VoteRequestDO
			if db.Where("initiator = ? AND vote_id IN ? AND type = ?", party, voteIDs, VoteTypeTeeDownload).
				Order("gmt_create DESC").Order("id DESC").First(&vr).Error == nil {
				p.Status = VoteStatusName(vr.Status)
				if vr.Status == StatusApproved {
					s.fillPullRecord(ctx, &p, resourceID)
				}
				var invites []model.VoteInviteDO
				db.Where("vote_id = ?", vr.VoteID).Order("id").Find(&invites)
				p.VoteInfos = make([]VoteInfo, 0, len(invites))
				for _, inv := range invites {
					p.VoteInfos = append(p.VoteInfos, VoteInfo{VoteID: inv.VoteID, NodeID: inv.VoteParticipantID,
						Action: inv.Action, Reason: inv.Reason})
				}
			}
		}
		out.Parties = append(out.Parties, p)
	}
	return out, nil
}

// fillPullRecord adds the TEE pull record of an approved download (table
// tee_node_datatable_management, kind Pull, datatable "{nodeId}-{resourceId}").
func (s *Service) fillPullRecord(ctx context.Context, p *Participant, resourceID string) {
	id := service.BuildTeeDatatableID(p.NodeID, resourceID)
	p.PullDatatableID = id
	if !s.d.DB.Migrator().HasTable(&model.TeeNodeDatatableManagementDO{}) {
		return
	}
	if m := service.TeePullStatus(ctx, s.d.DB, p.NodeID, "", id); m != nil {
		p.PullStatus, p.PullErrMsg = m.Status, m.ErrMsg
	}
}

// =============================== message ===============================

// MessageListRequest is Java MessageListRequest (page is 1-based).
type MessageListRequest struct {
	Page        int    `json:"page"`
	Size        int    `json:"size"`
	IsInitiator bool   `json:"isInitiator"`
	OwnerID     string `json:"ownerId"`
	IsProcessed *bool  `json:"isProcessed"`
	Type        string `json:"type"`
	KeyWord     string `json:"keyWord"`
}

// List is Java MessageServiceImpl.list.
func (s *Service) List(ctx context.Context, req *MessageListRequest) (*MessageListVO, error) {
	if err := s.identityVerification(ctx, req.OwnerID); err != nil {
		return nil, err
	}
	page, size := req.Page, req.Size
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	db := s.d.DB.WithContext(ctx)
	out := &MessageListVO{Messages: []MessageVO{}, PageNum: page, PageSize: size}
	if !req.IsInitiator {
		q := db.Model(&model.VoteInviteDO{}).Where("vote_participant_id = ?", req.OwnerID)
		if req.IsProcessed != nil {
			if *req.IsProcessed {
				q = q.Where("action <> ?", ActionReviewing)
			} else {
				q = q.Where("action = ?", ActionReviewing)
			}
		}
		if req.KeyWord != "" {
			q = q.Where("description LIKE ? ESCAPE '!'", sqlutil.Contains(req.KeyWord))
		}
		if req.Type != "" {
			q = q.Where("type = ?", req.Type)
		}
		if err := q.Count(&out.Total).Error; err != nil {
			return nil, err
		}
		var invites []model.VoteInviteDO
		if err := q.Order("gmt_create DESC").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&invites).Error; err != nil {
			return nil, err
		}
		for i := range invites {
			if vo := s.inviteVO(ctx, &invites[i]); vo != nil {
				out.Messages = append(out.Messages, *vo)
			}
		}
	} else {
		q := db.Model(&model.VoteRequestDO{}).Where("initiator = ?", req.OwnerID)
		if req.KeyWord != "" {
			q = q.Where("description LIKE ? ESCAPE '!'", sqlutil.Contains(req.KeyWord))
		}
		if req.Type != "" {
			q = q.Where("type = ?", req.Type)
		}
		// Java queryCount ignores isProcessed for requests.
		if err := q.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
			return nil, err
		}
		if req.IsProcessed != nil {
			if *req.IsProcessed {
				q = q.Where("status <> ?", StatusReviewing)
			} else {
				q = q.Where("status = ?", StatusReviewing)
			}
		}
		var reqs []model.VoteRequestDO
		if err := q.Order("gmt_create DESC").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&reqs).Error; err != nil {
			return nil, err
		}
		for i := range reqs {
			out.Messages = append(out.Messages, s.requestVO(ctx, &reqs[i]))
		}
	}
	out.TotalPage = (out.Total + int64(size) - 1) / int64(size)
	return out, nil
}

func (s *Service) listInfo(ctx context.Context, voteType, voteID string) interface{} {
	h, err := s.Handler(voteType)
	if err != nil {
		return nil
	}
	info, err := h.GetMessageListNecessaryInfo(ctx, voteID)
	if err != nil {
		s.d.log().Warn("message list info unavailable", zap.String("vote", voteID), zap.Error(err))
		return nil
	}
	return info
}

func (s *Service) inviteVO(ctx context.Context, inv *model.VoteInviteDO) *MessageVO {
	b := &base{s.d}
	vr, err := b.findRequest(ctx, nil, inv.VoteID)
	if err != nil {
		s.d.log().Error("vote request not found for invite", zap.String("vote", inv.VoteID))
		return nil
	}
	var initiatorName string
	if s.d.isAutonomy() {
		initiatorName = b.instNames(ctx, []string{vr.Initiator})[vr.Initiator]
	} else {
		initiatorName = b.nodeName(ctx, nil, vr.Initiator)
	}
	return &MessageVO{
		CreateTime:  rfc3339(inv.GmtCreate),
		VoteID:      inv.VoteID,
		Type:        inv.Type,
		MessageName: inv.Description,
		Status:      inv.Action,
		InitiatingTypeMessage: &ReceiverMessage{ParticipantID: inv.VoteParticipantID, Reason: inv.Reason,
			InitiatorNodeID: vr.Initiator, InitiatorNodeName: initiatorName},
		VoteTypeMessage: s.listInfo(ctx, inv.Type, inv.VoteID),
	}
}

func (s *Service) requestVO(ctx context.Context, vr *model.VoteRequestDO) MessageVO {
	statuses := []PartyVoteStatus{}
	if h, err := s.Handler(vr.Type); err == nil {
		if st, err := h.GetPartyStatusByVoteID(ctx, vr.VoteID); err == nil && st != nil {
			statuses = st
		}
	}
	return MessageVO{
		CreateTime:            rfc3339(vr.GmtCreate),
		VoteID:                vr.VoteID,
		Type:                  vr.Type,
		MessageName:           vr.Description,
		Status:                VoteStatusName(vr.Status),
		InitiatingTypeMessage: &InitiatorMessage{PartyVoteStatuses: statuses},
		VoteTypeMessage:       s.listInfo(ctx, vr.Type, vr.VoteID),
	}
}

// Detail is Java MessageServiceImpl.detail.
func (s *Service) Detail(ctx context.Context, isInitiator bool, ownerID, voteID, voteType string) (interface{}, error) {
	if err := s.identityVerification(ctx, ownerID); err != nil {
		return nil, err
	}
	if voteType == "" {
		vr, err := (&base{s.d}).findRequest(ctx, nil, voteID)
		if err != nil {
			return nil, err
		}
		voteType = vr.Type
	}
	h, err := s.Handler(voteType)
	if err != nil {
		return nil, err
	}
	return h.GetVoteMessageDetail(ctx, isInitiator, ownerID, voteID)
}

// PendingCount is Java MessageServiceImpl.pendingCount.
func (s *Service) PendingCount(ctx context.Context, ownerID string) (int64, error) {
	if err := s.identityVerification(ctx, ownerID); err != nil {
		return 0, err
	}
	var n int64
	err := s.d.DB.WithContext(ctx).Model(&model.VoteInviteDO{}).
		Where("vote_participant_id = ? AND action = ?", ownerID, ActionReviewing).Count(&n).Error
	return n, err
}

// Reply is Java MessageServiceImpl.reply: sign the reply with the voter's
// cert and store it per platform (CENTER local; EDGE → center sync; AUTONOMY
// local + party_vote_info + flushVoteStatus when everybody approved).
func (s *Service) Reply(ctx context.Context, action, reason, participantID, voteID string) error {
	if action != ActionApproved && action != ActionRejected {
		return bizErr(errcode.ParamError, "action must be APPROVED or REJECTED")
	}
	if err := s.identityVerification(ctx, participantID); err != nil {
		return err
	}
	b := &base{s.d}
	inv, err := b.findInvite(ctx, voteID, participantID)
	if err != nil {
		return err
	}
	h, err := s.Handler(inv.Type)
	if err != nil {
		return err
	}
	voter := h.GetVoter(participantID)
	vr, err := b.findRequest(ctx, nil, voteID)
	if err != nil {
		return err
	}
	if err := s.checkVoteRoute(ctx, inv); err != nil {
		return err
	}
	msg, body, err := parseRequest(vr.RequestMsg)
	if err != nil {
		return err
	}
	teeNodeRoute := vr.Type == VoteTypeNodeRoute && contains(parseStringList(vr.Executors), teeExecutor)
	signing := !teeNodeRoute && s.d.Kuscia != nil
	if signing && strings.TrimSpace(msg.VoteRequestSignature) == "" {
		return bizErr(errcode.VoteSignatureSynchronizing, "")
	}
	if !contains(body.Voters, participantID) {
		return bizErr(errcode.VoteCheckFailed, "")
	}
	if signing {
		kc, err := s.d.Kuscia.GenerateKeyCerts(ctx, voter)
		if err != nil {
			return bizWrap(errcode.KusciaRpcError, err, "")
		}
		teeAction := "REJECT"
		if action == ActionApproved {
			teeAction = "APPROVE"
		}
		replyBody := b64(toJSON(VoteReplyBody{VoteRequestID: voteID, Voter: voter, Action: teeAction, SignMeta: newSignMeta()}))
		sig, err := SignSHA256withRSA([]byte(replyBody+msg.VoteRequestSignature), kc.Key)
		if err != nil {
			return bizWrap(errcode.SystemSignatureError, err, "")
		}
		inv.VoteMsg = toJSON(VoteReplyMessage{Body: replyBody, CertChain: certChainToPEM(kc.CertChain), Signature: sig})
	}
	inv.Action = action
	inv.Reason = reason

	db := s.d.DB.WithContext(ctx)
	switch {
	case s.d.isEdge():
		// Java: EDGE does not write locally; the center syncs the row back.
		return s.syncSender().SendToCenter(ctx, DbSyncRequest{SyncDataType: SyncVoteInvite, ProjectNodesInfo: inv})
	case s.d.isAutonomy():
		infos := upsertPartyVoteInfo(parsePartyVoteInfos(vr.PartyVoteInfo),
			PartyVoteInfo{PartyID: inv.VoteParticipantID, Action: inv.Action, Reason: inv.Reason})
		allApproved := true
		for _, i := range infos {
			if i.Action != ActionApproved {
				allApproved = false
			}
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(inv).Error; err != nil {
				return err
			}
			return tx.Model(&model.VoteRequestDO{}).Where("vote_id = ?", voteID).
				Update("party_vote_info", toJSON(infos)).Error
		}); err != nil {
			return err
		}
		if allApproved {
			return h.FlushVoteStatus(ctx, voteID)
		}
		return nil
	default:
		return db.Save(inv).Error
	}
}

// checkVoteRoute: in AUTONOMY the route to the initiator's master must be ready.
func (s *Service) checkVoteRoute(ctx context.Context, inv *model.VoteInviteDO) error {
	if !s.d.isAutonomy() {
		return nil
	}
	var nodes []model.NodeDO
	s.d.DB.WithContext(ctx).Where("inst_id = ?", inv.Initiator).Order("id").Find(&nodes)
	if len(nodes) == 0 {
		return bizErr(errcode.VoteInitiatorInstNotFound, "initiator inst not found")
	}
	dst := nodes[0].MasterNodeID
	if dst == "" || dst == "master" {
		dst = nodes[0].NodeID
	}
	me := s.d.platformNodeID()
	if s.d.Kuscia == nil || dst == me {
		return nil
	}
	if !s.d.Kuscia.DomainRouteReady(ctx, me, me, dst) {
		return bizErr(errcode.VoteMasterRouteNotReady, me+"->"+dst)
	}
	return nil
}
