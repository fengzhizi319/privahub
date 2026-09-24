package votehandler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testPKI is a CA + leaf key pair in the Kuscia GenerateKeyCerts format.
type testPKI struct {
	caPEM   string
	leafPEM string
	keyB64  string // base64(PEM PKCS#8)
	// caKeyB64 is the domain (CA) key, base64(PEM PKCS#8).
	caKeyB64 string
}

func newTestPKI(t *testing.T, cn string) *testPKI {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caTpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn + "-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "vote"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		// Kuscia GenerateKeyCerts: KeyEncipherment | DigitalSignature.
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPKCS8, _ := x509.MarshalPKCS8PrivateKey(caKey)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(leafKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	return &testPKI{
		caPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})),
		leafPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})),
		keyB64:  base64.StdEncoding.EncodeToString(keyPEM),
		caKeyB64: base64.StdEncoding.EncodeToString(
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: caPKCS8})),
	}
}

func (p *testPKI) keyCerts() *service.KeyCerts {
	return &service.KeyCerts{Key: p.keyB64, CertChain: []string{b64(p.leafPEM), b64(p.caPEM)}}
}

// fakeKuscia implements KusciaOps. Every node shares one PKI unless set.
type fakeKuscia struct {
	mu        sync.Mutex
	pki       map[string]*testPKI
	def       *testPKI
	domains   map[string]string // channel/domain -> cert
	routes    map[string]bool   // channel/src/dst
	deleted   []string
	failCerts bool
}

func newFakeKuscia(t *testing.T) *fakeKuscia {
	return &fakeKuscia{pki: map[string]*testPKI{}, def: newTestPKI(t, "kuscia"),
		domains: map[string]string{}, routes: map[string]bool{}}
}

func (f *fakeKuscia) pkiOf(node string) *testPKI {
	if p, ok := f.pki[node]; ok {
		return p
	}
	return f.def
}

func (f *fakeKuscia) GenerateKeyCerts(_ context.Context, nodeID string) (*service.KeyCerts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failCerts {
		return nil, errors.New("kuscia down")
	}
	return f.pkiOf(nodeID).keyCerts(), nil
}

func (f *fakeKuscia) DomainCert(_ context.Context, nodeID, _ string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return b64(f.pkiOf(nodeID).caPEM)
}

func (f *fakeKuscia) CreatePartnerDomain(_ context.Context, channel, domainID, cert string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.domains[channel+"/"+domainID] = cert
	return nil
}

func (f *fakeKuscia) DeleteDomain(_ context.Context, channel, domainID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, "domain:"+channel+"/"+domainID)
	delete(f.domains, channel+"/"+domainID)
	return nil
}

func (f *fakeKuscia) DomainRouteExists(_ context.Context, channel, src, dst string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.routes[channel+"/"+src+"/"+dst], nil
}

func (f *fakeKuscia) DomainRouteReady(context.Context, string, string, string) bool { return true }

func (f *fakeKuscia) DeleteDomainRoute(_ context.Context, channel, src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, "route:"+channel+"/"+src+"/"+dst)
	delete(f.routes, channel+"/"+src+"/"+dst)
	return nil
}

// fakeRoutes implements RouteCreator by inserting node_route rows.
type fakeRoutes struct {
	db   *gorm.DB
	reqs []service.CreateRouteRequest
	err  error
}

func (r *fakeRoutes) CreateRoute(ctx context.Context, req *service.CreateRouteRequest) error {
	r.reqs = append(r.reqs, *req)
	if r.err != nil {
		return r.err
	}
	return r.db.WithContext(ctx).Create(&model.NodeRouteDO{RouteID: req.SrcNodeID + "__" + req.DstNodeID,
		SrcNodeID: req.SrcNodeID, DstNodeID: req.DstNodeID, SrcNetAddress: req.SrcNetAddress,
		DstNetAddress: req.DstNetAddress}).Error
}

type fakeSender struct{ reqs []DbSyncRequest }

func (s *fakeSender) SendToCenter(_ context.Context, reqs ...DbSyncRequest) error {
	s.reqs = append(s.reqs, reqs...)
	return nil
}

type fakePuller struct{ calls []string }

func (p *fakePuller) PullResultFromTeeNode(_ context.Context, nodeID, resourceID, voteResult, _, _, _, _ string) error {
	p.calls = append(p.calls, nodeID+"/"+resourceID+"/"+voteResult)
	return nil
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vote.db")+"?_pragma=busy_timeout(5000)"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.VoteRequestDO{}, &model.VoteInviteDO{}, &model.TeeDownloadApprovalConfigDO{},
		&model.NodeRouteApprovalConfigDO{}, &model.ProjectApprovalConfigDO{}, &model.NodeDO{}, &model.InstDO{},
		&model.NodeRouteDO{}, &model.ProjectDO{}, &model.ProjectNodeDO{}, &model.ProjectInstDO{},
		&model.ProjectJobDO{}, &model.ProjectJobTaskDO{}, &model.ProjectGraphDO{}, &model.ProjectDatatableDO{},
		&model.SysUserPermissionRelDO{}); err != nil {
		t.Fatal(err)
	}
	return db
}

type testEnv struct {
	db     *gorm.DB
	kuscia *fakeKuscia
	routes *fakeRoutes
	svc    *Service
	mon    *Monitor
	p2p    *P2PService
}

// newTestEnv builds a vote system for mode (center / autonomy / edge).
func newTestEnv(t *testing.T, mode, nodeID, instID string) *testEnv {
	t.Helper()
	db := newTestDB(t)
	env := service.NewEnvService(mode, nodeID, instID, nil)
	if env.IsCenter() {
		env.SetEmbeddedNodes([]string{"alice", "bob", "tee"})
	}
	fk := newFakeKuscia(t)
	fr := &fakeRoutes{db: db}
	d := &Deps{DB: db, Env: env, Kuscia: fk, Routes: fr}
	svc := NewService(d)
	return &testEnv{db: db, kuscia: fk, routes: fr, svc: svc, mon: NewMonitor(svc, MonitorOptions{Interval: 10 * time.Millisecond}),
		p2p: NewP2PService(d)}
}

func userCtx(owner, ownerType, platform string) context.Context {
	return auth.ContextWithUser(context.Background(), &auth.UserContext{Name: owner, OwnerID: owner,
		OwnerType: ownerType, PlatformType: platform})
}

func (e *testEnv) mustCreate(t *testing.T, v interface{}) {
	t.Helper()
	if err := e.db.Create(v).Error; err != nil {
		t.Fatal(err)
	}
}

func (e *testEnv) request(t *testing.T, voteID string) model.VoteRequestDO {
	t.Helper()
	var vr model.VoteRequestDO
	if err := e.db.Where("vote_id = ?", voteID).First(&vr).Error; err != nil {
		t.Fatal(err)
	}
	return vr
}

func (e *testEnv) invite(t *testing.T, voteID, participant string) model.VoteInviteDO {
	t.Helper()
	var inv model.VoteInviteDO
	if err := e.db.Where("vote_id = ? AND vote_participant_id = ?", voteID, participant).First(&inv).Error; err != nil {
		t.Fatal(err)
	}
	return inv
}

func bizCode(err error) int {
	if be, ok := service.AsBizError(err); ok && be.Code != nil {
		return be.Code.Code
	}
	return 0
}
