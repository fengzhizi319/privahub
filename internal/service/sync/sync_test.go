package datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", name, time.Now().UnixNano())),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1) // detects callbacks that escape the transaction connection
	t.Cleanup(func() { sqlDB.Close() })
	if err := migration.AutoMigrate(db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	return db
}

func tables(names ...string) TableSet {
	s, _ := DefaultRegistry().ResolveTables(names)
	return s
}

// --- registry / codec -------------------------------------------------------

func TestRegistry_CanonicalAndKeys(t *testing.T) {
	r := DefaultRegistry()
	cases := map[string]string{
		"project":   "project",
		"ProjectDO": "project",
		"org.secretflow.secretpad.persistence.entity.ProjectTaskDO":            "project_job_task",
		"org.secretflow.secretpad.persistence.entity.TeeDownLoadAuditConfigDO": "tee_download_approval_config",
		"org.secretflow.secretpad.persistence.entity.VoteInviteDO":             "vote_invite",
	}
	for in, want := range cases {
		got, ok := r.Canonical(in)
		if !ok || got != want {
			t.Errorf("Canonical(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := r.Canonical("org.secretflow.secretpad.persistence.entity.AccountsDO"); ok {
		t.Error("user accounts must never be synchronizable")
	}
	if got := strings.Join(r.KeyColumns("vote_invite"), ","); got != "vote_id,vote_participant_id" {
		t.Errorf("vote_invite key = %s", got)
	}
	if got := strings.Join(r.KeyColumns("node_route"), ","); got != "src_node_id,dst_node_id" {
		t.Errorf("node_route key = %s", got)
	}
	for _, tb := range append(append([]string{}, DefaultCenterTables...), DefaultP2PTables...) {
		if _, ok := r.info(tb); !ok {
			t.Errorf("default table %s not registered", tb)
		}
	}
}

func TestDecode_JavaEntityJSON(t *testing.T) {
	r := DefaultRegistry()
	raw := `{"upk":{"voteID":"v1","votePartitionID":"bob"},"initiator":"alice","action":"APPROVED",
		"desc":"d","type":"PROJECT_CREATE","gmtCreate":"2024-05-01T10:00:00","extra":1}`
	e, err := r.Decode("vote_invite", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	v := e.(*model.VoteInviteDO)
	if v.VoteID != "v1" || v.VoteParticipantID != "bob" || v.Initiator != "alice" || v.Description != "d" {
		t.Fatalf("decoded %+v", v)
	}
	if v.GmtCreate.IsZero() {
		t.Fatal("gmtCreate not parsed")
	}
	// Java list -> Go string field.
	e, err = r.Decode("vote_request", []byte(`{"voteID":"v2","voters":["alice","bob"],"status":"1"}`))
	if err != nil {
		t.Fatal(err)
	}
	vr := e.(*model.VoteRequestDO)
	if vr.Voters != `["alice","bob"]` || vr.Status != 1 {
		t.Fatalf("decoded %+v", vr)
	}
	route := RouteOf("vote_request", vr)
	if strings.Join(route.NodeIDs, ",") != "alice,bob" {
		t.Fatalf("route %+v", route)
	}
}

// --- consumer ---------------------------------------------------------------

func TestConsumer_UpsertDelete(t *testing.T) {
	db := newTestDB(t)
	c := NewConsumer(db, nil, tables("project", "project_node", "vote_request"), "alice", nil)
	ctx := context.Background()
	modified := time.Date(2024, 1, 2, 3, 4, 5, 0, time.Local)
	p := model.ProjectDO{ProjectID: "p1", Name: "one", OwnerID: "kuscia-system"}
	p.ID = 999 // remote id must not be used
	p.GmtModified = modified
	b, _ := json.Marshal(p)
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "org.secretflow.secretpad.persistence.entity.ProjectDO", Action: "create", Data: json.RawMessage(b)}); err != nil {
		t.Fatal(err)
	}
	var got model.ProjectDO
	db.Where("project_id = ?", "p1").First(&got)
	if got.Name != "one" || got.ID == 999 || !got.GmtModified.Equal(modified) {
		t.Fatalf("upsert create: %+v", got)
	}
	// update by business key
	p.Name = "two"
	b, _ = json.Marshal(p)
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "project", Action: "UPDATE", Data: json.RawMessage(b)}); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.ProjectDO{}).Count(&n)
	db.Where("project_id = ?", "p1").First(&got)
	if n != 1 || got.Name != "two" {
		t.Fatalf("upsert update: n=%d %+v", n, got)
	}
	// delete
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "project", Action: "remove", Data: json.RawMessage(b)}); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.ProjectDO{}).Count(&n)
	if n != 0 {
		t.Fatalf("delete: %d rows left", n)
	}
	// re-create restores the soft-deleted row
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "project", Action: "create", Data: json.RawMessage(b)}); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.ProjectDO{}).Count(&n)
	if n != 1 {
		t.Fatalf("restore: %d rows", n)
	}
	// not allowed table
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "node", Action: "create", Data: json.RawMessage(`{"node_id":"x"}`)}); err == nil {
		t.Fatal("expected table not allowed")
	}
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "user_accounts", Action: "create", Data: json.RawMessage(`{"name":"evil"}`)}); err == nil {
		t.Fatal("user_accounts must be rejected")
	}
	// edge ignores executed vote requests
	vr := `{"vote_id":"v1","initiator":"a","type":"t","voters":"[]","vote_counter":"a","executors":"[]","execute_status":"SUCCESS","description":"d"}`
	if _, err := c.Apply(ctx, ModeEdge, &service.SyncData{TableName: "vote_request", Action: "create", Data: json.RawMessage(vr)}); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.VoteRequestDO{}).Count(&n)
	if n != 0 {
		t.Fatal("executed vote request must be ignored on edge")
	}
}

