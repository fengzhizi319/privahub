package votehandler

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

// Autonomy world: this platform is institution instA (node a1); the peer is
// instB (node b1). Both live in the same DB, as after P2P data sync.
func newAutonomyEnv(t *testing.T) *testEnv {
	e := newTestEnv(t, "autonomy", "a1", "instA")
	e.mustCreate(t, &model.InstDO{InstID: "instA", Name: "Inst A"})
	e.mustCreate(t, &model.InstDO{InstID: "instB", Name: "Inst B"})
	e.mustCreate(t, &model.NodeDO{NodeID: "a1", Name: "A1", ControlNodeID: "a1", InstID: "instA"})
	e.mustCreate(t, &model.NodeDO{NodeID: "b1", Name: "B1", ControlNodeID: "b1", InstID: "instB"})
	return e
}

func ctxA() context.Context { return userCtx("instA", auth.OwnerTypeP2P, auth.PlatformTypeAutonomy) }
func ctxB() context.Context { return userCtx("instB", auth.OwnerTypeP2P, auth.PlatformTypeAutonomy) }

func createCfg(projectID string) *VoteConfig {
	return &VoteConfig{ProjectID: projectID, Participants: []string{"instA", "instB"},
		ParticipantNodeInstVOS: []ParticipantNodeInstVO{{InitiatorNodeID: "a1", Invitees: []NodeInstVO{{InviteeID: "b1"}}}}}
}

