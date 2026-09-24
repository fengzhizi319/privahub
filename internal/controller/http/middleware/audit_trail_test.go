package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
)

type captureRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *captureRecorder) Record(_ context.Context, e audit.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func withCapture(t *testing.T) *captureRecorder {
	rec := &captureRecorder{}
	prev := audit.GetRecorder()
	audit.SetRecorder(rec)
	t.Cleanup(func() { audit.SetRecorder(prev) })
	return rec
}

func TestAuditTrail_RecordsWriteWithResourceAndRedaction(t *testing.T) {
	rec := withCapture(t)
	r := gin.New()
	r.Use(AuditTrail(nil))
	var gotBody string
	r.POST("/api/v1alpha1/graph/create", func(c *gin.Context) {
		SetUser(c, &auth.UserContext{Name: "alice", OwnerID: "n1"})
		b, _ := io.ReadAll(c.Request.Body)
		gotBody = string(b)
		c.JSON(200, gin.H{"status": gin.H{"code": 0, "msg": "ok"}, "data": gin.H{"code": 5}})
	})
	body := `{"projectId":"p1","graphId":"g1","password":"s3cret","token":"tkn"}`
	req := httptest.NewRequest("POST", "/api/v1alpha1/graph/create?token=abc&x=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if gotBody != body {
		t.Fatalf("handler body altered: %q", gotBody)
	}
	if len(rec.events) != 1 {
		t.Fatalf("events %d", len(rec.events))
	}
	e := rec.events[0]
	if e.Action != "POST /api/v1alpha1/graph/create" || e.Actor != "alice" || e.OwnerID != "n1" ||
		e.ResourceType != "graph" || e.ResourceID != "g1" || e.Result != audit.ResultSuccess {
		t.Fatalf("unexpected event %+v", e)
	}
	if e.Extra["projectId"] != "p1" || e.Extra["query"] != "token=%2A%2A%2A&x=1" {
		t.Fatalf("extra %+v", e.Extra)
	}
	for k, v := range e.Extra {
		if strings.Contains(v, "s3cret") || strings.Contains(v, "tkn") || strings.Contains(v, "abc") || k == "password" {
			t.Fatalf("secret leaked in %s=%s", k, v)
		}
	}
}

func TestAuditTrail_SkipsReadsAndResults(t *testing.T) {
	rec := withCapture(t)
	r := gin.New()
	r.Use(AuditTrail(nil))
	r.POST("/api/v1alpha1/project/list", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	r.POST("/api/v1alpha1/project/delete", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": gin.H{"code": errcode.AuthFailed.Code, "msg": "no"}})
	})
	r.POST("/api/v1alpha1/node/create", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": gin.H{"code": 202011100, "msg": "bad"}})
	})
	r.POST("/api/v1alpha1/node/update", func(c *gin.Context) { c.AbortWithStatus(401) })
	r.GET("/api/v1alpha1/audit/verify", func(c *gin.Context) { c.JSON(200, gin.H{"status": gin.H{"code": 0}}) })
	r.POST("/api/login", func(c *gin.Context) { c.JSON(200, gin.H{"status": gin.H{"code": 0}}) })

	do := func(method, path, body string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
	do("POST", "/api/v1alpha1/project/list", `{}`)
	do("POST", "/api/v1alpha1/nope", `{}`) // unmatched 404
	do("POST", "/api/v1alpha1/project/delete", `{"projectId":"p9"}`)
	do("POST", "/api/v1alpha1/node/create", `{}`)
	do("POST", "/api/v1alpha1/node/update", `{"nodeId":"n2"}`)
	do("GET", "/api/v1alpha1/audit/verify", ``)
	do("POST", "/api/login", `{"name":"bob","passwordHash":"xyz"}`)

	want := []struct{ action, result string }{
		{"POST /api/v1alpha1/project/delete", audit.ResultDenied},
		{"POST /api/v1alpha1/node/create", audit.ResultFailure},
		{"POST /api/v1alpha1/node/update", audit.ResultDenied},
		{"GET /api/v1alpha1/audit/verify", audit.ResultSuccess},
		{"POST /api/login", audit.ResultSuccess},
	}
	if len(rec.events) != len(want) {
		t.Fatalf("events: %+v", rec.events)
	}
	for i, w := range want {
		if rec.events[i].Action != w.action || rec.events[i].Result != w.result {
			t.Errorf("event %d = %s/%s, want %s/%s", i, rec.events[i].Action, rec.events[i].Result, w.action, w.result)
		}
	}
	login := rec.events[4]
	if login.ActorType != audit.ActorAnonymous || login.Extra["claimedUser"] != "bob" {
		t.Fatalf("login event %+v", login)
	}
	for _, v := range login.Extra {
		if strings.Contains(v, "xyz") {
			t.Fatal("password hash leaked")
		}
	}
	if rec.events[0].ResourceID != "p9" || rec.events[2].ResourceID != "n2" {
		t.Fatalf("resource ids: %+v / %+v", rec.events[0], rec.events[2])
	}
}

func TestAuditTrail_ContextResourceWins(t *testing.T) {
	rec := withCapture(t)
	r := gin.New()
	r.Use(AuditTrail(nil))
	r.POST("/api/v1alpha1/project/update", func(c *gin.Context) {
		SetAuditResource(c, "project", "verified-p")
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("POST", "/api/v1alpha1/project/update", strings.NewReader(`{"projectId":"claimed"}`))
	r.ServeHTTP(httptest.NewRecorder(), req)
	if len(rec.events) != 1 || rec.events[0].ResourceID != "verified-p" {
		t.Fatalf("%+v", rec.events)
	}
}