func TestConsumer_P2PFinishedJobIgnored(t *testing.T) {
	db := newTestDB(t)
	db.Create(&model.ProjectJobDO{ProjectID: "p", JobID: "j1", Name: "j", Status: "SUCCEED"})
	c := NewConsumer(db, nil, tables("project_job", "project_graph_domain_datasource"), "alice", nil)
	if _, err := c.Apply(context.Background(), ModeP2P, &service.SyncData{TableName: "project_job", Action: "update",
		Data: json.RawMessage(`{"project_id":"p","job_id":"j1","name":"j","status":"RUNNING"}`)}); err != nil {
		t.Fatal(err)
	}
	var job model.ProjectJobDO
	db.Where("job_id = ?", "j1").First(&job)
	if job.Status != "SUCCEED" {
		t.Fatalf("finished job overwritten: %s", job.Status)
	}
	// graph domain datasource creation adds the local default binding
	if _, err := c.Apply(context.Background(), ModeP2P, &service.SyncData{TableName: "project_graph_domain_datasource", Action: "create",
		Data: json.RawMessage(`{"project_id":"p","graph_id":"g","domain_id":"bob","datasource_id":"ds"}`)}); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.ProjectGraphDomainDatasourceDO{}).Where("domain_id IN ?", []string{"alice", "bob"}).Count(&n)
	if n != 2 {
		t.Fatalf("expected bob + local alice binding, got %d", n)
	}
}

// --- producer ---------------------------------------------------------------

type eventLog struct {
	mu  sync.Mutex
	evs []*ChangeEvent
}

func (l *eventLog) sink(ev *ChangeEvent) { l.mu.Lock(); l.evs = append(l.evs, ev); l.mu.Unlock() }
func (l *eventLog) list() []*ChangeEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*ChangeEvent(nil), l.evs...)
}

