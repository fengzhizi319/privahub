package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeKuscia is an in-memory KusciaAPI (domain + route endpoints).
type fakeKuscia struct {
	mu               sync.Mutex
	domains          map[string]map[string]interface{}
	routes           map[string]map[string]interface{} // "src|dst" -> create request
	calls            []string
	failCreateDomain bool
	failCreateRoute  bool
	routeStatus      string
}

func newFakeKuscia(t *testing.T) (*fakeKuscia, *kuscia.Client, *httptest.Server) {
	t.Helper()
	f := &fakeKuscia{domains: map[string]map[string]interface{}{}, routes: map[string]map[string]interface{}{}, routeStatus: "Succeeded"}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	c := kuscia.NewClient(&kuscia.ClientConfig{Host: addr.IP.String(), Port: addr.Port, Protocol: "notls"})
	return f, c, srv
}

func (f *fakeKuscia) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeKuscia) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	str := func(k string) string { s, _ := body[k].(string); return s }
	ok := func(data interface{}) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": 0}, "data": data})
	}
	fail := func(code int, msg string) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": code, "message": msg}})
	}
	switch r.URL.Path {
	case "/api/v1/domain/create":
		f.calls = append(f.calls, "domain/create:"+str("domain_id"))
		if f.failCreateDomain {
			fail(11300, "create failed")
			return
		}
		f.domains[str("domain_id")] = body
		ok(nil)
	case "/api/v1/domain/query":
		id := str("domain_id")
		f.calls = append(f.calls, "domain/query:"+id)
		if _, exists := f.domains[id]; !exists {
			fail(11305, "not exists")
			return
		}
		ok(map[string]interface{}{
			"domain_id": id, "cert": "CERT-" + id,
			"node_statuses":         []map[string]string{{"name": id + "-0", "status": "Ready"}},
			"deploy_token_statuses": []map[string]string{{"token": "used-tok", "state": "used"}, {"token": "tok-" + id, "state": "unused", "last_transition_time": "t"}},
		})
	case "/api/v1/domain/batchQuery":
		f.calls = append(f.calls, "domain/batchQuery")
		var ds []map[string]interface{}
		for _, v := range body["domain_ids"].([]interface{}) {
			id := v.(string)
			if _, exists := f.domains[id]; exists {
				ds = append(ds, map[string]interface{}{"domain_id": id, "node_statuses": []map[string]string{{"status": "Ready"}}})
			}
		}
		ok(map[string]interface{}{"domains": ds})
	case "/api/v1/domain/delete":
		f.calls = append(f.calls, "domain/delete:"+str("domain_id"))
		delete(f.domains, str("domain_id"))
		ok(nil)
	case "/api/v1/route/create":
		key := str("source") + "|" + str("destination")
		f.calls = append(f.calls, "route/create:"+key)
		if f.failCreateRoute {
			fail(11400, "create route failed")
			return
		}
		f.routes[key] = body
		ok(map[string]string{"name": key})
	case "/api/v1/route/query":
		key := str("source") + "|" + str("destination")
		f.calls = append(f.calls, "route/query:"+key)
		if _, exists := f.routes[key]; !exists {
			fail(11404, "route not exists")
			return
		}
		ok(map[string]interface{}{"source": str("source"), "destination": str("destination"),
			"status": map[string]string{"status": f.routeStatus, "reason": ""}})
	case "/api/v1/route/delete":
		key := str("source") + "|" + str("destination")
		f.calls = append(f.calls, "route/delete:"+key)
		if _, exists := f.routes[key]; !exists {
			fail(11404, "route not exists")
			return
		}
		delete(f.routes, key)
		ok(nil)
	default:
		fail(404, "unknown path "+r.URL.Path)
	}
}

func setupNodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "node.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.NodeDO{}, &model.NodeRouteDO{}, &model.InstDO{}, &model.ProjectDO{},
		&model.ProjectNodeDO{}, &model.ProjectJobDO{}, &model.ProjectApprovalConfigDO{}, &model.ProjectResultDO{},
		&model.UserAccountsDO{}, &model.SysUserPermissionRelDO{}, &model.SysUserNodeRelDO{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func newTestNodeService(t *testing.T, db *gorm.DB, c *kuscia.Client, mode, localNode string) *NodeService {
	t.Helper()
	ns := NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), c, db)
	ns.Configure(NodeServiceOptions{
		Env:  NewEnvService(mode, localNode, "", nil),
		Pool: kuscia.NewClientPool(c, filepath.Join(t.TempDir(), "pool.json")),
	})
	return ns
}

func bizCode(err error) int {
	if be, ok := AsBizError(err); ok && be.Code != nil {
		return be.Code.Code
	}
	return 0
}

