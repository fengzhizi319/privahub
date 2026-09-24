package votehandler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"go.uber.org/zap"
)

// Monitor runs Java VoteRequestStatusMonitor and VoteInviteStatusMonitor
// (@Scheduled initialDelay=6s, fixedDelay=1s) as one background goroutine.
type Monitor struct {
	svc          *Service
	interval     time.Duration
	initialDelay time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// MonitorOptions configures the monitor timing.
type MonitorOptions struct {
	Interval     time.Duration
	InitialDelay time.Duration
}

// MonitorOptionsFromEnv reads PRIVAHUB_VOTE_MONITOR_INTERVAL /
// PRIVAHUB_VOTE_MONITOR_INITIAL_DELAY (Go durations or milliseconds);
// defaults are the Java 1s / 6s.
func MonitorOptionsFromEnv() MonitorOptions {
	parse := func(key string, def time.Duration) time.Duration {
		v := os.Getenv(key)
		if v == "" {
			return def
		}
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
		return def
	}
	return MonitorOptions{
		Interval:     parse("PRIVAHUB_VOTE_MONITOR_INTERVAL", time.Second),
		InitialDelay: parse("PRIVAHUB_VOTE_MONITOR_INITIAL_DELAY", 6*time.Second),
	}
}

// NewMonitor creates the vote monitors.
func NewMonitor(svc *Service, o MonitorOptions) *Monitor {
	if o.Interval <= 0 {
		o.Interval = time.Second
	}
	if o.InitialDelay < 0 {
		o.InitialDelay = 0
	}
	return &Monitor{svc: svc, interval: o.Interval, initialDelay: o.InitialDelay}
}

// Start launches the background loop (idempotent).
func (m *Monitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.done = make(chan struct{})
	go m.loop(ctx, m.done)
}

// Stop stops the loop and waits for the current tick to finish.
func (m *Monitor) Stop() {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.cancel, m.done = nil, nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (m *Monitor) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	select {
	case <-ctx.Done():
		return
	case <-time.After(m.initialDelay):
	}
	for {
		m.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.interval):
		}
	}
}

// Tick runs both monitors once (panics are contained).
func (m *Monitor) Tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			m.svc.d.log().Error("vote monitor panic", zap.Any("panic", r))
		}
	}()
	m.SyncInvites(ctx)
	m.SyncRequests(ctx)
}

func (m *Monitor) findByStatus(ctx context.Context, status int8, execute string) []model.VoteRequestDO {
	var out []model.VoteRequestDO
	q := m.svc.d.DB.WithContext(ctx).Where("status = ?", status)
	if execute != "" {
		q = q.Where("execute_status = ?", execute)
	}
	if err := q.Order("id").Find(&out).Error; err != nil {
		m.svc.d.log().Warn("vote monitor query failed", zap.Error(err))
	}
	return out
}

// SyncRequests is Java VoteRequestStatusMonitor.sync: sign pending requests
// (non-AUTONOMY) and run approved / rejected callbacks.
func (m *Monitor) SyncRequests(ctx context.Context) {
	d := m.svc.d
	if !d.isAutonomy() {
		m.sign(ctx, m.findByStatus(ctx, StatusReviewing, ExecCommitted))
	}
	for _, vr := range m.findByStatus(ctx, StatusApproved, ExecCommitted) {
		vr := vr
		if h, err := m.svc.Handler(vr.Type); err == nil {
			h.DoCallBackApproved(ctx, &vr)
		}
	}
	for _, vr := range m.findByStatus(ctx, StatusRejected, ExecCommitted) {
		vr := vr
		if h, err := m.svc.Handler(vr.Type); err == nil {
			h.DoCallBackRejected(ctx, &vr)
		}
	}
}

// sign adds the initiator's signature + cert chain to unsigned requests
// created on this platform (CENTER: embedded initiators; EDGE: own node).
func (m *Monitor) sign(ctx context.Context, reqs []model.VoteRequestDO) {
	d := m.svc.d
	if d.Kuscia == nil {
		return
	}
	env := d.env()
	for i := range reqs {
		vr := &reqs[i]
		if env.IsCenter() {
			if !env.IsEmbeddedNode(vr.Initiator) {
				continue
			}
		} else if vr.Initiator != env.GetPlatformNodeId() {
			continue
		}
		var msg VoteRequestMessage
		if err := json.Unmarshal([]byte(vr.RequestMsg), &msg); err != nil {
			continue
		}
		notSigned := msg.VoteRequestSignature == "" && len(msg.CertChain) == 0
		teeNodeRoute := vr.Type == VoteTypeNodeRoute && contains(parseStringList(vr.Executors), teeExecutor)
		if !notSigned || teeNodeRoute {
			continue
		}
		kc, err := d.Kuscia.GenerateKeyCerts(ctx, vr.Initiator)
		if err != nil {
			d.log().Warn("vote sign: generate key certs failed", zap.String("vote", vr.VoteID), zap.Error(err))
			continue
		}
		sig, err := SignSHA256withRSA([]byte(msg.Body), kc.Key)
		if err != nil {
			d.log().Warn("vote sign failed", zap.String("vote", vr.VoteID), zap.Error(err))
			continue
		}
		msg.CertChain = certChainToPEM(kc.CertChain)
		msg.VoteRequestSignature = sig
		vr.RequestMsg = toJSON(msg)
		if env.IsCenter() {
			if err := d.DB.WithContext(ctx).Model(&model.VoteRequestDO{}).Where("vote_id = ?", vr.VoteID).
				Update("request_msg", vr.RequestMsg).Error; err != nil {
				d.log().Warn("vote sign: save failed", zap.String("vote", vr.VoteID), zap.Error(err))
			}
			continue
		}
		if err := m.svc.syncSender().SendToCenter(ctx, DbSyncRequest{SyncDataType: SyncVoteRequest, ProjectNodesInfo: vr}); err != nil {
			d.log().Warn("vote sign: sync to center failed", zap.String("vote", vr.VoteID), zap.Error(err))
		}
	}
}

