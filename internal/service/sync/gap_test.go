package datasync

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
)

// Every Java synced entity (JpaSyncDataService + all data.sync lists) is
// registered; project_model_export (Go-only replacement of the node-local
// Java MODEL_EXPORT_CACHE) is registered for opt-in but not synced by default.
func TestRegistry_JavaSyncedEntitiesAndModelExport(t *testing.T) {
	r := DefaultRegistry()
	java := []string{"InstDO", "NodeDO", "NodeRouteDO", "ProjectDatatableDO", "ProjectFedTableDO", "ProjectGraphNodeDO",
		"ProjectGraphDO", "ProjectInstDO", "ProjectJobDO", "ProjectJobTaskLogDO", "ProjectTaskDO", "ProjectModelDO",
		"ProjectNodeDO", "ProjectReportDO", "ProjectDO", "ProjectResultDO", "ProjectRuleDO", "TeeDownLoadAuditConfigDO",
		"TeeNodeDatatableManagementDO", "VoteInviteDO", "VoteRequestDO", "NodeRouteApprovalConfigDO",
		"ProjectApprovalConfigDO", "ProjectGraphNodeKusciaParamsDO", "ProjectModelServingDO", "ProjectModelPackDO",
		"ProjectFeatureTableDO", "FeatureTableDO", "ProjectGraphDomainDatasourceDO", "ProjectScheduleDO",
		"ProjectScheduleJobDO", "ProjectScheduleTaskDO"}
	for _, j := range java {
		if _, ok := r.Canonical("org.secretflow.secretpad.persistence.entity." + j); !ok {
			t.Errorf("Java synced entity %s not registered", j)
		}
	}
	if got, ok := r.Canonical("project_model_export"); !ok || got != TableProjectModelExport {
		t.Fatalf("project_model_export not registered")
	}
	if k := r.KeyColumns(TableProjectModelExport); len(k) != 1 || k[0] != "job_id" {
		t.Errorf("project_model_export key = %v", k)
	}
	for _, tb := range append(append([]string{}, DefaultCenterTables...), DefaultP2PTables...) {
		if tb == TableProjectModelExport {
			t.Error("project_model_export must not be synced by default (Java does not sync it)")
		}
	}
	if len(DefaultCenterTables) != 21 || len(DefaultP2PTables) != 18 {
		t.Errorf("default lists drifted from Java: center=%d p2p=%d", len(DefaultCenterTables), len(DefaultP2PTables))
	}
}

type staticIdentity struct {
	inst  string
	nodes []string
}

func (s staticIdentity) InstID() string    { return s.inst }
func (s staticIdentity) NodeIDs() []string { return s.nodes }