func TestNodeService_CreateNode_KusciaAndRollback(t *testing.T) {
	db := setupNodeTestDB(t)
	fk, c, _ := newFakeKuscia(t)
	ns := newTestNodeService(t, db, c, "center", "kuscia-system")
	ctx := context.Background()

	id, err := ns.CreateNode(ctx, &CreateNodeRequest{Name: "Node A", Mode: 1})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if !regexp.MustCompile(`^[a-z]{8}$`).MatchString(id) {
		t.Errorf("node id %q is not 8 lowercase letters", id)
	}
	var node model.NodeDO
	if err := db.Where("node_id = ?", id).First(&node).Error; err != nil {
		t.Fatal(err)
	}
	if node.NetAddress != id+":1080" || node.Type != "normal" || node.ControlNodeID != id || node.Mode != 1 {
		t.Errorf("node row = %+v", node)
	}
	var perm, users int64
	db.Model(&model.SysUserPermissionRelDO{}).Where("user_key = ? AND target_code = ?", id, auth.RoleEdgeNode).Count(&perm)
	db.Model(&model.UserAccountsDO{}).Where("name = ? AND owner_id = ?", id, id).Count(&users)
	if perm != 1 || users != 1 {
		t.Errorf("perm=%d users=%d, want 1/1", perm, users)
	}
	ac := fk.domains[id]["auth_center"].(map[string]interface{})
	if ac["authentication_type"] != "Token" || ac["token_gen_method"] != "UID-RSA-GEN" {
		t.Errorf("auth_center = %v", ac)
	}

	// Kuscia failure rolls everything back.
	fk.failCreateDomain = true
	if _, err := ns.CreateNode(ctx, &CreateNodeRequest{Name: "Node B"}); bizCode(err) != errcode.NodeCreateError.Code {
		t.Fatalf("expected NodeCreateError, got %v", err)
	}
	var nodes, allUsers int64
	db.Model(&model.NodeDO{}).Count(&nodes)
	db.Model(&model.UserAccountsDO{}).Count(&allUsers)
	if nodes != 1 || allUsers != 1 {
		t.Errorf("rollback failed: nodes=%d users=%d", nodes, allUsers)
	}
}

