package service

import (
	"context"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/graphjob"
)

// isProjectJobRunning (page/get) reports an actual RUNNING job in a project
// shared by both route ends, the condition update/delete enforce.
func TestNodeRoute_IsProjectJobRunning(t *testing.T) {
	f := newGraphFixture(t)
	f.mustCreate(&model.NodeDO{NodeID: "alice", Name: "alice"})
	f.mustCreate(&model.NodeDO{NodeID: "bob", Name: "bob"})
	f.mustCreate(&model.NodeDO{NodeID: "carol", Name: "carol"})
	f.mustCreate(&model.NodeRouteDO{RouteID: "alice__bob", SrcNodeID: "alice", DstNodeID: "bob"})
	f.mustCreate(&model.NodeRouteDO{RouteID: "alice__carol", SrcNodeID: "alice", DstNodeID: "carol"})
	// p1: alice+bob (job finished), p2: alice+carol (job running), p3: bob only (running).
	for _, pn := range [][2]string{{"p1", "alice"}, {"p1", "bob"}, {"p2", "alice"}, {"p2", "carol"}, {"p3", "bob"}} {
		f.mustCreate(&model.ProjectNodeDO{ProjectID: pn[0], NodeID: pn[1]})
	}
	f.mustCreate(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "a", Status: graphjob.JobSucceed})
	f.mustCreate(&model.ProjectJobDO{ProjectID: "p2", JobID: "j2", Name: "b", Status: graphjob.JobRunning})
	f.mustCreate(&model.ProjectJobDO{ProjectID: "p3", JobID: "j3", Name: "c", Status: graphjob.JobRunning})

	ns := NewNodeService(repository.NewNodeRepo(f.db), repository.NewNodeRouteRepo(f.db), nil, f.db)
	ctx := context.Background()
	page, err := ns.Routes().Page(ctx, &PageNodeRouteRequest{Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, vo := range page.List {
		got[vo.SrcNodeID+"__"+vo.DstNodeID] = vo.IsProjectJobRunning
	}
	if got["alice__bob"] || !got["alice__carol"] {
		t.Fatalf("page isProjectJobRunning = %v, want alice__bob=false alice__carol=true", got)
	}
	for route, want := range map[string]bool{"alice__bob": false, "alice__carol": true} {
		vo, err := ns.Routes().Get(ctx, route)
		if err != nil {
			t.Fatal(err)
		}
		if vo.IsProjectJobRunning != want {
			t.Fatalf("get %s isProjectJobRunning = %v, want %v", route, vo.IsProjectJobRunning, want)
		}
	}
}
