package votehandler

import (
	"errors"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
)

func p2pNodeReq() *P2PCreateNodeRequest {
	return &P2PCreateNodeRequest{Name: "peer", CertText: "PEER-CERT", DstNodeID: "b1", SrcNodeID: "a1",
		SrcNetAddress: "http://a1:1080", DstNetAddress: "http://b1:1080", DstInstID: "instB", DstInstName: "Inst B"}
}

func TestP2PNode_CreateAndDelete(t *testing.T) {
	e := newTestEnv(t, "autonomy", "a1", "instA")
	e.mustCreate(t, &model.InstDO{InstID: "instA", Name: "Inst A"})
	e.mustCreate(t, &model.NodeDO{NodeID: "a1", Name: "A1", ControlNodeID: "a1", InstID: "instA"})
	ctx := userCtx("instA", auth.OwnerTypeP2P, auth.PlatformTypeAutonomy)

	id, err := e.p2p.CreateNode(ctx, p2pNodeReq())
	if err != nil || id != "b1" {
		t.Fatalf("create: %v %s", err, id)
	}
	var node model.NodeDO
	if err := e.db.Where("node_id = ?", "b1").First(&node).Error; err != nil {
		t.Fatal(err)
	}
	if node.InstID != "instB" || node.NetAddress != "http://b1:1080" || node.MasterNodeID != "a1" || node.Type != "normal" {
		t.Errorf("node = %+v", node)
	}
	var inst model.InstDO
	if e.db.Where("inst_id = ?", "instB").First(&inst).Error != nil || inst.Name != "Inst B" {
		t.Error("inst not created")
	}
	var perm int64
	e.db.Model(&model.SysUserPermissionRelDO{}).Where("user_key = ? AND target_code = ?", "b1", auth.RoleP2PNode).Count(&perm)
	if perm != 1 {
		t.Error("P2P_NODE permission missing")
	}
	if e.kuscia.domains["a1/b1"] != "PEER-CERT" {
		t.Errorf("partner domain = %+v", e.kuscia.domains)
	}
	if len(e.routes.reqs) != 1 || e.routes.reqs[0].SrcNodeID != "a1" || e.routes.reqs[0].DstNodeID != "b1" || e.routes.reqs[0].RouteType != "" {
		t.Fatalf("route requests = %+v", e.routes.reqs)
	}
	if _, err := e.p2p.CreateNode(ctx, p2pNodeReq()); bizCode(err) != errcode.NodeCreateError.Code {
		t.Errorf("duplicate node: %v", err)
	}

	// running job blocks deletion
	e.mustCreate(t, &model.ProjectDO{ProjectID: "p1", Name: "P", Status: ProjectApproved})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "a1"})
	e.mustCreate(t, &model.ProjectNodeDO{ProjectID: "p1", NodeID: "b1"})
	job := &model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "j", Status: "RUNNING"}
	e.mustCreate(t, job)
	// the fake route creator stored a1__b1 (Java stores the dst-owned route id too)
	e.mustCreate(t, &model.NodeRouteDO{RouteID: "b1__a1", SrcNodeID: "b1", DstNodeID: "a1"})
	if err := e.p2p.DeleteNodeRoute(ctx, "b1__a1"); bizCode(err) != errcode.NodeRouteUpdateError.Code {
		t.Fatalf("running job: %v", err)
	}
	e.db.Model(job).Update("status", "SUCCEEDED")
	e.kuscia.routes["a1/b1/a1"] = true
	if err := e.p2p.DeleteNodeRoute(ctx, "b1__a1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int64
	e.db.Unscoped().Model(&model.NodeDO{}).Where("node_id = ?", "b1").Count(&n)
	if n != 0 {
		t.Error("peer node not deleted")
	}
	e.db.Model(&model.SysUserPermissionRelDO{}).Where("user_key = ?", "b1").Count(&n)
	if n != 0 {
		t.Error("peer permission not deleted")
	}
	want := map[string]bool{"route:a1/b1/a1": true, "domain:a1/b1": true}
	for _, d := range e.kuscia.deleted {
		delete(want, d)
	}
	if len(want) != 0 {
		t.Errorf("kuscia deletions %v missing %v", e.kuscia.deleted, want)
	}
	if err := e.p2p.DeleteNodeRoute(ctx, "b1__a1"); bizCode(err) != errcode.NodeRouteNotExistError.Code {
		t.Errorf("deleted route: %v", err)
	}
}