func TestNodeService_GetListAndToken(t *testing.T) {
	db := setupNodeTestDB(t)
	fk, c, _ := newFakeKuscia(t)
	ns := newTestNodeService(t, db, c, "center", "kuscia-system")
	ctx := context.Background()
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice", Type: "normal", NetAddress: "alice:1080"})
	db.Create(&model.NodeDO{NodeID: "ghost", Name: "ghost", ControlNodeID: "ghost", Type: "normal"})
	fk.domains["alice"] = map[string]interface{}{}

	vo, err := ns.GetNode(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if vo.NodeStatus != "Ready" || vo.Cert != "configured" || vo.CertText != "CERT-alice" || vo.Token != "tok-alice" ||
		vo.TokenStatus != "unused" || len(vo.NodeInstances) != 1 || vo.NodeAuthenticationCode == "" {
		t.Errorf("GetNode = %+v", vo)
	}
	var alice model.NodeDO
	db.Where("node_id = ?", "alice").First(&alice)
	if alice.Token != "tok-alice" {
		t.Errorf("token not written back: %q", alice.Token)
	}

	list, err := ns.ListNodes(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListNodes = %v, %v", list, err)
	}
	st := map[string]string{}
	for _, n := range list {
		st[n.NodeID] = n.NodeStatus
	}
	if st["alice"] != "Ready" || st["ghost"] != "NotReady" {
		t.Errorf("statuses = %v", st)
	}

	// EDGE user in CENTER mode only sees cooperating nodes.
	db.Create(&model.NodeRouteDO{RouteID: "alice__bob", SrcNodeID: "alice", DstNodeID: "bob"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "bob", ControlNodeID: "bob", Type: "normal"})
	edgeCtx := auth.ContextWithUser(ctx, &auth.UserContext{Name: "alice", OwnerType: auth.OwnerTypeEdge, OwnerID: "alice"})
	list, _ = ns.ListNodes(edgeCtx)
	if len(list) != 2 {
		t.Errorf("edge user should see alice+bob, got %d", len(list))
	}

	// token: refresh picks the unused deploy token.
	db.Model(&model.NodeDO{}).Where("node_id = ?", "alice").Update("token", "used-tok")
	tk, err := ns.GetNodeToken(ctx, "alice", false)
	if err != nil || tk.Token != "used-tok" || tk.TokenStatus != "used" {
		t.Errorf("token(no refresh) = %+v, %v", tk, err)
	}
	tk, err = ns.GetNodeToken(ctx, "alice", true)
	if err != nil || tk.Token != "tok-alice" || tk.TokenStatus != "unused" {
		t.Errorf("token(refresh) = %+v, %v", tk, err)
	}
	if _, err := ns.GetNodeToken(ctx, "nope", true); err != ErrNodeNotFound {
		t.Errorf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestNodeService_DeleteNode(t *testing.T) {
	db := setupNodeTestDB(t)
	fk, c, _ := newFakeKuscia(t)
	ns := newTestNodeService(t, db, c, "center", "kuscia-system")
	ctx := context.Background()
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice", Type: "embedded"})
	db.Create(&model.NodeDO{NodeID: "nodea", Name: "a", ControlNodeID: "nodea", Type: "normal"})
	db.Create(&model.NodeRouteDO{RouteID: "nodea__alice", SrcNodeID: "nodea", DstNodeID: "alice"})
	db.Create(&model.NodeRouteDO{RouteID: "alice__nodea", SrcNodeID: "alice", DstNodeID: "nodea"})
	db.Create(&model.SysUserPermissionRelDO{UserType: "NODE", UserKey: "nodea", TargetType: "ROLE", TargetCode: auth.RoleEdgeNode})
	fk.domains["nodea"] = map[string]interface{}{}
	fk.routes["nodea|alice"] = map[string]interface{}{}

	if err := ns.DeleteNode(ctx, "alice"); bizCode(err) != errcode.SystemValidationError.Code {
		t.Errorf("embedded node must be protected, got %v", err)
	}
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "nodea"})
	if err := ns.DeleteNode(ctx, "nodea"); bizCode(err) != errcode.NodeDeleteError.Code {
		t.Errorf("node in project must not be deleted, got %v", err)
	}
	db.Where("project_id = ?", "p1").Delete(&model.ProjectNodeDO{})
	if err := ns.DeleteNode(ctx, "nodea"); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	var routes, perms, nodes int64
	db.Model(&model.NodeRouteDO{}).Count(&routes)
	db.Model(&model.SysUserPermissionRelDO{}).Count(&perms)
	db.Model(&model.NodeDO{}).Where("node_id = ?", "nodea").Count(&nodes)
	if routes != 0 || perms != 0 || nodes != 0 {
		t.Errorf("leftovers routes=%d perms=%d nodes=%d", routes, perms, nodes)
	}
	if fk.called("domain/delete:nodea") != 1 || fk.called("route/delete:") != 2 {
		t.Errorf("kuscia calls = %v", fk.calls)
	}
	if _, exists := fk.routes["nodea|alice"]; exists {
		t.Error("kuscia route must be removed")
	}
}

func TestNodeRouteService_CreateUpdateRefreshDelete(t *testing.T) {
	db := setupNodeTestDB(t)
	fk, c, _ := newFakeKuscia(t)
	ns := newTestNodeService(t, db, c, "center", "kuscia-system")
	rs := ns.Routes()
	ctx := context.Background()
	db.Create(&model.NodeDO{NodeID: "alice", Name: "alice", ControlNodeID: "alice", NetAddress: "http://10.0.0.1:1080"})
	db.Create(&model.NodeDO{NodeID: "bob", Name: "bob", ControlNodeID: "bob", NetAddress: "https://10.0.0.2:2080"})

	if err := ns.CreateRoute(ctx, &CreateRouteRequest{SrcNodeID: "alice", DstNodeID: "bob"}); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	fwd := fk.routes["alice|bob"]
	ep := fwd["endpoint"].(map[string]interface{})
	p := ep["ports"].([]interface{})[0].(map[string]interface{})
	if ep["host"] != "10.0.0.2" || p["port"].(float64) != 2080 || p["isTLS"] != true || fwd["authentication_type"] != "Token" {
		t.Errorf("forward route = %v", fwd)
	}
	if _, ok := fk.routes["bob|alice"]; !ok {
		t.Error("FullDuplex must create the reverse Kuscia route")
	}
	var ids []string
	db.Model(&model.NodeRouteDO{}).Order("route_id").Pluck("route_id", &ids)
	if strings.Join(ids, ",") != "alice__bob,bob__alice" {
		t.Errorf("route ids = %v", ids)
	}

	vo, err := rs.Refresh(ctx, "alice__bob")
	if err != nil || vo.Status != "Succeeded" || vo.SrcNode == nil || vo.DstNode == nil {
		t.Errorf("Refresh = %+v, %v", vo, err)
	}

	// Update rebuilds the Kuscia route with the new address.
	if err := rs.Update(ctx, &UpdateNodeRouterRequest{RouterID: "alice__bob", DstNetAddress: "10.9.9.9:3080"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	ep = fk.routes["alice|bob"]["endpoint"].(map[string]interface{})
	if ep["host"] != "10.9.9.9" || fk.called("route/delete:alice|bob") != 1 {
		t.Errorf("update did not rebuild route: %v calls=%v", ep, fk.calls)
	}
	var r model.NodeRouteDO
	db.Where("route_id = ?", "alice__bob").First(&r)
	if r.DstNetAddress != "10.9.9.9:3080" {
		t.Errorf("db dst address = %q", r.DstNetAddress)
	}

	// Running job in a shared project blocks update/delete.
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "bob"})
	db.Create(&model.ProjectJobDO{ProjectID: "p1", JobID: "j1", Name: "j", Status: "RUNNING"})
	if err := rs.Update(ctx, &UpdateNodeRouterRequest{RouterID: "alice__bob", DstNetAddress: "1.1.1.1:1"}); bizCode(err) != errcode.NodeRouteUpdateError.Code {
		t.Errorf("expected NodeRouteUpdateError, got %v", err)
	}
	page, err := rs.Page(ctx, &PageNodeRouteRequest{})
	if err != nil || page.Total != 2 || !page.List[0].IsProjectJobRunning {
		t.Errorf("Page = %+v, %v", page, err)
	}
	db.Model(&model.ProjectJobDO{}).Where("job_id = ?", "j1").Update("status", "SUCCEED")

	// Default route ids are protected.
	db.Create(&model.NodeRouteDO{RouteID: "1", SrcNodeID: "x", DstNodeID: "y"})
	if err := rs.Delete(ctx, "1"); bizCode(err) != errcode.SystemValidationError.Code {
		t.Errorf("default route delete: %v", err)
	}
	// Delete: Kuscia 11404 is tolerated.
	delete(fk.routes, "bob|alice")
	if err := rs.Delete(ctx, "bob__alice"); err != nil {
		t.Fatalf("Delete with 11404: %v", err)
	}
	if err := rs.Delete(ctx, "alice__bob"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(fk.routes) != 0 {
		t.Errorf("kuscia routes left: %v", fk.routes)
	}
	if err := rs.Delete(ctx, "alice__bob"); bizCode(err) != errcode.NodeRouteNotExistError.Code {
		t.Errorf("expected not-exist, got %v", err)
	}

	// Kuscia create failure rolls back the DB rows.
	fk.failCreateRoute = true
	if err := ns.CreateRoute(ctx, &CreateRouteRequest{SrcNodeID: "alice", DstNodeID: "bob"}); bizCode(err) != errcode.NodeRouteCreateError.Code {
		t.Errorf("expected NodeRouteCreateError, got %v", err)
	}
	var cnt int64
	db.Model(&model.NodeRouteDO{}).Where("src_node_id IN ?", []string{"alice", "bob"}).Count(&cnt)
	if cnt != 0 {
		t.Errorf("route rows not rolled back: %d", cnt)
	}
}

func selfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "node"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})
}