func TestProducer_CapturesChanges(t *testing.T) {
	db := newTestDB(t)
	log := &eventLog{}
	p := NewProducer(nil, tables("project", "vote_invite"), log.sink, nil)
	if err := p.Register(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pr := &model.ProjectDO{ProjectID: "p1", Name: "n"}
	if err := db.Create(pr).Error; err != nil {
		t.Fatal(err)
	}
	// update by primary key (Save / Model(&x).Update)
	db.Model(pr).Update("name", "n2")
	// update by where inside a transaction (single connection pool)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("description", "d").Error
	}); err != nil {
		t.Fatal(err)
	}
	// untracked table
	db.Create(&model.NodeDO{NodeID: "x", Name: "x", ControlNodeID: "x"})
	// consumer writes are not captured
	db.WithContext(service.WithSyncConsumer(ctx)).Create(&model.ProjectDO{ProjectID: "p2", Name: "n"})
	// delete by where
	db.Where("project_id = ?", "p1").Delete(&model.ProjectDO{})

	evs := log.list()
	var got []string
	for _, e := range evs {
		got = append(got, e.Table+":"+e.Action)
	}
	want := "project:create,project:update,project:update,project:remove"
	if strings.Join(got, ",") != want {
		t.Fatalf("events = %v, want %s", got, want)
	}
	upd := evs[2].Entity.(*model.ProjectDO)
	if upd.Name != "n2" || upd.Description != "d" {
		t.Fatalf("update snapshot must be the full row: %+v", upd)
	}
	if evs[3].Route.ProjectID != "p1" || !strings.Contains(evs[3].Key, `project_id="p1"`) {
		t.Fatalf("route/key: %+v %s", evs[3].Route, evs[3].Key)
	}
}

// --- center dispatcher + SSE + edge client ----------------------------------

type fakeSse struct {
	mu   sync.Mutex
	subs map[string][]string
	sent map[string][]*service.SyncData
	fail map[string]int
}

func newFakeSse() *fakeSse {
	return &fakeSse{subs: map[string][]string{}, sent: map[string][]*service.SyncData{}, fail: map[string]int{}}
}
func (f *fakeSse) SendE(n string, d *service.SyncData) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail[n] > 0 {
		f.fail[n]--
		return fmt.Errorf("boom")
	}
	f.sent[n] = append(f.sent[n], d)
	return nil
}
func (f *fakeSse) IsSubscribed(n, t string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs[n] {
		if s == t {
			return true
		}
	}
	return false
}
func (f *fakeSse) NodeIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for n := range f.subs {
		out = append(out, n)
	}
	return out
}
func (f *fakeSse) count(n string) int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent[n]) }

func TestCenterDispatcher_FilterRouteRetry(t *testing.T) {
	db := newTestDB(t)
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	sse := newFakeSse()
	sse.subs["alice"] = []string{"project", "project_job", "node"}
	sse.subs["bob"] = []string{"project", "node"}
	d := NewCenterDispatcher(db, nil, sse, tables(DefaultCenterTables...), time.Hour, 3, nil)
	p := NewProducer(nil, tables(DefaultCenterTables...), d.Push, nil)
	if err := p.Register(db); err != nil {
		t.Fatal(err)
	}
	db.Create(&model.ProjectDO{ProjectID: "p1", Name: "x"})                              // -> alice only (project nodes)
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j", Name: "j"})               // job create is filtered
	db.Model(&model.ProjectJobDO{}).Where("job_id = ?", "j").Update("status", "RUNNING") // job update -> alice
	db.Create(&model.NodeDO{NodeID: "n9", Name: "n9", ControlNodeID: "n9"})              // broadcast
	sse.fail["alice"] = 1
	d.Flush()
	if d.Pending() != 1 {
		t.Fatalf("one failed delivery must be re-queued, pending=%d", d.Pending())
	}
	d.Flush()
	if sse.count("alice") != 3 || sse.count("bob") != 1 {
		t.Fatalf("alice=%d bob=%d", sse.count("alice"), sse.count("bob"))
	}
}