func TestP2PNode_Validation(t *testing.T) {
	e := newTestEnv(t, "autonomy", "a1", "instA")
	ctx := userCtx("instA", auth.OwnerTypeP2P, auth.PlatformTypeAutonomy)
	bad := func(mut func(r *P2PCreateNodeRequest)) error {
		r := p2pNodeReq()
		mut(r)
		_, err := e.p2p.CreateNode(ctx, r)
		return err
	}
	mode := 5
	for name, c := range map[string]struct {
		mut  func(r *P2PCreateNodeRequest)
		code int
	}{
		"blank cert":        {func(r *P2PCreateNodeRequest) { r.CertText = "" }, errcode.SystemValidationError.Code},
		"no scheme":         {func(r *P2PCreateNodeRequest) { r.DstNetAddress = "b1:1080" }, errcode.SystemValidationError.Code},
		"no port":           {func(r *P2PCreateNodeRequest) { r.DstNetAddress = "http://b1" }, errcode.SystemValidationError.Code},
		"bad mode":          {func(r *P2PCreateNodeRequest) { r.Mode = &mode }, errcode.SystemValidationError.Code},
		"bad name":          {func(r *P2PCreateNodeRequest) { r.Name = "a b" }, errcode.SystemValidationError.Code},
		"same address":      {func(r *P2PCreateNodeRequest) { r.SrcNetAddress = r.DstNetAddress }, errcode.NodeRouteConfigError.Code},
		"self as dst":       {func(r *P2PCreateNodeRequest) { r.DstNodeID = "a1" }, errcode.NodeCertConfigError.Code},
		"own cert":          {func(r *P2PCreateNodeRequest) { r.CertText = b64(e.kuscia.def.caPEM) }, errcode.NodeCertConfigError.Code},
		"missing srcNodeId": {func(r *P2PCreateNodeRequest) { r.SrcNodeID = "" }, errcode.SystemValidationError.Code},
	} {
		if err := bad(c.mut); bizCode(err) != c.code {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestP2PNode_RouteFailureRollsBack(t *testing.T) {
	e := newTestEnv(t, "autonomy", "a1", "instA")
	e.routes.err = errors.New("kuscia route failed")
	ctx := userCtx("instA", auth.OwnerTypeP2P, auth.PlatformTypeAutonomy)
	if _, err := e.p2p.CreateNode(ctx, p2pNodeReq()); err == nil {
		t.Fatal("expected error")
	}
	var n int64
	e.db.Unscoped().Model(&model.NodeDO{}).Where("node_id = ?", "b1").Count(&n)
	if n != 0 {
		t.Error("node not rolled back")
	}
	if _, ok := e.kuscia.domains["a1/b1"]; ok {
		t.Error("kuscia domain not rolled back")
	}
}

func TestP2PNode_DeleteRefusedWhileProjectReviewing(t *testing.T) {
	e := newTestEnv(t, "autonomy", "a1", "instA")
	e.mustCreate(t, &model.NodeDO{NodeID: "a1", Name: "A1", ControlNodeID: "a1", InstID: "instA"})
	e.mustCreate(t, &model.NodeRouteDO{RouteID: "b1__a1", SrcNodeID: "b1", DstNodeID: "a1"})
	e.mustCreate(t, &model.ProjectDO{ProjectID: "p1", Name: "P", Status: ProjectReviewing})
	e.mustCreate(t, &model.ProjectApprovalConfigDO{VoteID: "v1", ProjectID: "p1", Type: VoteTypeProjectCreate, Initiator: "instA", Parties: `["b1"]`})
	ctx := userCtx("instA", auth.OwnerTypeP2P, auth.PlatformTypeAutonomy)
	if err := e.p2p.DeleteNodeRoute(ctx, "b1__a1"); bizCode(err) != errcode.NodeDeleteError.Code {
		t.Errorf("expected NODE_DELETE_ERROR, got %v", err)
	}
}

func TestMonitorStartStop(t *testing.T) {
	e := newTestEnv(t, "center", "kuscia-system", "")
	seedCenterNodes(t, e)
	voteID, err := e.svc.CreateApproval(centerCtx(), "alice", VoteTypeNodeRoute, &VoteConfig{SrcNodeID: "alice", DesNodeID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	e.mon.Start()
	e.mon.Start() // idempotent
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if msg, _, _ := parseRequest(e.request(t, voteID).RequestMsg); msg.VoteRequestSignature != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.mon.Stop()
	e.mon.Stop()
	if msg, _, _ := parseRequest(e.request(t, voteID).RequestMsg); msg.VoteRequestSignature == "" {
		t.Fatal("background monitor did not sign the request")
	}
}

func TestMonitorOptionsFromEnv(t *testing.T) {
	t.Setenv("PRIVAHUB_VOTE_MONITOR_INTERVAL", "250")
	t.Setenv("PRIVAHUB_VOTE_MONITOR_INITIAL_DELAY", "2s")
	o := MonitorOptionsFromEnv()
	if o.Interval != 250*time.Millisecond || o.InitialDelay != 2*time.Second {
		t.Errorf("options = %+v", o)
	}
}
