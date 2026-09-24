package kuscia

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hostPort(t *testing.T, srv *httptest.Server) (string, int) {
	t.Helper()
	addr := srv.Listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestClient_TokenHeaderAndAPIError(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get(TokenHeader)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": map[string]interface{}{"code": 11305, "message": "domain not exists"},
		})
	}))
	defer srv.Close()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, port := hostPort(t, srv)
	c := NewClient(&ClientConfig{Host: host, Port: port, Protocol: "notls", Token: tokenFile})

	_, err := c.QueryDomain(context.Background(), "alice")
	if err == nil {
		t.Fatal("expected APIError")
	}
	if gotToken != "secret-token" {
		t.Errorf("Token header = %q, want secret-token (read from file)", gotToken)
	}
	if ErrorCode(err) != ErrCodeDomainNotExists || !IsNotFound(err) {
		t.Errorf("unexpected error mapping: %v (code %d)", err, ErrorCode(err))
	}
	exists, err := c.DomainExists(context.Background(), "alice")
	if err != nil || exists {
		t.Errorf("DomainExists = %v, %v; want false, nil", exists, err)
	}
}

func TestClient_HTTPErrorWithoutStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	c := NewClient(&ClientConfig{Host: host, Port: port})
	err := c.DeleteDomain(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected HTTP 502 error, got %v", err)
	}
}

func TestClient_TLSWithCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": 0}})
	}))
	defer srv.Close()
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	host, port := hostPort(t, srv)
	c, err := NewClientE(&ClientConfig{Host: host, Port: port, Protocol: "tls", CAFile: caFile, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClientE: %v", err)
	}
	if !strings.HasPrefix(c.BaseURL(), "https://") {
		t.Errorf("baseURL = %s", c.BaseURL())
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping over TLS failed: %v", err)
	}
	// Without the CA the handshake must fail.
	bad := NewClient(&ClientConfig{Host: host, Port: port, Protocol: "tls"})
	if err := bad.Ping(context.Background()); err == nil {
		t.Fatal("expected TLS verification failure without CA")
	}
}

func TestClient_MTLSRequiresClientCert(t *testing.T) {
	c := NewClient(&ClientConfig{Host: "127.0.0.1", Port: 1, Protocol: "mtls"})
	if c.Err() == nil {
		t.Fatal("expected init error for mtls without cert/key")
	}
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("requests must fail when init failed")
	}
}

func TestDomain_CreateQueryBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/v1/domain/create":
			ac, _ := body["auth_center"].(map[string]interface{})
			if ac["authentication_type"] != "Token" || ac["token_gen_method"] != "UID-RSA-GEN" {
				t.Errorf("auth_center = %v", body["auth_center"])
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": map[string]interface{}{"code": 0}})
		case "/api/v1/domain/query":
			_, _ = w.Write([]byte(`{"status":{"code":0},"data":{"domain_id":"alice","role":"","cert":"CERT",
			"node_statuses":[{"name":"n1","status":"NotReady"},{"name":"n2","status":"Ready","version":"v1"}],
			"deploy_token_statuses":[{"token":"t1","state":"used"},{"token":"t2","state":"unused","last_transition_time":"ts"}],
			"annotations":{"a":"b"}}}`))
		case "/api/v1/domain/batchQuery":
			ids := body["domain_ids"].([]interface{})
			if len(ids) != 2 {
				t.Errorf("domain_ids = %v", ids)
			}
			_, _ = w.Write([]byte(`{"status":{"code":0},"data":{"domains":[{"domain_id":"alice","node_statuses":[{"status":"Ready"}]},{"domain_id":"bob"}]}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	c := NewClient(&ClientConfig{Host: host, Port: port})
	ctx := context.Background()

	if err := c.CreateDomain(ctx, &CreateDomainRequest{DomainID: "alice",
		AuthCenter: &AuthCenter{AuthenticationType: AuthTypeToken, TokenGenMethod: TokenGenUIDRSAGen}}); err != nil {
		t.Fatal(err)
	}
	q, err := c.QueryDomain(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !q.Data.IsReady() || len(q.Data.DeployTokenStatuses) != 2 || q.Data.Cert != "CERT" || q.Data.Annotations["a"] != "b" {
		t.Errorf("unexpected domain info %+v", q.Data)
	}
	ds, err := c.BatchQueryDomain(ctx, []string{"alice", "bob"})
	if err != nil || len(ds) != 2 || !ds[0].IsReady() || ds[1].IsReady() {
		t.Errorf("batch = %+v, %v", ds, err)
	}
}

func TestRoute_ProtoShapeAndStatusObject(t *testing.T) {
	var created map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/route/create":
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"status":{"code":0},"data":{"name":"alice-bob"}}`))
		case "/api/v1/route/query":
			_, _ = w.Write([]byte(`{"status":{"code":0},"data":{"name":"alice-bob","source":"alice","destination":"bob","status":{"status":"Succeeded","reason":""}}}`))
		case "/api/v1/route/delete":
			_, _ = w.Write([]byte(`{"status":{"code":11404,"message":"route not exists"}}`))
		case "/api/v1/route/status/batchQuery":
			_, _ = w.Write([]byte(`{"status":{"code":0},"data":{"routes":[{"source":"alice","destination":"bob","status":{"status":"Failed","reason":"x"}}]}}`))
		}
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	c := NewClient(&ClientConfig{Host: host, Port: port})
	ctx := context.Background()

	req, err := NewTokenRouteRequest("alice", "bob", "https://10.0.0.2:1080")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CreateDomainRoute(ctx, req); err != nil {
		t.Fatal(err)
	}
	if created["authentication_type"] != "Token" {
		t.Errorf("authentication_type = %v", created["authentication_type"])
	}
	tc := created["token_config"].(map[string]interface{})
	if tc["token_gen_method"] != "RSA-GEN" {
		t.Errorf("token_config = %v", tc)
	}
	ep := created["endpoint"].(map[string]interface{})
	port0 := ep["ports"].([]interface{})[0].(map[string]interface{})
	if ep["host"] != "10.0.0.2" || port0["port"].(float64) != 1080 || port0["isTLS"] != true || port0["protocol"] != "HTTPS" || port0["name"] != "https" {
		t.Errorf("endpoint = %v", ep)
	}
	if _, bad := created["source_net_address"]; bad {
		t.Error("non-proto field source_net_address must not be sent")
	}

	q, err := c.QueryDomainRoute(ctx, "alice", "bob")
	if err != nil || q.Data.Status.Status != RouteStatusSucceeded {
		t.Errorf("query status = %+v, %v", q, err)
	}
	if err := c.DeleteDomainRoute(ctx, "alice", "bob"); err != nil {
		t.Errorf("11404 must be treated as deleted, got %v", err)
	}
	st, err := c.BatchQueryDomainRouteStatus(ctx, []QueryDomainRouteRequest{{Source: "alice", Destination: "bob"}})
	if err != nil || len(st) != 1 || st[0].Status.Status != RouteStatusFailed {
		t.Errorf("batch = %+v, %v", st, err)
	}
}

