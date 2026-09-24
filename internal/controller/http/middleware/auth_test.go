package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
)

type fakeSessions struct {
	valid map[string]*auth.UserContext
}

func (f *fakeSessions) ValidateSession(_ context.Context, token string) (*auth.UserContext, error) {
	if u, ok := f.valid[token]; ok {
		return u, nil
	}
	return nil, errors.New("no session")
}

func (f *fakeSessions) DisabledAuthUser() *auth.UserContext {
	return &auth.UserContext{Name: "admin", OwnerType: "CENTER", OwnerID: "kuscia-system"}
}

func (f *fakeSessions) BuildNodeVirtualUser(_ context.Context, nodeID string) *auth.UserContext {
	return &auth.UserContext{Name: nodeID, OwnerID: nodeID, OwnerType: "EDGE", VirtualUserForNode: auth.BoolPtr(true)}
}

func sessionRouter(jwtMgr *auth.JWTManager, s SessionValidator, enabled bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SessionAuth(jwtMgr, s, enabled))
	r.POST("/api/x", func(c *gin.Context) {
		u := CurrentUser(c)
		c.JSON(200, gin.H{"status": gin.H{"code": 0}, "data": u.Name})
	})
	return r
}

func TestSessionAuth_RequiresDBSession(t *testing.T) {
	jwtMgr := auth.NewJWTManager("s", time.Hour, time.Hour)
	pair, _ := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	sessions := &fakeSessions{valid: map[string]*auth.UserContext{}}
	r := sessionRouter(jwtMgr, sessions, true)

	do := func(hdr, val string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/x", nil)
		if hdr != "" {
			req.Header.Set(hdr, val)
		}
		r.ServeHTTP(w, req)
		return w
	}
	// Valid JWT but no DB session (e.g. after logout) -> 401.
	if w := do("User-Token", pair.AccessToken); w.Code != http.StatusUnauthorized ||
		!strings.Contains(w.Body.String(), "202011602") {
		t.Fatalf("expected 401 AUTH_FAILED, got %d %s", w.Code, w.Body.String())
	}
	sessions.valid[pair.AccessToken] = &auth.UserContext{Name: "admin", OwnerType: "CENTER"}
	if w := do("Authorization", "Bearer "+pair.AccessToken); w.Code != 200 || !strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("expected success, got %d %s", w.Code, w.Body.String())
	}
	if w := do("", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token should be 401, got %d", w.Code)
	}
	// Refresh tokens are not accepted as access tokens.
	sessions.valid[pair.RefreshToken] = &auth.UserContext{Name: "admin"}
	if w := do("User-Token", pair.RefreshToken); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token should be rejected, got %d", w.Code)
	}
	// Tampered JWT rejected even if a session row existed.
	sessions.valid[pair.AccessToken+"x"] = &auth.UserContext{Name: "admin"}
	if w := do("User-Token", pair.AccessToken+"x"); w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered token should be rejected, got %d", w.Code)
	}
}

func TestSessionAuth_Disabled(t *testing.T) {
	r := sessionRouter(nil, &fakeSessions{}, false)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/x", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("auth disabled should inject admin, got %d %s", w.Code, w.Body.String())
	}
}

func TestInnerAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(InnerAuth(&fakeSessions{}))
	r.POST("/api/v1alpha1/data/sync", func(c *gin.Context) {
		u := CurrentUser(c)
		if !u.IsVirtualNode() {
			t.Error("expected virtual node user")
		}
		c.String(200, u.OwnerID)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1alpha1/data/sync", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing kuscia-origin-source should be rejected, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	req.Header.Set(KusciaOriginSourceHeader, "alice")
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "alice" {
		t.Fatalf("expected node user alice, got %d %s", w.Code, w.Body.String())
	}
}

func TestBlockInnerPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BlockInnerPaths([]string{"/api/v1alpha1/data/sync", "/sync"}))
	r.POST("/api/v1alpha1/data/sync", func(c *gin.Context) { c.String(200, "reached") })
	r.GET("/sync", func(c *gin.Context) { c.String(200, "reached") })
	r.POST("/api/v1alpha1/data/upload", func(c *gin.Context) { c.String(200, "ok") })
	for _, tc := range []struct{ method, path string }{{"POST", "/api/v1alpha1/data/sync"}, {"GET", "/sync"}} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		if w.Code != 404 || w.Body.String() != "404" {
			t.Errorf("%s should be 404 on main port, got %d %q", tc.path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1alpha1/data/upload", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("other paths unaffected, got %d", w.Code)
	}
}

func TestTraceID_AcceptsJavaHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TraceID())
	r.GET("/t", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Trace-Id", "java-trace-1")
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Trace-ID") != "java-trace-1" || w.Header().Get("Trace-Id") != "java-trace-1" {
		t.Errorf("trace id not propagated: %v", w.Header())
	}
}

func TestBodyLimitWithOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimitWithOverrides(10, map[string]int64{"/upload": -1}))
	h := func(c *gin.Context) { c.String(200, "ok") }
	r.POST("/upload", h)
	r.POST("/other", h)
	big := strings.Repeat("x", 100)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/upload", strings.NewReader(big))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("upload route should be unlimited, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/other", strings.NewReader(big))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("other route should be limited, got %d", w.Code)
	}
}

func TestCORS_ConfigurableAndExtraHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS("https://pad.example.com"))
	r.Use(ExtraHeaders(map[string]string{"content-security-policy": "base-uri 'self'"}))
	r.GET("/t", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Origin", "https://pad.example.com")
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://pad.example.com" {
		t.Error("configured origin not allowed")
	}
	if w.Header().Get("Content-Security-Policy") != "base-uri 'self'" {
		t.Error("extra header missing")
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Trace-Id") {
		t.Error("Trace-Id should be an allowed header")
	}
}

var _ = errcode.AuthFailed
