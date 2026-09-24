package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func seedNodeResults(f *graphFixture) {
	f.mustCreate(&model.ProjectDO{ProjectID: "p1", Name: "risk-project", ComputeMode: "MPC"})
	f.mustCreate(&model.ProjectJobDO{ProjectID: "p1", JobID: "job1", Name: "train-flow-A", Status: "SUCCEED"})
	f.mustCreate(&model.ProjectResultDO{ProjectID: "p1", Kind: "FedTable", NodeID: "alice", RefID: "out-aaa", JobID: "job1"})
	f.mustCreate(&model.ProjectResultDO{ProjectID: "p2", Kind: "Model", NodeID: "alice", RefID: "out-bbb", JobID: "job2"})
	f.mustCreate(&model.ProjectResultDO{ProjectID: "p2", Kind: "Model", NodeID: "bob", RefID: "out-ccc", JobID: "job2"})
	// TEE pull records for the same result from two TEE nodes.
	f.mustCreate(&model.TeeNodeDatatableManagementDO{NodeID: "alice", TeeNodeID: "tee", DatatableID: "out-aaa",
		Kind: TeeJobKindPull, JobID: "j-t1", Status: "SUCCESS"})
	f.mustCreate(&model.TeeNodeDatatableManagementDO{NodeID: "alice", TeeNodeID: "tee2", DatatableID: "out-aaa",
		Kind: TeeJobKindPull, JobID: "j-t2", Status: "FAILED", ErrMsg: "boom"})
}

func resultIDs(vo *AllNodeResultsListCompatVO) []string {
	out := []string{}
	for _, r := range vo.NodeAllResultsVOList {
		out = append(out, r.NodeResultsVO.DomainDataID)
	}
	return out
}

