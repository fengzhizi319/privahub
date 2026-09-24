package votehandler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

func seedCenterNodes(t *testing.T, e *testEnv) {
	e.mustCreate(t, &model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice", NetAddress: "alice:1080"})
	e.mustCreate(t, &model.NodeDO{NodeID: "bob", Name: "Bob", ControlNodeID: "bob", NetAddress: "bob:1080"})
}

var centerCtx = func() context.Context { return userCtx("kuscia-system", auth.OwnerTypeCenter, auth.PlatformTypeCenter) }

func TestNodeRoute_FullFlow(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	ctx := centerCtx()
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{
		SrcNodeID: "alice", DesNodeID: "bob", SrcNodeAddr: "http://alice:1080", DesNodeAddr: "http://bob:1080"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	vr := e.request(t, voteID)
	if vr.Status != StatusReviewing || vr.ExecuteStatus != ExecCommitted || vr.Description != "Bob" {
		t.Fatalf("request = %+v", vr)
	}
	if vr.Voters != `["bob"]` || vr.Executors != `["alice","bob"]` {
		t.Errorf("voters/executors = %s %s", vr.Voters, vr.Executors)
	}
	msg, body, err := parseRequest(vr.RequestMsg)
	if err != nil || msg.VoteRequestSignature != "" || body.ApprovedAction != "NODE_ROUTE,bob" {
		t.Fatalf("request msg %+v %+v %v", msg, body, err)
	}
	inv := e.invite(t, voteID, "bob")
	if inv.Action != ActionReviewing || inv.Description != "Alice" {
		t.Errorf("invite = %+v", inv)
	}

	// reply before the monitor signed → VOTE_SIGNATURE_SYNCHRONIZING
	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); bizCode(err) != errcode.VoteSignatureSynchronizing.Code {
		t.Fatalf("expected signature synchronizing, got %v", err)
	}
	e.mon.SyncRequests(context.Background()) // signs (alice is embedded)
	msg, _, _ = parseRequest(e.request(t, voteID).RequestMsg)
	if msg.VoteRequestSignature == "" || len(msg.CertChain) != 2 || !strings.Contains(msg.CertChain[0], "BEGIN CERTIFICATE") {
		t.Fatalf("not signed: %+v", msg)
	}
	if err := VerifySHA256withRSA([]byte(msg.Body), msg.CertChain[0], msg.VoteRequestSignature); err != nil {
		t.Fatalf("request signature: %v", err)
	}

	// pending / lists
	if n, _ := e.svc.PendingCount(ctx, "bob"); n != 1 {
		t.Errorf("pending = %d", n)
	}
	recv, err := e.svc.List(ctx, &MessageListRequest{Page: 1, Size: 10, OwnerID: "bob"})
	if err != nil || recv.Total != 1 || len(recv.Messages) != 1 {
		t.Fatalf("receiver list = %+v %v", recv, err)
	}
	rm := recv.Messages[0].InitiatingTypeMessage.(*ReceiverMessage)
	if rm.InitiatorNodeName != "Alice" || recv.Messages[0].Status != ActionReviewing || recv.Messages[0].VoteID != voteID {
		t.Errorf("receiver message = %+v %+v", recv.Messages[0], rm)
	}
	processed := true
	if l, _ := e.svc.List(ctx, &MessageListRequest{OwnerID: "bob", IsProcessed: &processed}); l.Total != 0 {
		t.Errorf("processed list total = %d", l.Total)
	}
	sent, _ := e.svc.List(ctx, &MessageListRequest{OwnerID: "alice", IsInitiator: true, KeyWord: "Bo"})
	if sent.Total != 1 || sent.Messages[0].Status != ActionReviewing {
		t.Fatalf("initiator list = %+v", sent)
	}
	st := sent.Messages[0].InitiatingTypeMessage.(*InitiatorMessage).PartyVoteStatuses
	if len(st) != 1 || st[0].ParticipantID != "bob" || st[0].ParticipantName != "Bob" {
		t.Errorf("party statuses = %+v", st)
	}

	// detail both sides
	d, err := e.svc.Detail(ctx, false, "bob", voteID, VoteTypeNodeRoute)
	if err != nil {
		t.Fatal(err)
	}
	nd := d.(*NodeRouteMessageDetail)
	if nd.URL != "http://alice:1080" || nd.InitiatorNodeName != "Alice" || nd.NodeID != "bob" || nd.MessageName != "Alice" {
		t.Errorf("receiver detail = %+v", nd)
	}
	d, _ = e.svc.Detail(ctx, true, "alice", voteID, "")
	if nd := d.(*NodeRouteMessageDetail); nd.URL != "http://bob:1080" || nd.MessageName != "Bob" || nd.Status != ActionReviewing {
		t.Errorf("initiator detail = %+v", nd)
	}

	// reply: signed with bob's cert
	if err := e.svc.Reply(ctx, "AGREE", "", "bob", voteID); bizCode(err) != errcode.ParamError.Code {
		t.Errorf("invalid action accepted: %v", err)
	}
	if err := e.svc.Reply(ctx, ActionApproved, "ok", "bob", voteID); err != nil {
		t.Fatalf("reply: %v", err)
	}
	inv = e.invite(t, voteID, "bob")
	var reply VoteReplyMessage
	if err := json.Unmarshal([]byte(inv.VoteMsg), &reply); err != nil || reply.Signature == "" {
		t.Fatalf("reply msg = %s", inv.VoteMsg)
	}
	vr = e.request(t, voteID)
	if !e.mon.VerifyReply(context.Background(), &inv, &vr) {
		t.Fatal("reply signature does not verify")
	}

	// monitors: aggregate → APPROVED, callback → both half routes
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	vr = e.request(t, voteID)
	if vr.Status != StatusApproved || vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("after monitor: status=%d exec=%s msg=%s", vr.Status, vr.ExecuteStatus, vr.Msg)
	}
	if len(e.routes.reqs) != 2 {
		t.Fatalf("routes created = %+v", e.routes.reqs)
	}
	for _, r := range e.routes.reqs {
		if r.RouteType != "HalfDuplex" {
			t.Errorf("route type = %s", r.RouteType)
		}
	}
	var n int64
	e.db.Model(&model.NodeRouteDO{}).Where("src_node_id = ? AND dst_node_id = ?", "bob", "alice").Count(&n)
	if n != 1 {
		t.Error("bob->alice route missing")
	}
	if n, _ := e.svc.PendingCount(ctx, "bob"); n != 0 {
		t.Errorf("pending after reply = %d", n)
	}
}