func TestBuildRouteEndpoint(t *testing.T) {
	cases := []struct {
		in       string
		host     string
		port     int32
		tls      bool
		wantFail bool
	}{
		{"alice:1080", "alice", 1080, false, false},
		{"http://127.0.0.1:28080", "127.0.0.1", 28080, false, false},
		{"https://bob.example.com", "bob.example.com", 443, true, false},
		{"http://host", "host", 80, false, false},
		{"", "", 0, false, true},
		{"http://:99", "", 0, false, true},
		{"host:99999", "", 0, false, true},
	}
	for _, c := range cases {
		ep, err := BuildRouteEndpoint(c.in)
		if c.wantFail {
			if err == nil {
				t.Errorf("%q: expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if ep.Host != c.host || ep.Ports[0].Port != c.port || ep.Ports[0].IsTLS != c.tls {
			t.Errorf("%q: got %+v", c.in, ep)
		}
	}
}

func TestClientPool_RegisterPersistLoad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":{"code":0}}`))
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	def := NewClient(&ClientConfig{Host: "127.0.0.1", Port: 1})
	file := filepath.Join(t.TempDir(), "kuscia", "nodes.json")
	pool := NewClientPool(def, file)

	var events []string
	pool.OnChange(func(ep NodeEndpoint, registered bool) {
		events = append(events, ep.DomainID+map[bool]string{true: "+", false: "-"}[registered])
	})

	if pool.Get("alice") != def || pool.IsRegistered("alice") {
		t.Fatal("unregistered domain must fall back to default")
	}
	// host:port in Host is split (Java validateAndProcess)
	err := pool.Register(NodeEndpoint{DomainID: "alice", Persist: true,
		ClientConfig: ClientConfig{Host: host + ":" + itoa(port), Protocol: "NOTLS"}})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := pool.Lookup("alice")
	if !ok || c == def {
		t.Fatal("alice not registered")
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("registered client unusable: %v", err)
	}
	// static (non-persisted) registration
	_ = pool.Register(NodeEndpoint{DomainID: "bob", ClientConfig: ClientConfig{Host: host, Port: port}})
	// tls without cert must be rejected
	if err := pool.Register(NodeEndpoint{DomainID: "carol", ClientConfig: ClientConfig{Host: host, Port: port, Protocol: "tls"}}); err == nil {
		t.Error("tls endpoint without cert/key/token must be rejected")
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"alice"`) || strings.Contains(string(data), `"bob"`) {
		t.Errorf("persist file = %s", data)
	}

	pool2 := NewClientPool(def, file)
	if err := pool2.Load(); err != nil {
		t.Fatal(err)
	}
	if !pool2.IsRegistered("alice") {
		t.Fatal("alice must be restored from pool file")
	}
	if err := pool2.Unregister("alice"); err != nil {
		t.Fatal(err)
	}
	pool3 := NewClientPool(def, file)
	_ = pool3.Load()
	if pool3.IsRegistered("alice") {
		t.Error("unregistered endpoint must be removed from pool file")
	}
	if strings.Join(events, ",") != "alice+,bob+" {
		t.Errorf("events = %v", events)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