func (e *testEnv) project(t *testing.T, id string) model.ProjectDO {
	t.Helper()
	var p model.ProjectDO
	if err := e.db.Where("project_id = ?", id).First(&p).Error; err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProjectCreateAndArchive_FullFlow(t *testing.T) {
	e := newAutonomyEnv(t)
	pid, err := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "joint", Description: "d", ComputeMode: "mpc", ComputeFunc: "PSI"})
	if err != nil {
		t.Fatal(err)
	}
	if p := e.project(t, pid); p.Status != ProjectReviewing || p.OwnerID != "instA" || p.ComputeMode != "MPC" || p.ComputeFunc != "PSI" {
		t.Fatalf("project = %+v", p)
	}
	// identity verification: instB cannot initiate for instA
	if _, err := e.svc.CreateApproval(ctxB(), "instA", VoteTypeProjectCreate, createCfg(pid)); bizCode(err) != errcode.VoteCheckFailed.Code {
		t.Fatalf("identity check: %v", err)
	}
	voteID, err := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectCreate, createCfg(pid))
	if err != nil {
		t.Fatalf("create approval: %v", err)
	}
	vr := e.request(t, voteID)
	msg, body, _ := parseRequest(vr.RequestMsg)
	if msg.VoteRequestSignature == "" || len(msg.CertChain) != 2 || body.Executors[0] != "a1" || vr.Description != "joint" {
		t.Fatalf("request not signed at creation: %+v %+v", msg, body)
	}
	if infos := parsePartyVoteInfos(vr.PartyVoteInfo); len(infos) != 2 {
		t.Errorf("party infos = %+v", infos)
	}

	// p2p list + participants + message detail
	list, err := e.p2p.ListProjects(ctxA())
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v %v", list, err)
	}
	pv := list[0]
	if pv.VoteID != voteID || pv.Status != "REVIEWING" || pv.InitiatorName != "Inst A" || len(pv.PartyVoteInfos) != 1 ||
		pv.PartyVoteInfos[0].PartyID != "instB" || pv.PartyVoteInfos[0].PartyName != "Inst B" || pv.PartyVoteInfos[0].Action != ActionReviewing {
		t.Errorf("project vo = %+v", pv)
	}
	parts, err := e.p2p.Participants(ctxA(), voteID)
	if err != nil {
		t.Fatal(err)
	}
	if parts.InitiatorName != "Inst A" || parts.ParticipantNodeInstVOS[0].InitiatorNodeName != "A1" ||
		parts.ParticipantNodeInstVOS[0].Invitees[0].InstName != "Inst B" || parts.ParticipantNodeInstVOS[0].Invitees[0].InviteeName != "B1" {
		t.Errorf("participants = %+v", parts)
	}
	d, err := e.svc.Detail(ctxB(), false, "instB", voteID, VoteTypeProjectCreate)
	if err != nil {
		t.Fatalf("detail (signature verification): %v", err)
	}
	if pd := d.(*ProjectCreateMessageDetail); pd.ProjectName != "joint" || pd.Type != VoteTypeProjectCreate || pd.ComputeFunc != "PSI" || len(pd.PartyVoteStatuses) != 2 {
		t.Errorf("detail = %+v", pd)
	}
	l, _ := e.svc.List(ctxB(), &MessageListRequest{OwnerID: "instB"})
	if l.Total != 1 || l.Messages[0].InitiatingTypeMessage.(*ReceiverMessage).InitiatorNodeName != "Inst A" {
		t.Errorf("receiver list = %+v", l)
	}
	if info := l.Messages[0].VoteTypeMessage.(*ProjectApprovalCustomizedMessage); info.ProjectID != pid || info.ComputeFunc != "PSI" {
		t.Errorf("list info = %+v", info)
	}

	// reply as instB
	if err := e.svc.Reply(ctxA(), ActionApproved, "", "instB", voteID); bizCode(err) != errcode.VoteCheckFailed.Code {
		t.Errorf("reply as someone else: %v", err)
	}
	if err := e.svc.Reply(ctxB(), ActionApproved, "fine", "instB", voteID); err != nil {
		t.Fatalf("reply: %v", err)
	}
	for _, i := range parsePartyVoteInfos(e.request(t, voteID).PartyVoteInfo) {
		if i.Action != ActionApproved {
			t.Errorf("party info not updated: %+v", i)
		}
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	vr = e.request(t, voteID)
	if vr.Status != StatusApproved || vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("create vote: status=%d exec=%s msg=%s", vr.Status, vr.ExecuteStatus, vr.Msg)
	}
	if p := e.project(t, pid); p.Status != ProjectApproved {
		t.Fatalf("project status = %d", p.Status)
	}
	var insts, nodes int64
	e.db.Model(&model.ProjectInstDO{}).Where("project_id = ?", pid).Count(&insts)
	e.db.Model(&model.ProjectNodeDO{}).Where("project_id = ?", pid).Count(&nodes)
	if insts != 2 || nodes != 2 {
		t.Fatalf("members insts=%d nodes=%d", insts, nodes)
	}
	// owner-only archive of a REVIEWING project is refused now
	if err := e.p2p.ArchiveProject(ctxA(), pid); bizCode(err) != errcode.ProjectCanNotArchive.Code {
		t.Errorf("direct archive of approved project: %v", err)
	}

	// PROJECT_ARCHIVE vote
	archiveID, err := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectArchive, &VoteConfig{ProjectID: pid})
	if err != nil {
		t.Fatalf("archive approval: %v", err)
	}
	if _, err := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectArchive, &VoteConfig{ProjectID: pid}); bizCode(err) != errcode.VoteProjectArchiveVoteAlreadyExist.Code {
		t.Errorf("second archive vote: %v", err)
	}
	st, err := e.svc.handlers[VoteTypeProjectArchive].GetPartyStatusByVoteID(context.Background(), archiveID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if s.ProjectCreateVoteAction != ActionApproved {
			t.Errorf("archive party status = %+v", s)
		}
	}
	if _, err := e.svc.Detail(ctxB(), false, "instB", archiveID, VoteTypeProjectArchive); err != nil {
		t.Fatalf("archive detail: %v", err)
	}
	if err := e.svc.Reply(ctxB(), ActionApproved, "", "instB", archiveID); err != nil {
		t.Fatal(err)
	}
	// flushVoteStatus archived the project immediately
	if p := e.project(t, pid); p.Status != ProjectArchived {
		t.Fatalf("flushVoteStatus: project status = %d", p.Status)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	if vr := e.request(t, archiveID); vr.Status != StatusApproved || vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("archive vote: %+v", vr)
	}
}

