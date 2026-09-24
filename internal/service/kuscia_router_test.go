package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeKusciaAPI is a minimal KusciaAPI HTTP server recording the calls it gets.
type fakeKusciaAPI struct {
	srv *httptest.Server

	mu      sync.Mutex
	calls   []string
	watches int // currently open watch streams
}

func newFakeKusciaAPI(t *testing.T) *fakeKusciaAPI {
	t.Helper()
	f := &fakeKusciaAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.URL.Path)
		f.mu.Unlock()
		if r.URL.Path == kuscia.WatchJobPath {
			f.mu.Lock()
			f.watches++
			f.mu.Unlock()
			defer func() {
				f.mu.Lock()
				f.watches--
				f.mu.Unlock()
			}()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"HEARTBEAT"}{"type":"MODIFIED","object":{"job_id":"unknown-job"}}`))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": map[string]any{"code": 0, "message": "success"},
			"data":   map[string]any{"job_id": "j", "status": map[string]any{"state": "Running"}},
		})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeKusciaAPI) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == path {
			n++
		}
	}
	return n
}

func (f *fakeKusciaAPI) openWatches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.watches
}

func (f *fakeKusciaAPI) endpoint(t *testing.T, domain string) kuscia.NodeEndpoint {
	t.Helper()
	host, port, err := net.SplitHostPort(f.srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := strconv.Atoi(port)
	return kuscia.NodeEndpoint{DomainID: domain, ClientConfig: kuscia.ClientConfig{Host: host, Port: p, Protocol: "notls", Timeout: 5 * time.Second}}
}

func (f *fakeKusciaAPI) client(t *testing.T) *kuscia.Client {
	ep := f.endpoint(t, "default")
	return kuscia.NewClient(&ep.ClientConfig)
}

func routerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.ProjectJobTaskDO{}, &model.ProjectJobDO{}, &model.ProjectScheduleJobDO{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestKusciaRouter_RoutesPerNodeWithDefaultFallback(t *testing.T) {
	def, alice := newFakeKusciaAPI(t), newFakeKusciaAPI(t)
	pool := kuscia.NewClientPool(def.client(t), "")
	if err := pool.Register(alice.endpoint(t, "alice")); err != nil {
		t.Fatal(err)
	}
	db := routerTestDB(t)
	r := NewKusciaRouter(pool, nil, db)
	ctx := context.Background()

	// CreateJob goes to the initiator's channel; later job calls follow it.
	if _, err := r.CreateJob(ctx, &kuscia.CreateJobRequest{JobID: "j1", Initiator: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := r.StopJob(ctx, "j1"); err != nil {
		t.Fatal(err)
	}
	if alice.count("/api/v1/job/create") != 1 || alice.count("/api/v1/job/stop") != 1 || def.count("/api/v1/job/create") != 0 {
		t.Fatalf("initiator routing: alice=%v default=%v", alice.calls, def.calls)
	}

	// Unregistered initiator -> default channel.
	if _, err := r.CreateJob(ctx, &kuscia.CreateJobRequest{JobID: "j2", Initiator: "bob"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.QueryJobDetail(ctx, "j2"); err != nil {
		t.Fatal(err)
	}
	if def.count("/api/v1/job/create") != 1 || def.count("/api/v1/job/query") != 1 {
		t.Fatalf("default fallback: default=%v", def.calls)
	}

	// Unknown job (e.g. after restart): resolved from the persisted task parties.
	db.Create(&model.ProjectJobTaskDO{ProjectID: "p", JobID: "j3", TaskID: "t", Parties: `["carol","alice"]`, Status: "RUNNING"})
	if _, err := r.QueryJobDetail(ctx, "j3"); err != nil {
		t.Fatal(err)
	}
	if alice.count("/api/v1/job/query") != 1 {
		t.Fatalf("party routing: alice=%v", alice.calls)
	}

	// Domain-scoped calls use the domain's channel.
	_, _ = r.QueryDomain(ctx, "alice")
	_, _ = r.QueryDomain(ctx, "bob")
	if alice.count("/api/v1/domain/query") != 1 || def.count("/api/v1/domain/query") != 1 {
		t.Fatalf("domain routing: alice=%v default=%v", alice.calls, def.calls)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestJobSync_WatchPerPoolNode(t *testing.T) {
	def, alice, bob := newFakeKusciaAPI(t), newFakeKusciaAPI(t), newFakeKusciaAPI(t)
	defClient := def.client(t)
	pool := kuscia.NewClientPool(defClient, "")
	if err := pool.Register(alice.endpoint(t, "alice")); err != nil {
		t.Fatal(err)
	}
	s := NewJobStatusSyncService(routerTestDB(t), defClient, zap.NewNop())
	s.SetKuscia(NewKusciaRouter(pool, defClient, nil))
	s.SetPool(pool)
	s.Start()
	defer s.Stop()

	waitFor(t, "default + alice watches", func() bool { return def.openWatches() == 1 && alice.openWatches() == 1 })
	waitFor(t, "watch connected", s.WatchConnected)

	// A node registered at runtime gets its own watch.
	if err := pool.Register(bob.endpoint(t, "bob")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob watch", func() bool { return bob.openWatches() == 1 })
	if got := s.WatchedNodes(); len(got) != 3 {
		t.Fatalf("watched nodes %v", got)
	}

	// A node served by an already watched KusciaAPI shares that stream.
	if err := pool.Register(bob.endpoint(t, "carol")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if bob.openWatches() != 1 || len(s.WatchedNodes()) != 3 {
		t.Fatalf("duplicate endpoint must not open a second watch: bob=%d nodes=%v", bob.openWatches(), s.WatchedNodes())
	}

	// Unregistering a node stops its watch.
	if err := pool.Unregister("alice"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "alice watch closed", func() bool { return alice.openWatches() == 0 })

	s.Stop()
	waitFor(t, "all watches closed", func() bool { return def.openWatches() == 0 && bob.openWatches() == 0 })
}