var (
	autonomyVoteTypes = []string{VoteTypeProjectCreate, VoteTypeProjectArchive}
	centerVoteTypes   = []string{VoteTypeTeeDownload, VoteTypeNodeRoute}
)

// SyncInvites is Java VoteInviteStatusMonitor.sync: verify reply signatures
// and aggregate invite actions into the request status.
func (m *Monitor) SyncInvites(ctx context.Context) {
	d := m.svc.d
	db := d.DB.WithContext(ctx)
	for _, vr := range m.findByStatus(ctx, StatusReviewing, "") {
		vr := vr
		var invites []model.VoteInviteDO
		db.Where("vote_id = ?", vr.VoteID).Order("id").Find(&invites)
		if len(invites) == 0 {
			continue
		}
		executors := parseStringList(vr.Executors)
		switch {
		case contains(centerVoteTypes, vr.Type):
			if !(vr.Type == VoteTypeNodeRoute && contains(executors, teeExecutor)) {
				m.verifyInvites(ctx, &vr, invites)
			}
		case contains(autonomyVoteTypes, vr.Type):
			if d.isCurrentInst(ctx, vr.Initiator) {
				var nodeIDs []string
				db.Model(&model.NodeDO{}).Where("inst_id = ?", vr.Initiator).Pluck("node_id", &nodeIDs)
				for _, e := range executors {
					if contains(nodeIDs, e) {
						m.verifyInvites(ctx, &vr, invites)
						break
					}
				}
			}
		}
		// re-read the invites: verification may have rejected one
		db.Where("vote_id = ?", vr.VoteID).Order("id").Find(&invites)
		anyRejected, allApproved := false, true
		for _, inv := range invites {
			if inv.Action == ActionRejected {
				anyRejected = true
			}
			if inv.Action != ActionApproved {
				allApproved = false
			}
		}
		status := vr.Status
		if anyRejected {
			status = StatusRejected
		} else if allApproved {
			status = StatusApproved
		}
		if status != vr.Status {
			if err := db.Model(&model.VoteRequestDO{}).Where("vote_id = ? AND status = ?", vr.VoteID, StatusReviewing).
				Update("status", status).Error; err != nil {
				d.log().Warn("vote monitor: update status failed", zap.String("vote", vr.VoteID), zap.Error(err))
			}
		}
	}
}

// verifyInvites rejects the vote at the first replied invite whose signature
// does not verify.
func (m *Monitor) verifyInvites(ctx context.Context, vr *model.VoteRequestDO, invites []model.VoteInviteDO) {
	d := m.svc.d
	if d.Kuscia == nil {
		return
	}
	for i := range invites {
		inv := &invites[i]
		if inv.Action == ActionReviewing || m.VerifyReply(ctx, inv, vr) {
			continue
		}
		msg := fmt.Sprintf("in voteID-> %s,voteInvite participant %s verify fail", inv.VoteID, inv.VoteParticipantID)
		d.log().Info(msg)
		inv.Reason = "verify fail-> " + inv.VoteParticipantID
		inv.Action = ActionRejected
		infos := upsertPartyVoteInfo(parsePartyVoteInfos(vr.PartyVoteInfo),
			PartyVoteInfo{PartyID: inv.VoteParticipantID, Action: inv.Action, Reason: inv.Reason})
		vr.Status = StatusRejected
		vr.Msg = msg
		vr.PartyVoteInfo = toJSON(infos)
		db := d.DB.WithContext(ctx)
		db.Model(&model.VoteRequestDO{}).Where("vote_id = ?", vr.VoteID).Updates(map[string]interface{}{
			"status": StatusRejected, "msg": msg, "party_vote_info": vr.PartyVoteInfo,
		})
		db.Model(&model.VoteInviteDO{}).Where("vote_id = ? AND vote_participant_id = ?", inv.VoteID, inv.VoteParticipantID).
			Updates(map[string]interface{}{"action": inv.Action, "reason": inv.Reason})
		return
	}
}

// VerifyReply is Java VoteInviteStatusMonitor.verify: the reply signature over
// (replyBody + requestSignature) must verify with the reply's leaf cert, whose
// chain root must carry the voter's domain public key.
func (m *Monitor) VerifyReply(ctx context.Context, inv *model.VoteInviteDO, vr *model.VoteRequestDO) bool {
	var reply VoteReplyMessage
	if err := json.Unmarshal([]byte(inv.VoteMsg), &reply); err != nil || len(reply.CertChain) < 2 {
		return false
	}
	var req VoteRequestMessage
	if err := json.Unmarshal([]byte(vr.RequestMsg), &req); err != nil {
		return false
	}
	if VerifySHA256withRSA([]byte(reply.Body+req.VoteRequestSignature), reply.CertChain[0], reply.Signature) != nil {
		return false
	}
	raw, err := unb64(reply.Body)
	if err != nil {
		return false
	}
	var body VoteReplyBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return false
	}
	if m.svc.d.Kuscia == nil {
		return true
	}
	cert := m.svc.d.Kuscia.DomainCert(ctx, body.Voter, "")
	if !CompareCertPubKey(cert, reply.CertChain[1]) {
		return false
	}
	return ValidateCertChain([]string{reply.CertChain[0], reply.CertChain[1]})
}
