package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
