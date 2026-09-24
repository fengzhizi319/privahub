package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"github.com/gin-gonic/gin"
)

func guardStatus(t *testing.T, h gin.HandlerFunc, remote string, hdr map[string]string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", h, func(c *gin.Context) { c.String(200, "ok") })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = remote
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestInnerGuard(t *testing.T) {
	const tok = "0123456789abcdef-shared"
	cases := []struct {
		name   string
		cidrs  []string
		token  string
		remote string
		hdr    map[string]string
		want   int
	}{
		{"nothing configured", nil, "", "8.8.8.8:1", nil, 200},
		{"allowlisted peer", []string{"172.17.0.0/16"}, "", "172.17.0.5:1", nil, 200},
		{"peer outside allowlist", []string{"172.17.0.0/16"}, "", "10.0.0.1:1", nil, 403},
		{"forwarded-for is ignored", []string{"172.17.0.0/16"}, "", "10.0.0.1:1", map[string]string{"X-Forwarded-For": "172.17.0.5"}, 403},
		{"token ok", nil, tok, "8.8.8.8:1", map[string]string{innerauth.TokenHeader: tok}, 200},
		{"token missing", nil, tok, "8.8.8.8:1", nil, 401},
		{"token wrong", nil, tok, "8.8.8.8:1", map[string]string{innerauth.TokenHeader: "nope"}, 401},
		{"both required", []string{"172.17.0.0/16"}, tok, "172.17.0.5:1", nil, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := guardStatus(t, InnerGuard(tc.cidrs, tc.token), tc.remote, tc.hdr); got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMetricsGuard(t *testing.T) {
	g := MetricsGuard([]string{"10.1.0.0/16"}, "scrape-token-123456")
	cases := []struct {
		remote string
		hdr    map[string]string
		want   int
	}{
		{"127.0.0.1:5", nil, 200},
		{"[::1]:5", nil, 200},
		{"10.1.2.3:5", nil, 200},
		{"8.8.8.8:5", nil, 403},
		{"8.8.8.8:5", map[string]string{"Authorization": "Bearer scrape-token-123456"}, 200},
		{"8.8.8.8:5", map[string]string{"Authorization": "Bearer wrong"}, 403},
	}
	for _, tc := range cases {
		if got := guardStatus(t, g, tc.remote, tc.hdr); got != tc.want {
			t.Errorf("%s %v: status = %d, want %d", tc.remote, tc.hdr, got, tc.want)
		}
	}
}

func TestInnerAuthApply(t *testing.T) {
	defer innerauth.SetOutgoingToken("")
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set(innerauth.TokenHeader, "copied-from-client")
	innerauth.Apply(req)
	if req.Header.Get(innerauth.TokenHeader) != "" {
		t.Fatal("an incoming token must not be forwarded when none is configured")
	}
	innerauth.SetOutgoingToken("configured")
	innerauth.Apply(req)
	if req.Header.Get(innerauth.TokenHeader) != "configured" {
		t.Fatal("configured token must be sent")
	}
}