func TestCenterDispatcher_InitialSyncVisibility(t *testing.T) {
	db := newTestDB(t)
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.ProjectDO{ProjectID: "p1", Name: "a"})
	db.Create(&model.ProjectDO{ProjectID: "p2", Name: "b"})
	db.Create(&model.NodeRouteDO{RouteID: "r", SrcNodeID: "alice", DstNodeID: "bob"})
	db.Create(&model.NodeRouteDO{RouteID: "r2", SrcNodeID: "carol", DstNodeID: "bob"})
	sse := newFakeSse()
	d := NewCenterDispatcher(db, nil, sse, tables(DefaultCenterTables...), time.Hour, 3, nil)
	d.InitialSync("alice", []service.SyncData{{TableName: "project", LastUpdateTime: "0"},
		{TableName: "node_route", LastUpdateTime: "0"}, {TableName: "user_accounts", LastUpdateTime: "0"}})
	if sse.count("alice") != 2 {
		t.Fatalf("alice should see p1 + one route, got %d", sse.count("alice"))
	}
	// incremental: nothing newer than the future
	future := time.Now().Add(time.Hour).Format(checkpointLayout)
	d.InitialSync("bob", []service.SyncData{{TableName: "node_route", LastUpdateTime: future}})
	if sse.count("bob") != 0 {
		t.Fatalf("bob incremental got %d", sse.count("bob"))
	}
}

// TestEdgeClient_EndToEnd runs a real SseServer + CenterDispatcher behind an
// httptest server and an EdgeClient against it.
func TestEdgeClient_EndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	center := newTestDB(t)
	edge := newTestDB(t)
	center.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "edge1"})
	center.Create(&model.ProjectDO{ProjectID: "p1", Name: "before"})

	sseSrv := service.NewSseServerWithInterval(zap.NewNop(), 50*time.Millisecond)
	defer sseSrv.Stop()
	all := tables(DefaultCenterTables...)
	sseSrv.SetTableFilter(func(n string) (string, bool) { t, ok := DefaultRegistry().Canonical(n); return t, ok && all.Has(t) })
	d := NewCenterDispatcher(center, nil, sseSrv, all, 20*time.Millisecond, 3, nil)
	sseSrv.SetOpenHook(d.InitialSync)
	prod := NewProducer(nil, all, d.Push, nil)
	if err := prod.Register(center); err != nil {
		t.Fatal(err)
	}
	d.Start()
	defer d.Stop()

	var gotHost, gotOrigin string
	var hmu sync.Mutex
	r := gin.New()
	r.GET("/sync", func(c *gin.Context) {
		hmu.Lock()
		gotHost, gotOrigin = c.Request.Host, c.GetHeader("kuscia-origin-source")
		hmu.Unlock()
		sseSrv.HandleSseSync(c)
	})
	srv := httptest.NewUnstartedServer(r)
	srv.Config.WriteTimeout = 100 * time.Millisecond // SSE must survive server timeouts
	srv.Config.ReadTimeout = 100 * time.Millisecond
	srv.Start()
	defer srv.Close()

	consumer := NewConsumer(edge, nil, all, "edge1", nil)
	ec := NewEdgeClient(edge, nil, consumer, EdgeClientOptions{
		Gateway: srv.URL, CenterHost: "secretpad.master.svc", NodeID: "edge1",
		Tables: tables("project", "node"), Reconnect: 50 * time.Millisecond, IdleTimeout: time.Second,
	}, nil)
	ec.Start()
	defer ec.Stop()

	waitFor(t, "initial sync", func() bool {
		var p model.ProjectDO
		return edge.Where("project_id = ?", "p1").Limit(1).Find(&p).RowsAffected == 1 && p.Name == "before"
	})
	time.Sleep(300 * time.Millisecond) // beyond the server write/read timeouts
	center.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("name", "after")
	waitFor(t, "live update", func() bool {
		var p model.ProjectDO
		edge.Where("project_id = ?", "p1").Limit(1).Find(&p)
		return p.Name == "after"
	})
	center.Where("project_id = ?", "p1").Delete(&model.ProjectDO{})
	waitFor(t, "live delete", func() bool {
		var n int64
		edge.Model(&model.ProjectDO{}).Count(&n)
		return n == 0
	})
	if !ec.Connected() || sseSrv.ActiveConnections() != 1 {
		t.Fatalf("stream should still be connected")
	}
	hmu.Lock()
	defer hmu.Unlock()
	if gotHost != "secretpad.master.svc" || gotOrigin != "edge1" {
		t.Fatalf("headers host=%q origin=%q", gotHost, gotOrigin)
	}
	var logs []model.EdgeDataSyncLogDO
	edge.Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("edge_data_sync_log rows = %d", len(logs))
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestEdgeClient_ParseStream(t *testing.T) {
	db := newTestDB(t)
	c := NewConsumer(db, nil, tables("project"), "", nil)
	ec := NewEdgeClient(db, nil, c, EdgeClientOptions{Tables: tables("project")}, nil)
	stream := "id:ping\n: ping\ndata:ping\n\n" +
		"id:project\ndata:{\"tableName\":\"project\",\"action\":\"create\",\"data\":{\"project_id\":\"p9\",\"name\":\"x\"}}\n\n"
	_ = ec.consume(context.Background(), strings.NewReader(stream), make(chan struct{}, 1))
	if ec.Received() != 1 {
		t.Fatalf("received %d", ec.Received())
	}
}

