package v1

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/dao/repository"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/kuscia"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func instTestSetup(t *testing.T) (*gorm.DB, *service.NodeService, *InstHandler) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "inst.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.NodeDO{}, &model.NodeRouteDO{}, &model.InstDO{}, &model.ProjectNodeDO{}); err != nil {
		t.Fatal(err)
	}
	ns := service.NewNodeService(repository.NewNodeRepo(db), repository.NewNodeRouteRepo(db), nil, db)
	ns.Configure(service.NodeServiceOptions{Pool: kuscia.NewClientPool(nil, "")})
	return db, ns, NewInstHandler(db, service.NewInstService(db, ns, t.TempDir(), "", nil))
}

func testCertPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "n"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})
}

func postMultipart(r http.Handler, path string, fields map[string]string, files map[string][]byte) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for k, v := range files {
		fw, _ := mw.CreateFormFile(k, k)
		_, _ = fw.Write(v)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func respCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var resp struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad body %s", w.Body.String())
	}
	return resp.Status.Code
}

func TestInstHandler_RegisterMultipartJSONData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, ns, h := instTestSetup(t)
	r := gin.New()
	r.POST("/inst/node/register", h.RegisterInstNode)

	db.Create(&model.InstDO{InstID: "inst1", Name: "I"})
	tok, err := service.SignInstToken("inst1", "nodex")
	if err != nil {
		t.Fatal(err)
	}
	db.Create(&model.NodeDO{NodeID: "nodex", Name: "x", ControlNodeID: "nodex", InstID: "inst1",
		InstToken: tok, InstTokenState: service.InstTokenUnused})

	// missing json_data
	w := postMultipart(r, "/inst/node/register", nil, nil)
	if respCode(t, w) != errcode.InstRegisterCheckFailed.Code {
		t.Errorf("missing json_data: %s", w.Body.String())
	}

	cert, key := testCertPEM(t)
	jd, _ := json.Marshal(map[string]interface{}{
		"token": tok, "domainId": "nodex", "host": "ignored", "port": 8083, "transPort": 1080,
		"mode": "p2p", "protocol": "notls",
	})
	w = postMultipart(r, "/inst/node/register", map[string]string{"json_data": string(jd)},
		map[string][]byte{"certFile": cert, "keyFile": key, "token": []byte("kt")})
	if respCode(t, w) != 0 {
		t.Fatalf("register failed: %s", w.Body.String())
	}
	if !ns.Pool().IsRegistered("nodex") {
		t.Fatal("nodex channel not registered")
	}
	ep := ns.Pool().Endpoints()[0]
	if ep.Port != 8082 || ep.Host != "192.0.2.1" {
		t.Errorf("endpoint = %+v (8083 must map to HTTP 8082, host = remote addr)", ep.ClientConfig)
	}
	var n model.NodeDO
	db.Where("node_id = ?", "nodex").First(&n)
	if n.InstTokenState != service.InstTokenUsed {
		t.Errorf("token state = %s", n.InstTokenState)
	}
}

func TestNodeRouteHandler_CamelCaseContract(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := NewNodeRouteHandler(db, nil)
	r := setupTestRouter()
	r.POST("/get", h.Get)
	db.Create(&model.NodeDO{NodeID: "alice", Name: "Alice", ControlNodeID: "alice"})
	db.Create(&model.NodeRouteDO{RouteID: "alice__bob", SrcNodeID: "alice", DstNodeID: "bob", DstNetAddress: "bob:1080"})

	w := postJSON(r, "/get", map[string]interface{}{"routerId": "alice__bob"})
	var resp struct {
		Data struct {
			RouteID       string `json:"routeId"`
			DstNetAddress string `json:"dstNetAddress"`
			SrcNode       *struct {
				NodeID string `json:"nodeId"`
			} `json:"srcNode"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.RouteID != "alice__bob" || resp.Data.DstNetAddress != "bob:1080" || resp.Data.SrcNode == nil || resp.Data.SrcNode.NodeID != "alice" {
		t.Errorf("get = %s", w.Body.String())
	}
}
