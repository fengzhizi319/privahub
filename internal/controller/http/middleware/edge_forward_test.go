package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/privahub/pkg/auth"
)

type edgeFakeSessions struct{}

func (edgeFakeSessions) ValidateSession(_ context.Context, token string) (*auth.UserContext, error) {
	switch token {
	case "good":
		return &auth.UserContext{Name: "admin"}, nil
	case "mustchange":
		return &auth.UserContext{Name: "admin", MustChangePassword: true}, nil
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
	if gotHost != "secretpad.master.svc" || gotOrigin != "edge1" || gotBody != `{"nodeId":"edge1"}` || gotToken != "" || gotPath != "/api/v1alpha1/node/update" {
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

// TestEdgeForward_StripsCredentialsAndCapsResponse: the user's credentials
// never reach the center, oversized center responses and transport errors
// answer a generic message.
func TestEdgeForward_StripsCredentialsAndCapsResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got http.Header
	big := false
	center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		if big {
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
			return
		}
		_, _ = w.Write([]byte(`{"status":{"code":0}}`))
	}))
	defer center.Close()
	mk := func(gw string) *gin.Engine {
		r := gin.New()
		r.Use(EdgeForward(EdgeForwardConfig{
			Forward: []string{"/api/v1alpha1/node/update"}, Gateway: gw, NodeID: "edge1",
			Sessions: edgeFakeSessions{}, MaxResponseBytes: 1024, MaxRequestBytes: 1024,
		}))
		return r
	}
	send := func(r *gin.Engine, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/node/update", strings.NewReader(body))
		req.Header.Set("User-Token", "good")
		req.Header.Set("Authorization", "Bearer good")
		req.Header.Set("Cookie", "session=good")
		req.Header.Set("X-CSRF-Token", "csrf")
		req.Header.Set("Token", "good")
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		req.Header.Set("Connection", "X-Custom-Hop")
		req.Header.Set("X-Custom-Hop", "v")
		req.Header.Set("X-Keep", "v")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	r := mk(center.URL)
	if w := send(r, `{}`); !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatalf("forward failed: %s", w.Body.String())
	}
	for _, h := range []string{"User-Token", "Authorization", "Cookie", "X-Csrf-Token", "Token", "X-Forwarded-For", "X-Custom-Hop"} {
		if got.Get(h) != "" {
			t.Errorf("header %s forwarded to the center", h)
		}
	}
	if got.Get("X-Keep") != "v" || got.Get("kuscia-origin-source") != "edge1" {
		t.Errorf("expected headers missing: %v", got)
	}
	big = true
	if w := send(r, `{}`); strings.Contains(w.Body.String(), "xxxx") || !strings.Contains(w.Body.String(), "forwarding to the center failed") {
		t.Errorf("oversized response must be refused: %s", w.Body.String())
	}
	if w := send(r, strings.Repeat("y", 2048)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized request must be refused: %d", w.Code)
	}
	// Transport errors do not leak addresses / error strings.
	dead := mk("http://127.0.0.1:1")
	if w := send(dead, `{}`); strings.Contains(w.Body.String(), "127.0.0.1") || strings.Contains(w.Body.String(), "refused") {
		t.Errorf("transport error leaked: %s", w.Body.String())
	}
}

// TestEdgeForward_CookieSessionRequiresCSRF: a forwarded write authenticated
// by the ambient session cookie must pass the same Origin + CSRF double-submit
// check as SessionAuth, otherwise a cross-site form could drive center writes.
func TestEdgeForward_CookieSessionRequiresCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := GetBrowserSessionConfig()
	defer SetBrowserSessionConfig(prev)
	SetBrowserSessionConfig(BrowserSessionConfig{AllowedOrigins: []string{"https://console.example"}})

	forwarded := 0
	center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded++
		_, _ = w.Write([]byte(`{"status":{"code":0}}`))
	}))
	defer center.Close()

	r := gin.New()
	r.Use(EdgeForward(EdgeForwardConfig{
		Forward:  []string{"/api/v1alpha1/node/update"},
		Include:  []string{"/api/v1alpha1/node/update"},
		Gateway:  strings.TrimPrefix(center.URL, "http://"),
		NodeID:   "edge1",
		Sessions: edgeFakeSessions{},
	}))

	sessName, _, csrfName := cookieNames(false)
	do := func(origin, csrfHeader string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/node/update", strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: sessName, Value: "good"})
		req.AddCookie(&http.Cookie{Name: csrfName, Value: "csrf-1"})
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if csrfHeader != "" {
			req.Header.Set(CSRFHeader, csrfHeader)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := do("https://console.example", ""); w.Code != http.StatusForbidden || forwarded != 0 {
		t.Fatalf("missing CSRF header must be rejected locally: code=%d forwarded=%d", w.Code, forwarded)
	}
	if w := do("https://evil.example", "csrf-1"); w.Code != http.StatusForbidden || forwarded != 0 {
		t.Fatalf("foreign Origin must be rejected locally: code=%d forwarded=%d", w.Code, forwarded)
	}
	if w := do("https://console.example", "csrf-1"); w.Code != http.StatusOK || forwarded != 1 {
		t.Fatalf("valid cookie+CSRF must forward: code=%d forwarded=%d body=%s", w.Code, forwarded, w.Body.String())
	}
}

// TestEdgeForward_SameRulesAsSessionAuth: forwarded writes are held to the
// SessionAuth rules (authenticateSession): must_change_password sessions and
// invalid / refresh JWTs never reach the center.
func TestEdgeForward_SameRulesAsSessionAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	forwarded := 0
	center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded++
		_, _ = w.Write([]byte(`{"status":{"code":0}}`))
	}))
	defer center.Close()
	jm := auth.NewJWTManager("edge-forward-test-secret-0123456789", time.Hour, time.Hour)
	r := gin.New()
	r.Use(EdgeForward(EdgeForwardConfig{
		Forward: []string{"/api/v1alpha1/node/update"}, Gateway: center.URL, NodeID: "edge1",
		Sessions: edgeFakeSessions{}, JWT: jm,
	}))
	send := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/node/update", strings.NewReader(`{}`))
		req.Header.Set("User-Token", token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := send("mustchange"); w.Code != http.StatusForbidden || forwarded != 0 {
		t.Fatalf("must_change_password session must not forward: code=%d forwarded=%d body=%s", w.Code, forwarded, w.Body.String())
	}
	if w := send("a.b.c"); w.Code != http.StatusUnauthorized || forwarded != 0 {
		t.Fatalf("invalid JWT must not forward: code=%d forwarded=%d", w.Code, forwarded)
	}
	pair, err := jm.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	if err != nil {
		t.Fatal(err)
	}
	if w := send(pair.RefreshToken); w.Code != http.StatusUnauthorized || forwarded != 0 {
		t.Fatalf("refresh token must not authenticate a forward: code=%d forwarded=%d", w.Code, forwarded)
	}
	if w := send("good"); w.Code != http.StatusOK || forwarded != 1 {
		t.Fatalf("valid session must forward: code=%d forwarded=%d", w.Code, forwarded)
	}
}