func TestProjectCreate_Rejected(t *testing.T) {
	e := newAutonomyEnv(t)
	pid, _ := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "p"})
	voteID, err := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectCreate, createCfg(pid))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Reply(ctxB(), ActionRejected, "no", "instB", voteID); err != nil {
		t.Fatal(err)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	if vr := e.request(t, voteID); vr.Status != StatusRejected || vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("vote = %+v", vr)
	}
	if p := e.project(t, pid); p.Status != ProjectArchived {
		t.Errorf("project status = %d", p.Status)
	}
	list, _ := e.p2p.ListProjects(ctxA())
	if len(list) != 1 || list[0].Status != "ARCHIVED" || list[0].PartyVoteInfos[0].Action != ActionRejected {
		t.Errorf("list = %+v", list)
	}
}

func TestProjectCreate_NotInitiatorSkipsCallback(t *testing.T) {
	e := newAutonomyEnv(t)
	pid, _ := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "p"})
	voteID, _ := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectCreate, createCfg(pid))
	e.svc.d.Env.SetInstID("instB") // we are now the peer
	e.db.Model(&model.VoteRequestDO{}).Where("vote_id = ?", voteID).Update("status", StatusApproved)
	e.mon.SyncRequests(context.Background())
	if vr := e.request(t, voteID); vr.ExecuteStatus != ExecCommitted {
		t.Errorf("peer executed the callback: %s", vr.ExecuteStatus)
	}
}

func TestProjectCreate_PreChecks(t *testing.T) {
	e := newAutonomyEnv(t)
	e.mustCreate(t, &model.NodeDO{NodeID: "a2", Name: "A2", ControlNodeID: "a2", InstID: "instA"})
	pid, _ := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "p"})
	cases := []struct {
		name string
		cfg  *VoteConfig
		code int
	}{
		{"no participants", &VoteConfig{ProjectID: pid, Participants: []string{"instA"}}, errcode.VoteParticipantNotExist.Code},
		{"duplicate initiator node", &VoteConfig{ProjectID: pid, Participants: []string{"instA", "instB"},
			ParticipantNodeInstVOS: []ParticipantNodeInstVO{{InitiatorNodeID: "a1"}, {InitiatorNodeID: "a1"}}}, errcode.VoteInitiatorNodeDuplicate.Code},
		{"initiator node of other inst", &VoteConfig{ProjectID: pid, Participants: []string{"instA", "instB"},
			ParticipantNodeInstVOS: []ParticipantNodeInstVO{{InitiatorNodeID: "b1"}}}, errcode.InstInitiatorInstNodeMismatch.Code},
		{"invitee of own inst", &VoteConfig{ProjectID: pid, Participants: []string{"instA", "instB"},
			ParticipantNodeInstVOS: []ParticipantNodeInstVO{{InitiatorNodeID: "a1", Invitees: []NodeInstVO{{InviteeID: "a2"}}}}}, errcode.InstInviteeInstNodeMismatch.Code},
		{"unknown project", createCfg("nope"), errcode.ProjectNotExists.Code},
	}
	for _, c := range cases {
		if _, err := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectCreate, c.cfg); bizCode(err) != c.code {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectArchive, &VoteConfig{ProjectID: pid}); bizCode(err) != errcode.ProjectCanNotCreateArchiveVote.Code {
		t.Errorf("archive of reviewing project: %v", err)
	}
}

func TestProjectDetail_SignatureVerification(t *testing.T) {
	e := newAutonomyEnv(t)
	pid, _ := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "p"})
	voteID, _ := e.svc.CreateApproval(ctxA(), "instA", VoteTypeProjectCreate, createCfg(pid))
	// the executor's domain cert no longer matches the signing chain
	e.kuscia.pki["a1"] = newTestPKI(t, "other")
	if _, err := e.svc.Detail(ctxA(), true, "instA", voteID, VoteTypeProjectCreate); bizCode(err) != errcode.SystemVerifySignatureError.Code {
		t.Errorf("expected verify error, got %v", err)
	}
}

