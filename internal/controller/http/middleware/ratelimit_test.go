package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
)

func TestKeyedLimiter_GuavaLikeSpacing(t *testing.T) {
	l := NewKeyedLimiter(5, time.Minute, 1)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	if !l.Allow("u") {
		t.Fatal("first request must pass")
	}
	if l.Allow("u") {
		t.Fatal("second immediate request must be throttled")
	}
	if !l.Allow("other") {
		t.Fatal("keys are independent")
	}
	now = now.Add(11 * time.Second)
	if l.Allow("u") {
		t.Fatal("5/60s means one permit per 12s")
	}
	now = now.Add(1 * time.Second)
	if !l.Allow("u") {
		t.Fatal("permit must be available after 12s")
	}
	// Idle buckets are evicted after an hour.
	now = now.Add(2 * time.Hour)
	l.Allow("x")
	if _, ok := l.buckets["u"]; ok {
		t.Fatal("idle bucket should be evicted")
	}
}

func TestKeyedLimiter_DisabledAndBurst(t *testing.T) {
	var nilL *KeyedLimiter
	if !nilL.Allow("a") || NewKeyedLimiter(0, time.Minute, 1) != nil {
		t.Fatal("non-positive permits disable the limiter")
	}
	l := NewKeyedLimiter(5, time.Minute, 3)
	for i := 0; i < 3; i++ {
		if !l.Allow("a") {
			t.Fatalf("burst request %d should pass", i)
		}
	}
	if l.Allow("a") {
		t.Fatal("burst exhausted")
	}
}

func TestUserRateLimit_GuardedPathsSharePerUserBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewKeyedLimiter(5, time.Minute, 1)
	r := gin.New()
	user := "alice"
	r.Use(func(c *gin.Context) {
		SetUser(c, &auth.UserContext{Name: user})
		c.Next()
	})
	r.Use(UserRateLimit(l, []string{"/a/create", "/b/create"}))
	ok := func(c *gin.Context) { c.JSON(200, gin.H{"status": gin.H{"code": 0}}) }
	r.POST("/a/create", ok)
	r.POST("/b/create", ok)
	r.POST("/list", ok)

	if code := postCode(t, r, "/a/create"); code != 0 {
		t.Fatalf("first create: %d", code)
	}
	if code := postCode(t, r, "/b/create"); code != errcode.SystemRequestFrequencyError.Code {
		t.Fatalf("second create (other path, same user) should be throttled, got %d", code)
	}
	if code := postCode(t, r, "/list"); code != 0 {
		t.Fatalf("unguarded path must not be throttled, got %d", code)
	}
	user = "bob"
	if code := postCode(t, r, "/a/create"); code != 0 {
		t.Fatalf("other user must have its own bucket, got %d", code)
	}
}

func TestLoginRateLimit_PerIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/login", LoginRateLimit(NewKeyedLimiter(20, time.Minute, 1)), func(c *gin.Context) {
		c.JSON(200, gin.H{"status": gin.H{"code": 0}})
	})
	send := func(ip string) int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/login", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)
		var body struct {
			Status struct {
				Code int `json:"code"`
			} `json:"status"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return body.Status.Code
	}
	a, b, c := send("10.0.0.1"), send("10.0.0.1"), send("10.0.0.2")
	if a != 0 || b != errcode.SystemRequestFrequencyError.Code || c != 0 {
		t.Fatalf("expected second login from same IP throttled only: %d %d %d", a, b, c)
	}
}