// --- P2P --------------------------------------------------------------------

type fakeIdentity struct{ inst string }

func (f fakeIdentity) InstID() string    { return f.inst }
func (f fakeIdentity) NodeIDs() []string { return []string{"alice"} }

type fakePoster struct {
	mu    sync.Mutex
	calls []string
	fail  int
}

func (f *fakePoster) PostDataSync(_ context.Context, dst string, d *service.SyncData) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail > 0 {
		f.fail--
		return fmt.Errorf("down")
	}
	f.calls = append(f.calls, dst+":"+d.TableName+":"+d.Action)
	return nil
}
func (f *fakePoster) list() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func TestP2P_FilterPadQueue(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	poster := &fakePoster{fail: 1}
	p := NewP2PSync(db, nil, tables(DefaultP2PTables...), fakeIdentity{"instA"}, poster, dir, 3, time.Hour, nil)
	prod := NewProducer(nil, tables(DefaultP2PTables...), p.Push, nil)
	if err := prod.Register(db); err != nil {
		t.Fatal(err)
	}
	db.Create(&model.ProjectInstDO{ProjectID: "p1", InstID: "instA"}) // not owner yet -> filtered
	db.Create(&model.ProjectInstDO{ProjectID: "p1", InstID: "instB"})
	db.Create(&model.ProjectDO{ProjectID: "p1", Name: "x", OwnerID: "instA"})                                                      // owner -> instB
	db.Create(&model.ProjectDO{ProjectID: "p2", Name: "y", OwnerID: "instZ"})                                                      // foreign -> filtered
	db.Create(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "bob", DatatableID: "t", TableConfigs: "{}", Source: "IMPORTED"}) // not local node
	db.Create(&model.ProjectDatatableDO{ProjectID: "p1", NodeID: "alice", DatatableID: "t", TableConfigs: "{}", Source: "IMPORTED"})
	db.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("name", "x2")
	db.Model(&model.ProjectDO{}).Where("project_id = ?", "p1").Update("name", "x3") // supersedes x2

	if n := p.Pending("instB"); n != 3 {
		t.Fatalf("pending instB = %d (queue must dedupe updates)", n)
	}
	if p.Pending("instA") != 0 {
		t.Fatal("local inst must never be a destination")
	}
	if _, err := os.Stat(filepath.Join(dir, "instB")); err != nil {
		t.Fatalf("queue not persisted: %v", err)
	}
	p.Drain("instB") // first send fails -> moved to tail, round stops
	p.Drain("instB")
	calls := poster.list()
	if len(calls) != 3 || p.Pending("instB") != 0 {
		t.Fatalf("calls=%v pending=%d", calls, p.Pending("instB"))
	}
	if _, err := os.Stat(filepath.Join(dir, "instB")); !os.IsNotExist(err) {
		t.Fatal("empty queue file must be removed")
	}

	// restored from disk
	p.enqueue(&ChangeEvent{Table: "project", Action: "update", Data: json.RawMessage(`{}`)}, []string{"instC"})
	p2 := NewP2PSync(db, nil, tables(DefaultP2PTables...), fakeIdentity{"instA"}, poster, dir, 3, time.Hour, nil)
	p2.Load()
	if p2.Pending("instC") != 1 {
		t.Fatalf("restored pending = %d", p2.Pending("instC"))
	}
}

