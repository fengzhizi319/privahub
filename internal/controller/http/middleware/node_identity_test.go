package middleware

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInnerGuard_MTLSNodeIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := InnerGuardWith(InnerGuardConfig{RequireClientCert: true, GatewayNames: []string{"kuscia-gateway"}})
	r := gin.New()
	r.POST("/x", h, func(c *gin.Context) { c.String(200, "ok") })
	do := func(cn string, sans []string, origin string) int {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		if cn != "" || sans != nil {
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{
				Subject: pkix.Name{CommonName: cn}, DNSNames: sans,
			}}}
		}
		if origin != "" {
			req.Header.Set(KusciaOriginSourceHeader, origin)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	cases := []struct {
		name       string
		cn         string
		sans       []string
		origin     string
		wantStatus int
	}{
		{"cn matches origin", "alice", nil, "alice", 200},
		{"san matches origin", "node", []string{"bob"}, "bob", 200},
		{"forged origin", "alice", nil, "bob", http.StatusForbidden},
		{"missing origin", "alice", nil, "", http.StatusForbidden},
		{"no client cert", "", nil, "alice", http.StatusUnauthorized},
		{"relay gateway may assert any origin", "kuscia-gateway", nil, "bob", 200},
	}
	for _, tc := range cases {
		if got := do(tc.cn, tc.sans, tc.origin); got != tc.wantStatus {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.wantStatus)
		}
	}
}

// TestTrustedProxies: by default X-Forwarded-For is ignored (ClientIP =
// RemoteAddr); only configured proxies may set it.
func TestTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ipOf := func(proxies []string, platform string, hdr map[string]string) string {
		r := gin.New()
		if err := ConfigureTrustedProxies(r, proxies, platform); err != nil {
			t.Fatal(err)
		}
		var got string
		r.GET("/x", func(c *gin.Context) { got = c.ClientIP() })
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.0.0.9:1234"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}
	spoof := map[string]string{"X-Forwarded-For": "1.2.3.4", "X-Real-IP": "1.2.3.4", "CF-Connecting-IP": "5.6.7.8"}
	if ip := ipOf(nil, "", spoof); ip != "10.0.0.9" {
		t.Errorf("default must ignore forwarding headers, got %s", ip)
	}
	if ip := ipOf([]string{"10.0.0.0/8"}, "", spoof); ip != "1.2.3.4" {
		t.Errorf("trusted proxy XFF not honoured, got %s", ip)
	}
	if ip := ipOf([]string{"192.168.0.1"}, "", spoof); ip != "10.0.0.9" {
		t.Errorf("untrusted peer XFF honoured, got %s", ip)
	}
	if ip := ipOf(nil, "CF-Connecting-IP", spoof); ip != "5.6.7.8" {
		t.Errorf("trusted platform header not honoured, got %s", ip)
	}
	if err := ConfigureTrustedProxies(gin.New(), []string{"bogus"}, ""); err == nil {
		t.Error("invalid proxy must fail")
	}
}

// TestMetricsGuard_BehindProxy: loopback is only trusted without a reverse
// proxy; behind one the token (or an allowlisted client) is required.
func TestMetricsGuard_BehindProxy(t *testing.T) {
	const tok = "metrics-token-0123456789"
	direct := MetricsGuardWith(MetricsGuardConfig{Token: tok})
	proxied := MetricsGuardWith(MetricsGuardConfig{Token: tok, BehindProxy: true, AllowedCIDRs: []string{"192.0.2.0/24"}})
	if s := guardStatus(t, direct, "127.0.0.1:5555", nil); s != 200 {
		t.Errorf("direct loopback denied: %d", s)
	}
	if s := guardStatus(t, proxied, "127.0.0.1:5555", nil); s != http.StatusForbidden {
		t.Errorf("loopback behind proxy must need the token: %d", s)
	}
	if s := guardStatus(t, proxied, "127.0.0.1:5555", map[string]string{"Authorization": "Bearer " + tok}); s != 200 {
		t.Errorf("token denied: %d", s)
	}
	// Spoofed X-Forwarded-For with the default (no trusted proxies) engine
	// does not grant loopback / allowlist access.
	if s := guardStatus(t, direct, "203.0.113.5:5555", map[string]string{"X-Forwarded-For": "127.0.0.1"}); s != http.StatusForbidden {
		t.Errorf("spoofed XFF granted access: %d", s)
	}
}
