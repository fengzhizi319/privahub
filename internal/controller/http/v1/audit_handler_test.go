package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/controller/http/middleware"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuditAPI_Permissions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "a.db")), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AuditLogDO{}, &model.AuditChainTailDO{}, &model.AuditSegmentDO{},
		&model.SysUserPermissionRelDO{}); err != nil {
		t.Fatal(err)
	}
	for user, role := range map[string]string{"aud-admin": auth.RoleAdmin, "aud-auditor": auth.RoleAuditor, "aud-plain": "EDGE_USER"} {
		db.Create(&model.SysUserPermissionRelDO{UserType: "USER", UserKey: user, TargetType: "ROLE", TargetCode: role})
	}
	svc, err := audit.New(db, nil, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Start()
	t.Cleanup(func() { _ = svc.Close(context.Background()) })
	for i := 0; i < 3; i++ {
		svc.Record(context.Background(), audit.Event{Actor: "x", Action: "auth.login", Result: "success"})
	}
	if err := svc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	h := NewAuditHandler(svc, db, "", "")
	build := func(rbac bool) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			if name := c.GetHeader("X-User"); name != "" {
				// CENTER owner: would pass every Java interface check.
				middleware.SetUser(c, &auth.UserContext{Name: name, OwnerType: auth.OwnerTypeCenter})
			}
			c.Next()
		})
		if rbac {
			r.Use(middleware.RBAC(db, nil, middleware.RBACConfig{Enabled: true}))
		}
		r.POST("/api/v1alpha1/audit/list", h.List)
		r.GET("/api/v1alpha1/audit/verify", h.Verify)
		return r
	}
	type resp struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
		Data json.RawMessage `json:"data"`
	}
	call := func(r *gin.Engine, user, method, path string) resp {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"size":2}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User", user)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out resp
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	for _, rbac := range []bool{true, false} { // handler re-checks even without RBAC
		r := build(rbac)
		for _, tc := range []struct {
			user string
			ok   bool
		}{{"aud-admin", true}, {"aud-auditor", true}, {"aud-plain", false}, {"", false}} {
			for _, ep := range [][2]string{{"POST", "/api/v1alpha1/audit/list"}, {"GET", "/api/v1alpha1/audit/verify"}} {
				out := call(r, tc.user, ep[0], ep[1])
				if (out.Status.Code == 0) != tc.ok {
					t.Errorf("rbac=%v user=%q %s: code %d", rbac, tc.user, ep[1], out.Status.Code)
				}
				if !tc.ok && out.Status.Code != errcode.AuthFailed.Code {
					t.Errorf("expected AUTH_FAILED, got %d", out.Status.Code)
				}
			}
		}
	}
	out := call(build(true), "aud-auditor", "POST", "/api/v1alpha1/audit/list")
	var page AuditListResponse
	if err := json.Unmarshal(out.Data, &page); err != nil || page.Total != 3 || len(page.List) != 2 {
		t.Fatalf("list %s %v", out.Data, err)
	}
	out = call(build(true), "aud-auditor", "GET", "/api/v1alpha1/audit/verify")
	var vr audit.VerifyResult
	if err := json.Unmarshal(out.Data, &vr); err != nil || !vr.Valid || vr.Checked != 3 {
		t.Fatalf("verify %s %v", out.Data, err)
	}
}