func TestP2P_VoteInviteReplyGoesToInitiator(t *testing.T) {
	db := newTestDB(t)
	p := NewP2PSync(db, nil, tables(DefaultP2PTables...), fakeIdentity{"instB"}, &fakePoster{}, "", 3, time.Hour, nil)
	inv := &model.VoteInviteDO{VoteID: "v", Initiator: "instA", VoteParticipantID: "instB"}
	ev := &ChangeEvent{Table: "vote_invite", Action: "update", Entity: inv, Route: RouteOf("vote_invite", inv)}
	if p.Filter(ev) || strings.Join(ev.Route.NodeIDs, ",") != "instA" {
		t.Fatalf("reply must go to the initiator: %+v", ev.Route)
	}
	ev2 := &ChangeEvent{Table: "vote_invite", Action: "create", Entity: inv, Route: RouteOf("vote_invite", inv)}
	if !p.Filter(ev2) {
		t.Fatal("non-initiator create must not be synced")
	}
}

// --- sender / vote sync -----------------------------------------------------

func TestGatewaySender_VoteSyncToCenterAndPeers(t *testing.T) {
	db := newTestDB(t)
	db.Create(&model.NodeDO{NodeID: "bobnode", Name: "b", ControlNodeID: "b", InstID: "instB", MasterNodeID: "bobnode"})
	var mu sync.Mutex
	var reqs []*http.Request
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, r)
		bodies = append(bodies, string(b))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":{"code":0,"msg":"success"}}`))
	}))
	defer srv.Close()
	s := NewGatewaySender(db, nil, GatewaySenderOptions{Gateway: strings.TrimPrefix(srv.URL, "http://"),
		CenterHost: "secretpad.master.svc", LocalNodeID: "edge1", PlatformType: service.PlatformEdge})
	vr := &model.VoteRequestDO{VoteID: "v1", Initiator: "edge1"}
	if err := s.Send(context.Background(), "", service.SyncData{TableName: "vote_request", Data: vr}); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(context.Background(), "", service.SyncData{TableName: "project_job", Data: vr}); err == nil {
		t.Fatal("non vote-sync table must be rejected")
	}
	s.opts.PlatformType = service.PlatformAutonomy
	if err := s.Send(context.Background(), "instB", service.SyncData{TableName: "vote_invite", Action: "update", Data: &model.VoteInviteDO{VoteID: "v1"}}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d", len(reqs))
	}
	if reqs[0].URL.Path != VoteSyncPath || reqs[0].Host != "secretpad.master.svc" || reqs[0].Header.Get("kuscia-origin-source") != "edge1" {
		t.Fatalf("center call: %s host=%s", reqs[0].URL.Path, reqs[0].Host)
	}
	if !strings.Contains(bodies[0], `"syncDataType":"VOTE_REQUEST"`) || !strings.Contains(bodies[0], `"vote_id":"v1"`) {
		t.Fatalf("body %s", bodies[0])
	}
	if reqs[1].URL.Path != DataSyncPath || reqs[1].Host != "secretpad.bobnode.svc" {
		t.Fatalf("peer call: %s host=%s", reqs[1].URL.Path, reqs[1].Host)
	}

	// remote error surfaces
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"code":202011100,"msg":"nope"}}`))
	}))
	defer bad.Close()
	s2 := NewGatewaySender(db, nil, GatewaySenderOptions{Gateway: bad.URL, PlatformType: service.PlatformEdge})
	if err := s2.Send(context.Background(), "", service.SyncData{TableName: "vote_request", Data: vr}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected remote error, got %v", err)
	}
}

func TestVoteSyncService_SevenTypes(t *testing.T) {
	db := newTestDB(t)
	s := NewVoteSyncService(db, nil, nil)
	body := `{"dbSyncRequests":[
	 {"syncDataType":"VOTE_REQUEST","projectNodesInfo":{"voteID":"v1","initiator":"alice","type":"PROJECT_CREATE","voters":["bob"],"voteCounter":"alice","executors":["alice"],"approvedThreshold":1,"status":0,"desc":"d"}},
	 {"syncDataType":"VOTE_INVITE","projectNodesInfo":{"upk":{"voteID":"v1","votePartitionID":"bob"},"initiator":"alice","type":"PROJECT_CREATE","action":"REVIEWING","desc":"d"}},
	 {"syncDataType":"NODE_ROUTE","projectNodesInfo":{"routeId":"r1","srcNodeId":"alice","dstNodeId":"bob","srcNetAddress":"a:1","dstNetAddress":"b:1"}},
	 {"syncDataType":"TEE_NODE_DATATABLE_MANAGEMENT","projectNodesInfo":{"nodeId":"alice","teeNodeId":"tee","datatableId":"t","jobId":"j","datasourceId":"ds","kind":"Push","status":"RUNNING"}},
	 {"syncDataType":"PROJECT","projectNodesInfo":{"projectId":"p1","name":"n","ownerId":"alice"}},
	 {"syncDataType":"PROJECT_NODE","projectNodesInfo":{"upk":{"projectId":"p1","nodeId":"bob"}}},
	 {"syncDataType":"PROJECT_APPROVAL_CONFIG","projectNodesInfo":{"voteID":"v1","initiator":"alice","type":"PROJECT_CREATE","parties":["alice","bob"],"projectId":"p1"}}
	]}`
	var req VoteSyncRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := s.Sync(context.Background(), req.DbSyncRequests); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []interface{}{&model.VoteRequestDO{}, &model.VoteInviteDO{}, &model.NodeRouteDO{},
		&model.TeeNodeDatatableManagementDO{}, &model.ProjectDO{}, &model.ProjectNodeDO{}, &model.ProjectApprovalConfigDO{}} {
		var n int64
		db.Model(m).Count(&n)
		if n != 1 {
			t.Errorf("%T rows = %d", m, n)
		}
	}
	var inv model.VoteInviteDO
	db.First(&inv)
	if inv.VoteParticipantID != "bob" || inv.Initiator != "alice" {
		t.Fatalf("invite %+v", inv)
	}
	if err := s.Sync(context.Background(), []DbSyncRequest{{SyncDataType: "USER", ProjectNodesInfo: json.RawMessage(`{}`)}}); err == nil {
		t.Fatal("unknown type must fail")
	}
	if err := s.Sync(context.Background(), nil); err == nil {
		t.Fatal("empty request must fail")
	}
}

func TestManager_PlatformWiring(t *testing.T) {
	db := newTestDB(t)
	sse := service.NewSseServer(nil)
	defer sse.Stop()
	for _, tc := range []struct {
		platform          string
		center, p2p, edge bool
	}{{"CENTER", true, false, false}, {"AUTONOMY", false, true, false}, {"EDGE", false, false, true}} {
		env := service.NewEnvService(tc.platform, "n1", "", nil)
		m, err := NewManager(Options{DB: db, Env: env, Sse: sse, Gateway: "127.0.0.1:1"})
		if err != nil {
			t.Fatal(err)
		}
		if (m.Center != nil) != tc.center || (m.P2P != nil) != tc.p2p || (m.Edge != nil) != tc.edge {
			t.Errorf("%s: center=%v p2p=%v edge=%v", tc.platform, m.Center != nil, m.P2P != nil, m.Edge != nil)
		}
		if m.Consumer == nil || m.VoteSync == nil || m.Sender == nil {
			t.Errorf("%s: missing consumer/vote/sender", tc.platform)
		}
		m.Stop()
	}
}