func TestNodeRoute_TamperedReplyRejected(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	ctx := centerCtx()
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	e.mon.SyncRequests(context.Background())
	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	// tamper: rewrite the reply body (APPROVE → REJECT) without re-signing
	inv := e.invite(t, voteID, "bob")
	var reply VoteReplyMessage
	_ = json.Unmarshal([]byte(inv.VoteMsg), &reply)
	reply.Body = b64(`{"vote_request_id":"` + voteID + `","voter":"bob","action":"APPROVE","x":1}`)
	e.db.Model(&model.VoteInviteDO{}).Where("id = ?", inv.ID).Update("vote_msg", toJSON(reply))

	e.mon.SyncInvites(context.Background())
	vr := e.request(t, voteID)
	if vr.Status != StatusRejected || !strings.Contains(vr.Msg, "verify fail") {
		t.Fatalf("tampered vote not rejected: %+v", vr)
	}
	if inv = e.invite(t, voteID, "bob"); inv.Action != ActionRejected {
		t.Errorf("invite action = %s", inv.Action)
	}
	e.mon.SyncRequests(context.Background())
	if vr = e.request(t, voteID); vr.ExecuteStatus != ExecSuccess {
		t.Errorf("rejected callback exec = %s", vr.ExecuteStatus)
	}
	if len(e.routes.reqs) != 0 {
		t.Error("route created for rejected vote")
	}
}

func TestNodeRoute_RejectedReply(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	ctx := centerCtx()
	voteID, _ := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"})
	e.mon.SyncRequests(context.Background())
	if err := e.svc.Reply(ctx, ActionRejected, "no", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	vr := e.request(t, voteID)
	if vr.Status != StatusRejected || vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("status=%d exec=%s", vr.Status, vr.ExecuteStatus)
	}
	if e.invite(t, voteID, "bob").Reason != "no" {
		t.Error("reason not stored")
	}
}

