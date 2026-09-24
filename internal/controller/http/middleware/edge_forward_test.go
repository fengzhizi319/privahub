package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/privahub/pkg/auth"
)

type edgeFakeSessions struct{}

func (edgeFakeSessions) ValidateSession(_ context.Context, token string) (*auth.UserContext, error) {
	if token == "good" {
		return &auth.UserContext{Name: "admin"}, nil
	}
	return nil, errors.New("no session")
}
func (edgeFakeSessions) DisabledAuthUser() *auth.UserContext { return &auth.UserContext{} }
func (edgeFakeSessions) BuildNodeVirtualUser(context.Context, string) *auth.UserContext {
	return &auth.UserContext{}
}

func TestEdgeForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotHost, gotOrigin, gotBody, gotToken, gotPath string
	center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotHost, gotOrigin, gotBody, gotToken, gotPath = r.Host, r.Header.Get("kuscia-origin-source"), string(b), r.Header.Get("User-Token"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":{"code":0,"msg":"from-center"}}`))
	}))
	defer center.Close()

	r := gin.New()
	r.Use(EdgeForward(EdgeForwardConfig{
		Forward:    []string{"/api/v1alpha1/node/update"},
		Include:    []string{"/api/v1alpha1/node/get", "/api/v1alpha1/node/update"},
		Gateway:    strings.TrimPrefix(center.URL, "http://"),
		CenterHost: "secretpad.master.svc",
		NodeID:     "edge1",
		Sessions:   edgeFakeSessions{},
	}))
	local := func(c *gin.Context) { c.String(200, "local") }
	r.POST("/api/v1alpha1/node/update", local)
	r.POST("/api/v1alpha1/node/get", local)
	r.POST("/api/v1alpha1/project/create", local)
	r.POST("/api/login", local)

	do := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"nodeId":"edge1"}`))
		if token != "" {
			req.Header.Set("User-Token", token)
		}
		req.Header.Set("kuscia-origin-source", "spoofed")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	w := do("/api/v1alpha1/node/update", "good")
	if w.Body.String() != `{"status":{"code":0,"msg":"from-center"}}` {
		t.Fatalf("forward resp %q", w.Body.String())
	}
	if gotHost != "secretpad.master.svc" || gotOrigin != "edge1" || gotBody != `{"nodeId":"edge1"}` || gotToken != "good" || gotPath != "/api/v1alpha1/node/update" {
		t.Fatalf("forwarded host=%q origin=%q body=%q token=%q path=%q", gotHost, gotOrigin, gotBody, gotToken, gotPath)
	}
	gotPath = ""
	if w := do("/api/v1alpha1/node/update", "bad"); !strings.Contains(w.Body.String(), "202011602") || gotPath != "" {
		t.Fatalf("unauthenticated forward must fail locally: %s", w.Body.String())
	}
	if w := do("/api/v1alpha1/node/get", ""); w.Body.String() != "local" {
		t.Fatalf("include path must be served locally: %q", w.Body.String())
	}
	if w := do("/api/v1alpha1/project/create", "good"); w.Code != http.StatusNotFound {
		t.Fatalf("non-included path must 404, got %d", w.Code)
	}
	if w := do("/api/login", ""); w.Body.String() != "local" {
		t.Fatalf("non /api/v1alpha1 path must pass: %q", w.Body.String())
	}
}
