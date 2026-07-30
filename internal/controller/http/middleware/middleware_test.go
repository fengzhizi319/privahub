package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAuditLog_WriteOperation(t *testing.T) {
	log, _ := zap.NewDevelopment()
	r := gin.New()
	r.Use(AuditLog(log))
	r.POST("/api/v1alpha1/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1alpha1/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuditLog_ReadOperationSkipped(t *testing.T) {
	log, _ := zap.NewDevelopment()
	r := gin.New()
	r.Use(AuditLog(log))
	r.GET("/api/v1alpha1/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestIPWhitelist_Allowed(t *testing.T) {
	r := gin.New()
	r.Use(IPWhitelist("127.0.0.0/8", "10.0.0.0/8"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 for allowed IP, got %d", w.Code)
	}
}

func TestIPWhitelist_Denied(t *testing.T) {
	r := gin.New()
	r.Use(IPWhitelist("10.0.0.0/8"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 for denied IP, got %d", w.Code)
	}
}

func TestIPWhitelist_EmptyAllowsAll(t *testing.T) {
	r := gin.New()
	r.Use(IPWhitelist())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 when whitelist empty, got %d", w.Code)
	}
}

func TestIPWhitelist_SingleIP(t *testing.T) {
	r := gin.New()
	r.Use(IPWhitelist("192.168.1.100"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Allowed
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 for exact IP match, got %d", w.Code)
	}

	// Denied
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.101:12345"
	r.ServeHTTP(w2, req2)
	if w2.Code != 403 {
		t.Fatalf("expected 403 for non-matching IP, got %d", w2.Code)
	}
}

func TestRateLimiter_AllowsBurst(t *testing.T) {
	r := gin.New()
	r.Use(RateLimiter(5, 1)) // 5 burst, 1/s refill
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// First 5 requests should succeed (burst)
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}

	// 6th request should be rate limited
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("expected 429 after burst exhausted, got %d", w.Code)
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	r := gin.New()
	r.Use(RateLimiter(1, 0.001)) // 1 burst, very slow refill
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// IP1 first request OK
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	r.ServeHTTP(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("IP1 first request: expected 200, got %d", w1.Code)
	}

	// IP2 first request also OK (independent bucket)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "2.2.2.2:1234"
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("IP2 first request: expected 200, got %d", w2.Code)
	}

	// IP1 second request should be limited
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "1.1.1.1:1234"
	r.ServeHTTP(w3, req3)
	if w3.Code != 429 {
		t.Fatalf("IP1 second request: expected 429, got %d", w3.Code)
	}
}

func TestBodyLimit_RejectsLargeBody(t *testing.T) {
	r := gin.New()
	r.Use(BodyLimit(10)) // 10 bytes max
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Request with Content-Length > limit
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", strings.NewReader("this is way more than 10 bytes"))
	req.ContentLength = 100
	r.ServeHTTP(w, req)
	if w.Code != 413 {
		t.Fatalf("expected 413 for oversized body, got %d", w.Code)
	}
}

func TestBodyLimit_AllowsSmallBody(t *testing.T) {
	r := gin.New()
	r.Use(BodyLimit(1024))
	r.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", strings.NewReader("small"))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 for small body, got %d", w.Code)
	}
}

// --- CORS Tests ---