func TestNodeRoute_PreChecks(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	ctx := centerCtx()
	cases := []struct {
		name      string
		initiator string
		cfg       VoteConfig
		code      int
	}{
		{"src must be initiator", "alice", VoteConfig{SrcNodeID: "bob", DesNodeID: "alice"}, errcode.AuthFailed.Code},
		{"same node", "alice", VoteConfig{SrcNodeID: "alice", DesNodeID: "alice"}, errcode.NodeRouteSrcNodeAndDestNodeSame.Code},
		{"unknown initiator", "nobody", VoteConfig{SrcNodeID: "nobody", DesNodeID: "bob"}, errcode.NodeNotExistError.Code},
	}
	for _, c := range cases {
		cfg := c.cfg
		if _, err := e.svc.CreateApproval(ctx, c.initiator, VoteTypeNodeRoute, &cfg); bizCode(err) != c.code {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
	e.mustCreate(t, &model.NodeRouteDO{RouteID: "bob__alice", SrcNodeID: "bob", DstNodeID: "alice"})
	if _, err := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"}); bizCode(err) != errcode.NodeRouteAlreadyExists.Code {
		t.Errorf("existing route: %v", err)
	}
	edge := userCtx("bob", auth.OwnerTypeEdge, auth.PlatformTypeEdge)
	if _, err := e.svc.CreateApproval(edge, "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"}); bizCode(err) != errcode.AuthFailed.Code {
		t.Errorf("edge user acting for another node: %v", err)
	}
	if _, err := e.svc.CreateApproval(ctx, "alice", "UNKNOWN", &VoteConfig{}); bizCode(err) != errcode.ParamError.Code {
		t.Errorf("unknown type: %v", err)
	}
	var count int64
	e.db.Model(&model.VoteRequestDO{}).Count(&count)
	if count != 0 {
		t.Errorf("failed approvals left %d requests", count)
	}
}

func TestNodeRoute_ObserverOnForeignExecutors(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	e.mustCreate(t, &model.NodeDO{NodeID: "edge1", Name: "E1", ControlNodeID: "edge1"})
	e.mustCreate(t, &model.NodeDO{NodeID: "edge2", Name: "E2", ControlNodeID: "edge2"})
	ctx := centerCtx()
	voteID, err := e.svc.CreateApproval(ctx, "edge1", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "edge1", DesNodeID: "edge2"})
	if err != nil {
		t.Fatal(err)
	}
	// edge-initiated: the center does not sign it
	e.mon.SyncRequests(context.Background())
	if msg, _, _ := parseRequest(e.request(t, voteID).RequestMsg); msg.VoteRequestSignature != "" {
		t.Error("center signed an edge vote")
	}
	e.db.Model(&model.VoteRequestDO{}).Where("vote_id = ?", voteID).Update("status", StatusApproved)
	e.mon.SyncRequests(context.Background())
	if vr := e.request(t, voteID); vr.ExecuteStatus != ExecObserver {
		t.Errorf("exec = %s, want OBSERVER", vr.ExecuteStatus)
	}
}

func TestEdgeReplySyncsToCenter(t *testing.T) {
	e := newTestEnv(t, "edge", "bob", "")
	seedCenterNodes(t, e)
	sender := &fakeSender{}
	e.svc.SetDbSyncSender(sender)
	ctx := userCtx("bob", auth.OwnerTypeEdge, auth.PlatformTypeEdge)
	voteID, err := e.svc.CreateApproval(userCtx("alice", auth.OwnerTypeEdge, auth.PlatformTypeEdge), "alice", VoteTypeNodeRoute,
		&VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	// sign as the center would
	vr := e.request(t, voteID)
	msg, _, _ := parseRequest(vr.RequestMsg)
	msg.VoteRequestSignature, _ = SignSHA256withRSA([]byte(msg.Body), e.kuscia.def.keyB64)
	e.db.Model(&model.VoteRequestDO{}).Where("vote_id = ?", voteID).Update("request_msg", toJSON(msg))

	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	if len(sender.reqs) != 1 || sender.reqs[0].SyncDataType != SyncVoteInvite {
		t.Fatalf("sync requests = %+v", sender.reqs)
	}
	if inv := e.invite(t, voteID, "bob"); inv.Action != ActionReviewing {
		t.Error("edge must not write the reply locally when a sender is configured")
	}
}

func TestEdgeMonitorSignsOwnVotesAndSyncs(t *testing.T) {
	e := newTestEnv(t, "edge", "alice", "")
	seedCenterNodes(t, e)
	sender := &fakeSender{}
	e.svc.SetDbSyncSender(sender)
	ctx := userCtx("alice", auth.OwnerTypeEdge, auth.PlatformTypeEdge)
	if _, err := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"}); err != nil {
		t.Fatal(err)
	}
	e.mon.SyncRequests(context.Background())
	if len(sender.reqs) != 1 || sender.reqs[0].SyncDataType != SyncVoteRequest {
		t.Fatalf("sync = %+v", sender.reqs)
	}
	vr := sender.reqs[0].ProjectNodesInfo.(*model.VoteRequestDO)
	if msg, _, _ := parseRequest(vr.RequestMsg); msg.VoteRequestSignature == "" {
		t.Error("synced request not signed")
	}
}