func TestNodeResults_ListFiltersLikeJava(t *testing.T) {
	f := newGraphFixture(t)
	seedNodeResults(f)
	ns := NewNodeService(repository.NewNodeRepo(f.db), repository.NewNodeRouteRepo(f.db), nil, f.db)
	ns.Configure(NodeServiceOptions{TeeNodeID: "tee"})
	ctx := context.Background()

	cases := []struct {
		name string
		req  ListNodeResultRequest
		want []string
	}{
		{"owner only", ListNodeResultRequest{OwnerID: "alice"}, []string{"out-aaa", "out-bbb"}},
		{"name matches result id", ListNodeResultRequest{OwnerID: "alice", NameFilter: "bbb"}, []string{"out-bbb"}},
		{"name matches project name", ListNodeResultRequest{OwnerID: "alice", NameFilter: "risk"}, []string{"out-aaa"}},
		{"name matches train flow", ListNodeResultRequest{OwnerID: "alice", NameFilter: "flow-A"}, []string{"out-aaa"}},
		{"kind filter", ListNodeResultRequest{OwnerID: "alice", KindFilters: []string{"Model"}}, []string{"out-bbb"}},
		// Java NodeManager.listResult accepts but does not apply dataVendorFilter.
		{"vendor filter ignored", ListNodeResultRequest{OwnerID: "alice", DataVendorFilter: "manual"}, []string{"out-aaa", "out-bbb"}},
		// Outside AUTONOMY nodeNamesFilter is ignored (only ownerId).
		{"node filter ignored outside autonomy", ListNodeResultRequest{OwnerID: "alice", NodeNamesFilter: []string{"bob"}}, []string{"out-aaa", "out-bbb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vo, err := ns.ListNodeResults(ctx, &tc.req)
			if err != nil {
				t.Fatal(err)
			}
			got := resultIDs(vo)
			if len(got) != len(tc.want) || vo.TotalNodeResultNums != len(tc.want) {
				t.Fatalf("got %v (total %d), want %v", got, vo.TotalNodeResultNums, tc.want)
			}
			seen := map[string]bool{}
			for _, g := range got {
				seen[g] = true
			}
			for _, w := range tc.want {
				if !seen[w] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}

	// teeNodeId selects the pull record of that TEE node (default: configured).
	pull := func(tee string) string {
		vo, _ := ns.ListNodeResults(ctx, &ListNodeResultRequest{OwnerID: "alice", NameFilter: "aaa", TeeNodeID: tee})
		return vo.NodeAllResultsVOList[0].NodeResultsVO.PullFromTeeStatus
	}
	if got := pull(""); got != "SUCCESS" {
		t.Fatalf("default tee pull status = %q", got)
	}
	if got := pull("tee2"); got != "FAILED" {
		t.Fatalf("tee2 pull status = %q", got)
	}
}

func TestNodeResults_AutonomyListsReadyInstNodes(t *testing.T) {
	f := newGraphFixture(t)
	seedNodeResults(f)
	f.mustCreate(&model.InstDO{InstID: "inst1", Name: "inst-one"})
	f.mustCreate(&model.InstDO{InstID: "inst2", Name: "inst-two"})
	f.mustCreate(&model.NodeDO{NodeID: "alice", Name: "alice", InstID: "inst1"})
	f.mustCreate(&model.NodeDO{NodeID: "bob", Name: "bob", InstID: "inst2"})
	ns := NewNodeService(repository.NewNodeRepo(f.db), repository.NewNodeRouteRepo(f.db), nil, f.db)
	ns.Configure(NodeServiceOptions{Env: NewEnvService("AUTONOMY", "alice", "inst1", nil)})
	vo, err := ns.ListNodeResults(context.Background(), &ListNodeResultRequest{OwnerID: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range vo.NodeAllResultsVOList {
		if r.NodeID != "alice" {
			t.Fatalf("AUTONOMY must list only the institution's nodes, got %s", r.NodeID)
		}
	}
	if vo.TotalNodeResultNums != 2 {
		t.Fatalf("total = %d", vo.TotalNodeResultNums)
	}
	// nodeNamesFilter (node ids) narrows the institution's Ready nodes.
	vo, _ = ns.ListNodeResults(context.Background(), &ListNodeResultRequest{NodeNamesFilter: []string{"bob"}})
	if vo.TotalNodeResultNums != 0 {
		t.Fatalf("foreign-inst node filter must yield nothing, got %d", vo.TotalNodeResultNums)
	}
}

// domainDataServer answers /api/v1/domaindata/query with the given data, or
// the Kuscia "domaindata not exists" error when data is nil.
func domainDataServer(t *testing.T, data map[string]any) *kuscia.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/domaindata/query" && data == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"code": kuscia.ErrCodeDomainDataNotExists, "message": "not exists"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"code": 0}, "data": data})
	}))
	t.Cleanup(srv.Close)
	return kuscia.NewClient(&kuscia.ClientConfig{Host: "127.0.0.1", Port: srvPort(t, srv), Protocol: "notls"})
}

func srvPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	f := &fakeKusciaAPI{srv: srv}
	return f.endpoint(t, "x").Port
}

func TestNodeResults_DetailDatasourcePathAndMissingDatatable(t *testing.T) {
	f := newGraphFixture(t)
	seedNodeResults(f)
	ctx := context.Background()

	kc := domainDataServer(t, map[string]any{"domaindata_id": "out-aaa", "domain_id": "alice",
		"datasource_id": "default-data-source", "type": "table",
		"columns": []map[string]any{{"name": "id", "type": "str"}}})
	ns := NewNodeService(repository.NewNodeRepo(f.db), repository.NewNodeRouteRepo(f.db), kc, f.db)
	vo, err := ns.GetNodeResultDetail(ctx, &GetNodeResultDetailRequest{NodeID: "alice", DomainDataID: "out-aaa"})
	if err != nil {
		t.Fatal(err)
	}
	if vo.Datasource != "/home/kuscia/var/storage/data" {
		t.Fatalf("datasource = %q, want the default datasource path", vo.Datasource)
	}
	if len(vo.TableColumnVOList) != 1 {
		t.Fatalf("columns = %v", vo.TableColumnVOList)
	}

	missing := NewNodeService(repository.NewNodeRepo(f.db), repository.NewNodeRouteRepo(f.db), domainDataServer(t, nil), f.db)
	_, err = missing.GetNodeResultDetail(ctx, &GetNodeResultDetailRequest{NodeID: "alice", DomainDataID: "out-aaa"})
	pe, ok := AsPadError(err)
	if !ok || pe.Code != errcode.DatatableNotExists {
		t.Fatalf("err = %v, want DATATABLE_NOT_EXISTS", err)
	}
}