func TestP2PProject_UpdateArchive(t *testing.T) {
	e := newAutonomyEnv(t)
	pid, _ := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "p", Description: "d"})
	if err := e.p2p.UpdateProject(ctxB(), pid, "x", ""); bizCode(err) != errcode.ProjectUpdateFail.Code {
		t.Errorf("non-owner update: %v", err)
	}
	if err := e.p2p.UpdateProject(ctxA(), pid, "renamed", ""); err != nil {
		t.Fatal(err)
	}
	if p := e.project(t, pid); p.Name != "renamed" || p.Description != "d" {
		t.Errorf("project = %+v", p)
	}
	if err := e.p2p.ArchiveProject(ctxB(), pid); bizCode(err) != errcode.ProjectArchiveFail.Code {
		t.Errorf("non-owner archive: %v", err)
	}
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: pid, NodeID: "a1"})
	e.mustCreate(t, &model.ProjectInstDO{ProjectID: pid, InstID: "instA"})
	if err := e.p2p.ArchiveProject(ctxA(), pid); err != nil {
		t.Fatal(err)
	}
	if p := e.project(t, pid); p.Status != ProjectArchived {
		t.Errorf("status = %d", p.Status)
	}
	var n int64
	e.db.Unscoped().Model(&model.ProjectNodeDO{}).Where("project_id = ?", pid).Count(&n)
	if n != 0 {
		t.Error("project nodes not deleted")
	}
	if err := e.p2p.ArchiveProject(ctxA(), "missing"); bizCode(err) != errcode.ProjectNotExists.Code {
		t.Errorf("missing project: %v", err)
	}
	if _, err := e.p2p.CreateProject(ctxA(), &P2PCreateProjectRequest{Name: "t", ComputeMode: "TEE"}); bizCode(err) != errcode.SystemValidationError.Code {
		t.Errorf("tee without node: %v", err)
	}
	if _, err := e.p2p.CreateProject(context.Background(), &P2PCreateProjectRequest{Name: "t"}); bizCode(err) != errcode.AuthFailed.Code {
		t.Errorf("anonymous create: %v", err)
	}
}

func TestProjectNodeAdd(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	e.mustCreate(t, &model.NodeDO{NodeID: "carol", Name: "Carol", ControlNodeID: "carol"})
	e.mustCreate(t, &model.ProjectDO{ProjectID: "p1", Name: "Proj"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	ctx := centerCtx()
	if _, err := e.svc.CreateApproval(ctx, "alice", VoteTypeProjectNodeAdd, &VoteConfig{ProjectID: "p1"}); bizCode(err) != errcode.ParamError.Code {
		t.Errorf("missing inviteNodeId: %v", err)
	}
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeProjectNodeAdd, &VoteConfig{ProjectID: "p1", InviteNodeID: "carol"})
	if err != nil {
		t.Fatal(err)
	}
	if vr := e.request(t, voteID); vr.Voters != `["bob"]` {
		t.Errorf("voters = %s", vr.Voters)
	}
	d, err := e.svc.Detail(ctx, false, "bob", voteID, VoteTypeProjectNodeAdd)
	if err != nil || d.(*ProjectCreateMessageDetail).ParticipantNodeInstVOS[0].Invitees[0].InviteeName != "Carol" {
		t.Errorf("detail = %+v %v", d, err)
	}
	e.mon.SyncRequests(context.Background())
	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	if vr := e.request(t, voteID); vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("vote = %+v", vr)
	}
	var n int64
	e.db.Model(&model.ProjectNodeDO{}).Where("project_id = ? AND node_id = ?", "p1", "carol").Count(&n)
	if n != 1 {
		t.Error("carol not added to project")
	}
}