func TestTeeDownload_FullFlow(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	puller := &fakePuller{}
	e.svc.SetTeeResultPuller(puller)
	ctx := centerCtx()
	e.mustCreate(t, &model.ProjectDO{ProjectID: "p1", Name: "Proj", ComputeMode: "TEE"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	e.mustCreate(t, &model.ProjectGraphDO{ProjectID: "p1", GraphID: "g1", Name: "Graph One"})
	e.mustCreate(t, &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "job", Status: "SUCCEEDED", GraphID: "g1"})
	e.mustCreate(t, &model.ProjectJobTaskDO{ProjectID: "p1", JobID: "j1", TaskID: "t1", Parties: `["tee"]`, Status: "SUCCEEDED"})
	e.mustCreate(t, &model.ProjectDatatableDO{ProjectID: "p1", NodeID: "tee", DatatableID: "out1",
		TableConfigs: `[{"colName":"id","colType":"str","colComment":"key"}]`})

	pull := func() *PullStatusVO {
		vo, err := e.svc.PullStatus(ctx, "p1", "j1", "t1", "out1", "table")
		if err != nil {
			t.Fatal(err)
		}
		return vo
	}
	if vo := pull(); vo.GraphID != "g1" || len(vo.Parties) != 2 || vo.Parties[0].Status != ActionNotInitiated {
		t.Fatalf("pull before = %+v", vo)
	}
	cfg := &VoteConfig{JobID: "j1", TaskID: "t1", ResourceType: "table", ResourceID: "out1", ProjectID: "p1", GraphID: "g1"}
	if _, err := e.svc.CreateApproval(ctx, "carol", VoteTypeTeeDownload, cfg); bizCode(err) != errcode.NodeNotExistError.Code {
		t.Errorf("unknown initiator: %v", err)
	}
	e.mustCreate(t, &model.NodeDO{NodeID: "carol", Name: "Carol", ControlNodeID: "carol"})
	if _, err := e.svc.CreateApproval(ctx, "carol", VoteTypeTeeDownload, cfg); bizCode(err) != errcode.ProjectInstNotExists.Code {
		t.Errorf("non project node: %v", err)
	}
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeTeeDownload, cfg)
	if err != nil {
		t.Fatal(err)
	}
	vo := pull()
	if vo.Parties[0].NodeID != "alice" || vo.Parties[0].Status != ActionReviewing || len(vo.Parties[0].VoteInfos) != 1 ||
		vo.Parties[1].Status != ActionNotInitiated {
		t.Fatalf("pull after create = %+v", vo)
	}
	d, err := e.svc.Detail(ctx, false, "bob", voteID, VoteTypeTeeDownload)
	if err != nil {
		t.Fatal(err)
	}
	td := d.(*TeeDownloadMessageDetail)
	// Java: graphName / graphDetailVO come from projectService.getProjectJob (job name + job graph).
	if td.GraphDetailVO == nil || td.GraphDetailVO.GraphID != "g1" || td.GraphDetailVO.ProjectID != "p1" ||
		td.GraphDetailVO.Name != "job" || len(td.GraphDetailVO.Nodes) != 1 || td.GraphDetailVO.Nodes[0].TaskID != "t1" {
		t.Errorf("tee detail graphDetailVO = %+v", td.GraphDetailVO)
	}
	if td.GraphName != "job" || td.Project.ProjectName != "Proj" || len(td.TableColumns) != 1 ||
		td.TableColumns[0].ColName != "id" || td.TaskID != "t1" || td.Parties[0] != "tee" || td.MessageName != "out1" {
		t.Errorf("tee detail = %+v", td)
	}
	l, _ := e.svc.List(ctx, &MessageListRequest{OwnerID: "bob"})
	if info, ok := l.Messages[0].VoteTypeMessage.(*TeeListCustomizedMessage); !ok || info.ProjectID != "p1" {
		t.Errorf("list info = %+v", l.Messages[0].VoteTypeMessage)
	}

	e.mon.SyncRequests(context.Background())
	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	vr := e.request(t, voteID)
	if vr.Status != StatusApproved || vr.ExecuteStatus != ExecSuccess {
		t.Fatalf("status=%d exec=%s msg=%s", vr.Status, vr.ExecuteStatus, vr.Msg)
	}
	if len(puller.calls) != 1 || !strings.HasPrefix(puller.calls[0], "alice/out1/") || !strings.Contains(puller.calls[0], "vote_invite") {
		t.Fatalf("puller calls = %v", puller.calls)
	}
	if vo := pull(); vo.Parties[0].Status != ActionApproved || vo.Parties[0].VoteInfos[0].Action != ActionApproved {
		t.Errorf("pull after approval = %+v", vo)
	}
}

