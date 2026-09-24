package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	migration "github.com/fengzhizi319/privahub/internal/dao/migrations"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:boot_%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := migration.AutoMigrate(db, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	return db
}

// fakeKuscia implements DomainAPI / EndpointRegistrar / DataProxyUpdater.
type fakeKuscia struct {
	mu        sync.Mutex
	domains   map[string]bool
	created   []string
	endpoints []kuscia.NodeEndpoint
	proxied   []string
	down      bool
}

func (f *fakeKuscia) QueryDomain(_ context.Context, id string) (*kuscia.QueryDomainResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, fmt.Errorf("connection refused")
	}
	if !f.domains[id] {
		return nil, &kuscia.APIError{Code: 11100, Message: "not found"}
	}
	return &kuscia.QueryDomainResponse{Data: kuscia.DomainInfo{DomainID: id}}, nil
}
func (f *fakeKuscia) CreateDomain(_ context.Context, req *kuscia.CreateDomainRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.domains[req.DomainID] = true
	f.created = append(f.created, req.DomainID+":"+req.AuthCenter.TokenGenMethod)
	return nil
}
func (f *fakeKuscia) Register(ep kuscia.NodeEndpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.endpoints = append(f.endpoints, ep)
	return nil
}
func (f *fakeKuscia) UpdateDomain(_ context.Context, id string) {
	f.mu.Lock()
	f.proxied = append(f.proxied, id)
	f.mu.Unlock()
}
func (f *fakeKuscia) UpdateDataSourceUseDataProxyInP2p(_ context.Context, inst string) {
	f.mu.Lock()
	f.proxied = append(f.proxied, "inst:"+inst)
	f.mu.Unlock()
}

func newFake() *fakeKuscia { return &fakeKuscia{domains: map[string]bool{}} }

func TestEdgeResourceInit(t *testing.T) {
	db := newDB(t)
	db.Create(&model.UserAccountsDO{Name: "admin", PasswordHash: "x", OwnerID: "edge1"})
	db.Create(&model.NodeDO{NodeID: "edge1", Name: "old", ControlNodeID: "x", Token: "t", Type: "embedded"})
	fk := newFake()
	env := service.NewEnvService("EDGE", "edge1", "", nil)
	i := New(Deps{DB: db, Env: env, Kuscia: fk, DataProxy: fk})

	fk.down = true
	if err := i.Run(context.Background()); err == nil {
		t.Fatal("kuscia down must be reported")
	}
	fk.down = false
	fk.domains["edge1"] = true
	for k := 0; k < 2; k++ { // idempotent
		if err := i.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var n model.NodeDO
	db.Where("node_id = ?", "edge1").First(&n)
	if n.ControlNodeID != "edge1" || n.Type != "normal" || n.Mode != 1 || n.Token != "" {
		t.Fatalf("node %+v", n)
	}
	var cnt int64
	db.Model(&model.NodeDO{}).Where("node_id = ?", "edge1").Count(&cnt)
	if cnt != 1 || len(fk.proxied) != 2 || fk.proxied[0] != "edge1" {
		t.Fatalf("rows=%d proxied=%v", cnt, fk.proxied)
	}
}

func TestP2pDataInit(t *testing.T) {
	db := newDB(t)
	db.Create(&model.UserAccountsDO{Name: "admin", PasswordHash: "x", OwnerID: "n1"})
	fk := newFake()
	env := service.NewEnvService("AUTONOMY", "n1", "", nil)
	i := New(Deps{DB: db, Env: env, DataProxy: fk, KusciaProtocol: "notls", InstName: "Org"})
	<-i.Start(context.Background())
	inst := env.GetInstID()
	if len(inst) != 8 {
		t.Fatalf("inst id %q", inst)
	}
	var node model.NodeDO
	db.Where("node_id = ?", "n1").First(&node)
	if node.InstID != inst || node.Type != "primary" || node.MasterNodeID != "n1" || node.Protocol != "notls" {
		t.Fatalf("node %+v", node)
	}
	var acc model.UserAccountsDO
	db.Where("name = ?", "admin").First(&acc)
	if acc.OwnerID != inst || acc.InstID != inst {
		t.Fatalf("account %+v", acc)
	}
	if !env.IsNodeInCurrentInst("n1") {
		t.Fatal("local node must be in the current inst")
	}
	// re-run keeps the inst
	env2 := service.NewEnvService("AUTONOMY", "n1", "", nil)
	if err := New(Deps{DB: db, Env: env2}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var insts int64
	db.Model(&model.InstDO{}).Count(&insts)
	if env2.GetInstID() != inst || insts != 1 {
		t.Fatalf("inst changed %q -> %q (insts=%d)", inst, env2.GetInstID(), insts)
	}
	if fk.proxied[0] != "inst:"+inst {
		t.Fatalf("dataproxy %v", fk.proxied)
	}
}

func TestCenterInits_MasterRouteAndTee(t *testing.T) {
	db := newDB(t)
	db.Create(&model.NodeRouteDO{RouteID: "1", SrcNodeID: "alice", DstNodeID: "bob", SrcNetAddress: "127.0.0.1:28080", DstNetAddress: "127.0.0.1:38080"})
	db.Create(&model.NodeRouteDO{RouteID: "2", SrcNodeID: "bob", DstNodeID: "alice", SrcNetAddress: "https://b:1", DstNetAddress: "https://a:1"})
	fk := newFake()
	env := service.NewEnvService("CENTER", "kuscia-system", "", nil)
	env.SetDeployMode("ALL-IN-ONE")
	i := New(Deps{DB: db, Env: env, Kuscia: fk, Pool: fk, KusciaProtocol: "notls",
		Tee: TeeOptions{Enabled: true, Address: "tee-host", Port: 8082}})
	for k := 0; k < 2; k++ {
		if err := i.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var r1, r2 model.NodeRouteDO
	db.Where("route_id = ?", "1").First(&r1)
	db.Where("route_id = ?", "2").First(&r2)
	if r1.SrcNetAddress != "http://127.0.0.1:28080" || r1.DstNetAddress != "http://127.0.0.1:38080" || r2.SrcNetAddress != "https://b:1" {
		t.Fatalf("routes %+v %+v", r1, r2)
	}
	var tee model.NodeDO
	if err := db.Where("node_id = ?", "tee").First(&tee).Error; err != nil || tee.Type != "embedded" {
		t.Fatalf("tee node %+v %v", tee, err)
	}
	var routes int64
	db.Model(&model.NodeRouteDO{}).Where("src_node_id = ? OR dst_node_id = ?", "tee", "tee").Count(&routes)
	if routes != 4 {
		t.Fatalf("tee routes %d", routes)
	}
	if len(fk.created) != 1 || fk.created[0] != "tee:UID-RSA-GEN" {
		t.Fatalf("created domains %v", fk.created)
	}
	if len(fk.endpoints) != 2 || fk.endpoints[0].DomainID != "tee" || fk.endpoints[0].Host != "tee-host" ||
		!strings.HasSuffix(fk.endpoints[0].CertFile, "tee/client.crt") {
		t.Fatalf("endpoints %+v", fk.endpoints)
	}

	// MPC deploy mode / disabled: no tee
	db2 := newDB(t)
	env2 := service.NewEnvService("CENTER", "kuscia-system", "", nil)
	env2.SetDeployMode("MPC")
	if err := New(Deps{DB: db2, Env: env2, Kuscia: fk, Tee: TeeOptions{Enabled: true}}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var c int64
	db2.Model(&model.NodeDO{}).Where("node_id = ?", "tee").Count(&c)
	if c != 0 {
		t.Fatal("tee must not be created in MPC mode")
	}
}