func TestInstService_Flow(t *testing.T) {
	db := setupNodeTestDB(t)
	fk, c, srv := newFakeKuscia(t)
	ns := newTestNodeService(t, db, c, "autonomy", "local1")
	certsDir := t.TempDir()
	is := NewInstService(db, ns, certsDir, "", nil)
	db.Create(&model.InstDO{InstID: "inst1", Name: "Inst One"})
	ctx := auth.ContextWithUser(context.Background(), &auth.UserContext{Name: "u", OwnerID: "inst1"})

	// get: owner mismatch / ok
	if _, err := is.GetInst(ctx, "other"); bizCode(err) != errcode.InstMismatchLogin.Code {
		t.Errorf("expected InstMismatchLogin, got %v", err)
	}
	if vo, err := is.GetInst(ctx, "inst1"); err != nil || vo.InstName != "Inst One" {
		t.Errorf("GetInst = %+v, %v", vo, err)
	}

	tok, err := is.CreateNode(ctx, &InstCreateNodeRequest{Name: "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyInstToken(tok.InstToken, "inst1", tok.NodeID) || VerifyInstToken(tok.InstToken, "inst2", tok.NodeID) {
		t.Error("inst token must verify with instId secret and nodeId audience only")
	}
	got, _ := is.GetToken(ctx, tok.NodeID)
	if got.InstToken != tok.InstToken || got.InstTokenState != InstTokenUnused {
		t.Errorf("GetToken = %+v", got)
	}
	nt, err := is.NewToken(ctx, tok.NodeID)
	if err != nil || nt.InstToken == "" {
		t.Fatalf("NewToken: %v", err)
	}

	// register: stale token rejected; valid token registers the channel.
	certPEM, keyPEM := selfSignedPEM(t)
	addr := srv.Listener.Addr().(*net.TCPAddr)
	req := &InstRegisterRequest{InstToken: tok.InstToken, NodeID: tok.NodeID, Port: 8083, HTTPPort: addr.Port, TransPort: 1080,
		Mode: "p2p", Protocol: "notls", CertFile: certPEM, KeyFile: keyPEM, Token: []byte("kt")}
	if err := is.RegisterNode(context.Background(), req, "127.0.0.1"); bizCode(err) != errcode.InstTokenMismatch.Code {
		t.Errorf("old token must be rejected, got %v", err)
	}
	req.InstToken = nt.InstToken
	req.CertFile = []byte("garbage")
	if err := is.RegisterNode(context.Background(), req, "127.0.0.1"); bizCode(err) != errcode.InstRegisterCheckFailed.Code {
		t.Errorf("bad cert must be rejected, got %v", err)
	}
	req.CertFile = certPEM
	// Production refuses plaintext KusciaAPI channels.
	is.WithRequireTLS(true)
	if err := is.RegisterNode(context.Background(), req, "127.0.0.1"); bizCode(err) != errcode.InstRegisterCheckFailed.Code {
		t.Errorf("notls registration must be refused in production, got %v", err)
	}
	is.WithRequireTLS(false)
	if err := is.RegisterNode(context.Background(), req, "127.0.0.1"); err != nil {
		t.Fatalf("RegisterNode: %v", err)
	}
	if !ns.Pool().IsRegistered(tok.NodeID) {
		t.Fatal("channel not registered")
	}
	var node model.NodeDO
	db.Where("node_id = ?", tok.NodeID).First(&node)
	if node.InstTokenState != InstTokenUsed || node.NetAddress != "127.0.0.1:1080" || node.Protocol != "notls" {
		t.Errorf("node after register = %+v", node)
	}
	if err := is.RegisterNode(context.Background(), req, "127.0.0.1"); bizCode(err) != errcode.InstTokenUsed.Code {
		t.Errorf("used token must be rejected, got %v", err)
	}

	// list via the registered channel
	fk.domains[tok.NodeID] = map[string]interface{}{}
	nodes, err := is.ListNodes(ctx)
	if err != nil || len(nodes) != 1 || nodes[0].NodeStatus != "Ready" || nodes[0].InstName != "Inst One" || !nodes[0].AllowDeletion {
		t.Errorf("ListNodes = %+v, %v", nodes, err)
	}

	// delete: local node forbidden; route blocks; ok deletes domain and unregisters.
	if err := is.DeleteNode(ctx, "local1"); bizCode(err) != errcode.NodeDeleteError.Code {
		t.Errorf("local node delete: %v", err)
	}
	db.Create(&model.NodeRouteDO{RouteID: "r", SrcNodeID: tok.NodeID, DstNodeID: "x"})
	if err := is.DeleteNode(ctx, tok.NodeID); bizCode(err) != errcode.NodeDeleteError.Code {
		t.Errorf("node with route delete: %v", err)
	}
	db.Where("route_id = ?", "r").Delete(&model.NodeRouteDO{})
	if err := is.DeleteNode(ctx, tok.NodeID); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	if ns.Pool().IsRegistered(tok.NodeID) || fk.called("domain/delete:"+tok.NodeID) != 1 {
		t.Errorf("domain not deleted/unregistered: %v", fk.calls)
	}

	// MAX_NODE_NUM
	for i := 0; i < InstMaxNodeNum; i++ {
		if _, err := is.CreateNode(ctx, &InstCreateNodeRequest{Name: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := is.CreateNode(ctx, &InstCreateNodeRequest{Name: "n"}); bizCode(err) != errcode.InstNodeCountLimited.Code {
		t.Errorf("expected InstNodeCountLimited, got %v", err)
	}
}

func TestCertificateService_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/certificate/generate" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":{"code":0},"key":"K","cert_chain":["C1","C2"]}`))
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().(*net.TCPAddr)
	c := kuscia.NewClient(&kuscia.ClientConfig{Host: addr.IP.String(), Port: addr.Port})
	db := setupNodeTestDB(t)
	cs := NewCertificateService(NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), c, db))
	kc, err := cs.GenerateKeyCerts(context.Background(), "alice", "", 3600)
	if err != nil || kc.Key != "K" || len(kc.CertChain) != 2 {
		t.Errorf("GenerateKeyCerts = %+v, %v", kc, err)
	}
}