func TestOfflineModeSkipsSigning(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	e.svc.d.Kuscia = nil
	seedCenterNodes(t, e)
	ctx := centerCtx()
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); err != nil {
		t.Fatalf("offline reply: %v", err)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	if vr := e.request(t, voteID); vr.Status != StatusApproved || vr.ExecuteStatus != ExecSuccess {
		t.Errorf("offline flow: %+v", vr)
	}
}

// service.TeePullService is the production TeeResultPuller.
var _ TeeResultPuller = (*service.TeePullService)(nil)

func TestPullStatus_ReadsTeePullRecord(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	if err := e.db.AutoMigrate(&model.TeeNodeDatatableManagementDO{}); err != nil {
		t.Fatal(err)
	}
	seedCenterNodes(t, e)
	puller := &fakePuller{}
	e.svc.SetTeeResultPuller(puller)
	ctx := centerCtx()
	e.mustCreate(t, &model.ProjectDO{ProjectID: "p1", Name: "Proj", ComputeMode: "TEE"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	e.mustCreate(t, &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "job", Status: "SUCCEEDED", GraphID: "g1"})
	voteID, err := e.svc.CreateApproval(ctx, "alice", VoteTypeTeeDownload,
		&VoteConfig{JobID: "j1", TaskID: "t1", ResourceType: "table", ResourceID: "j1-out", ProjectID: "p1", GraphID: "g1"})
	if err != nil {
		t.Fatal(err)
	}
	e.mon.SyncRequests(context.Background())
	if err := e.svc.Reply(ctx, ActionApproved, "", "bob", voteID); err != nil {
		t.Fatal(err)
	}
	e.mon.Tick(context.Background())
	e.mon.Tick(context.Background())
	if len(puller.calls) != 1 {
		t.Fatalf("puller calls = %v", puller.calls)
	}
	// the record TeePullService.PullResult writes for this approval
	e.mustCreate(t, &model.TeeNodeDatatableManagementDO{NodeID: "alice", TeeNodeID: "tee", DatatableID: service.BuildTeeDatatableID("alice", "j1-out"),
		DatasourceID: "default-data-source", Kind: service.TeeJobKindPull, JobID: "abcd", Status: service.TeeJobStatusSuccess,
		OperateInfo: `{"relativeUri":"alice-j1-out","vote_result":"{}","project_id":"p1","projectJobId":"j1","projectJobTaskId":"t1","resultType":"table"}`})
	vo, err := e.svc.PullStatus(ctx, "p1", "j1", "t1", "j1-out", "table")
	if err != nil {
		t.Fatal(err)
	}
	a := vo.Parties[0]
	if a.NodeID != "alice" || a.Status != ActionApproved || a.PullDatatableID != "alice-j1-out" || a.PullStatus != service.TeeJobStatusSuccess {
		t.Errorf("alice = %+v", a)
	}
	if b := vo.Parties[1]; b.Status != ActionNotInitiated || b.PullStatus != "" {
		t.Errorf("bob = %+v", b)
	}
}