// DataSyncConsumerTemplate + NodeManager.getTargetNodeIds: in AUTONOMY a
// created peer graph-domain-datasource also gets default bindings for the
// nodes paired with the source domain in the participant tree.
func TestConsumer_GraphDomainDatasourceTargetNodes(t *testing.T) {
	db := newTestDB(t)
	tree := `[{"initiatorNodeId":"bob","invitees":[{"inviteeId":"alice"},{"inviteeId":"carol"}]},` +
		`{"initiatorNodeId":"dave","invitees":[{"inviteeId":"bob"}]},` +
		`{"initiatorNodeId":"erin","invitees":[{"inviteeId":"frank"}]}]`
	if err := db.Create(&model.ProjectApprovalConfigDO{ProjectID: "p", VoteID: "v1", Type: "PROJECT_CREATE",
		ParticipantNodeInfo: tree}).Error; err != nil {
		t.Fatal(err)
	}
	c := NewConsumer(db, nil, tables("project_graph_domain_datasource"), "alice", nil).
		SetLocalIdentity(staticIdentity{inst: "instA", nodes: []string{"alice", "alice2"}}, true)

	if got := c.targetNodeIDs(context.Background(), "bob", "p"); !equalSorted(got, []string{"alice", "carol", "dave"}) {
		t.Fatalf("targetNodeIDs(bob) = %v", got)
	}
	if got := c.targetNodeIDs(context.Background(), "alice2", "p"); len(got) != 0 {
		t.Fatalf("local inst node must have no targets: %v", got)
	}

	if _, err := c.Apply(context.Background(), ModeP2P, &service.SyncData{TableName: "project_graph_domain_datasource",
		Action: "create", Data: json.RawMessage(`{"project_id":"p","graph_id":"g","domain_id":"bob","datasource_id":"ds"}`)}); err != nil {
		t.Fatal(err)
	}
	var rows []model.ProjectGraphDomainDatasourceDO
	db.Where("project_id = ? AND graph_id = ?", "p", "g").Find(&rows)
	got := map[string]string{}
	for _, r := range rows {
		got[r.DomainID] = r.DatasourceID
	}
	want := map[string]string{"bob": "ds", "alice": defaultDataSource, "carol": defaultDataSource, "dave": defaultDataSource}
	if len(got) != len(want) {
		t.Fatalf("bindings = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("binding %s = %q want %q", k, got[k], v)
		}
	}

	// non-AUTONOMY: only the local binding is added.
	c2 := NewConsumer(db, nil, tables("project_graph_domain_datasource"), "alice", nil)
	if _, err := c2.Apply(context.Background(), ModeP2P, &service.SyncData{TableName: "project_graph_domain_datasource",
		Action: "create", Data: json.RawMessage(`{"project_id":"p","graph_id":"g2","domain_id":"bob","datasource_id":"ds"}`)}); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.ProjectGraphDomainDatasourceDO{}).Where("graph_id = ?", "g2").Count(&n)
	if n != 2 {
		t.Fatalf("expected bob + local alice binding, got %d", n)
	}
}

func equalSorted(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Writes that bypass a model (db.Table("x") + map Updates / Create) used to
// have no Statement.Schema and were silently not captured.
func TestProducer_CapturesModelLessTableWrites(t *testing.T) {
	db := newTestDB(t)
	log := &eventLog{}
	p := NewProducer(nil, tables("project_job", "project_node"), log.sink, nil)
	if err := p.Register(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ProjectJobDO{ProjectID: "p", JobID: "j1", Name: "j", Status: "RUNNING"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Table("project_job").Where("job_id = ?", "j1").
			Updates(map[string]interface{}{"status": "STOPPED"}).Error
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Table("project_node").Create(map[string]interface{}{
		"project_id": "p", "node_id": "alice", "gmt_create": now, "gmt_modified": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// aliased tables are not tracked (and must not panic)
	db.Table("project_job AS j").Where("j.job_id = ?", "none").Updates(map[string]interface{}{"status": "X"})

	evs := log.list()
	if len(evs) != 3 {
		for _, e := range evs {
			t.Logf("%s %s %s", e.Table, e.Action, e.Data)
		}
		t.Fatalf("expected 3 events, got %d", len(evs))
	}
	up := evs[1]
	if up.Table != "project_job" || up.Action != service.SyncActionUpdate {
		t.Fatalf("event 1 = %s %s", up.Table, up.Action)
	}
	job, ok := up.Entity.(*model.ProjectJobDO)
	if !ok || job.Status != "STOPPED" || job.JobID != "j1" || job.ProjectID != "p" {
		t.Fatalf("update snapshot = %+v", up.Entity)
	}
	cr := evs[2]
	pn, ok := cr.Entity.(*model.ProjectNodeDO)
	if cr.Table != "project_node" || cr.Action != service.SyncActionCreate || !ok || pn.NodeID != "alice" {
		t.Fatalf("create event = %s %s %+v", cr.Table, cr.Action, cr.Entity)
	}
	if cr.Route.ProjectID != "p" {
		t.Errorf("route = %+v", cr.Route)
	}
}