func TestCORS_AllowedOrigin(t *testing.T) {
	r := gin.New()
	r.Use(CORS("http://localhost:8000", "http://example.com"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:8000")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8000" {
		t.Fatalf("expected ACAO http://localhost:8000, got %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected ACAC true, got %q", got)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	r := gin.New()
	r.Use(CORS("http://localhost:8000"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Should NOT set Access-Control-Allow-Origin for disallowed origin
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected empty ACAO for disallowed origin, got %q", got)
	}
}

func TestCORS_PreflightReturns204(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.OPTIONS("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:8000")
	r.ServeHTTP(w, req)

	if w.Code != 204 {
		t.Fatalf("expected 204 for OPTIONS preflight, got %d", w.Code)
	}
}

func TestCORS_AllowsUserTokenHeader(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:8000")
	r.ServeHTTP(w, req)

	allowedHeaders := w.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowedHeaders, "User-Token") {
		t.Fatalf("expected Access-Control-Allow-Headers to contain User-Token, got %q", allowedHeaders)
	}
	if !strings.Contains(allowedHeaders, "token") {
		t.Fatalf("expected Access-Control-Allow-Headers to contain token, got %q", allowedHeaders)
	}
}

// --- SecurityHeaders Tests ---

func TestSecurityHeaders(t *testing.T) {
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Cache-Control", "no-store"},
	}
	for _, tt := range tests {
		if got := w.Header().Get(tt.header); got != tt.expected {
			t.Errorf("header %s: expected %q, got %q", tt.header, tt.expected, got)
		}
	}
}

// --- TraceID Tests ---

func TestTraceID_GeneratesWhenMissing(t *testing.T) {
	r := gin.New()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	traceID := w.Header().Get("X-Trace-ID")
	if traceID == "" {
		t.Fatal("expected X-Trace-ID header to be set")
	}
	if len(traceID) != 36 { // UUID format
		t.Fatalf("expected UUID-length trace ID, got %q (len=%d)", traceID, len(traceID))
	}
}

func TestTraceID_PreservesIncoming(t *testing.T) {
	r := gin.New()
	r.Use(TraceID())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Trace-ID", "my-custom-trace-123")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Trace-ID"); got != "my-custom-trace-123" {
		t.Fatalf("expected preserved trace ID, got %q", got)
	}
}

// --- JWTAuth Tests ---

func TestJWTAuth_MissingToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	r := gin.New()
	r.Use(JWTAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401 for missing token, got %d", w.Code)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	r := gin.New()
	r.Use(JWTAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	pair, err := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	r := gin.New()
	r.Use(JWTAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		username, _ := c.Get("username")
		c.JSON(200, gin.H{"username": username})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 for valid token, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("expected response to contain username admin, got %q", w.Body.String())
	}
}

func TestJWTAuth_RefreshTokenRejected(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	pair, err := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	r := gin.New()
	r.Use(JWTAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Using refresh token for API access should be rejected
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401 for refresh token used as access, got %d", w.Code)
	}
}

func TestJWTAuth_UserTokenHeader(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	pair, err := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	r := gin.New()
	r.Use(JWTAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Token via User-Token header
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Token", pair.AccessToken)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 for User-Token header auth, got %d", w.Code)
	}
}

// --- Recovery Tests ---

func TestRecovery_HandlesPanic(t *testing.T) {
	log, _ := zap.NewDevelopment()
	r := gin.New()
	r.Use(Recovery(log))
	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 (graceful recovery), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "system unknown error") {
		t.Fatalf("expected error message in body, got %q", w.Body.String())
	}
}

// --- OptionalAuth Tests ---

func TestOptionalAuth_NoToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	r := gin.New()
	r.Use(OptionalAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		_, exists := c.Get("username")
		c.JSON(200, gin.H{"authenticated": exists})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "false") {
		t.Fatalf("expected authenticated=false, got %q", w.Body.String())
	}
}

func TestOptionalAuth_ValidToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager("test-secret", 2*time.Hour, 7*24*time.Hour)
	pair, err := jwtMgr.GenerateTokenPair("1", "admin", "CENTER", "kuscia-system")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	r := gin.New()
	r.Use(OptionalAuth(jwtMgr))
	r.GET("/test", func(c *gin.Context) {
		username, _ := c.Get("username")
		c.JSON(200, gin.H{"username": username})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("expected username admin, got %q", w.Body.String())
	}
}

func TestMetrics_RecordsRequestMetrics(t *testing.T) {
	r := gin.New()
	r.Use(Metrics())
	r.GET("/api/v1alpha1/projects", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/projects", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMetrics_UnmatchedRoute(t *testing.T) {
	r := gin.New()
	r.Use(Metrics())
	// No route registered - will hit __unmatched__ path

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/nonexistent/path", nil)
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
