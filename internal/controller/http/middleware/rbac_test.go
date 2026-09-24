package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRBACTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.SysUserPermissionRelDO{},
		&model.SysRoleResourceRelDO{},
		&model.NodeDO{},
		&model.ProjectNodeDO{},
		&model.ProjectDO{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

type fakePlatform struct{ edge bool }

func (f fakePlatform) IsEdge() bool { return f.edge }

func rbacRouter(db *gorm.DB, platform PlatformProvider, u *auth.UserContext, paths ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u != nil {
			SetUser(c, u)
		}
		c.Next()
	})
	r.Use(RBAC(db, platform, RBACConfig{Enabled: true}))
	for _, p := range paths {
		r.POST(p, func(c *gin.Context) { c.JSON(200, gin.H{"status": gin.H{"code": 0}}) })
	}
	return r
}

func postCode(t *testing.T, r *gin.Engine, path string) int {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", path, nil)
	r.ServeHTTP(w, req)
	var body struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Status.Code
}

func TestRBAC_AdminOnlyUserManagement(t *testing.T) {
	db := setupRBACTestDB(t)
	InvalidateUserCache("admin")
	InvalidateUserCache("bobby")
	db.Create(&model.SysUserPermissionRelDO{UserType: "USER", UserKey: "admin", TargetType: "ROLE", TargetCode: "ADMIN"})
	path := "/api/v1alpha1/user/create"

	admin := &auth.UserContext{Name: "admin", OwnerType: "CENTER", OwnerID: "kuscia-system"}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, admin, path), path); code != 0 {
		t.Errorf("admin should pass, got %d", code)
	}
	// A CENTER user without ADMIN role is denied on admin-only routes.
	other := &auth.UserContext{Name: "bobby", OwnerType: "CENTER", OwnerID: "kuscia-system"}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, other, path), path); code != errcode.AuthFailed.Code {
		t.Errorf("non-admin should be denied, got %d", code)
	}
	// Virtual node users are never admins.
	node := &auth.UserContext{Name: "admin", OwnerType: "EDGE", OwnerID: "admin", VirtualUserForNode: auth.BoolPtr(true)}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, node, path), path); code != errcode.AuthFailed.Code {
		t.Errorf("virtual node should be denied, got %d", code)
	}
}

func TestRBAC_InterfaceResource(t *testing.T) {
	db := setupRBACTestDB(t)
	path := "/api/v1alpha1/project/create" // PRJ_CREATE
	free := "/api/v1alpha1/message/list"   // no @ApiResource in Java

	edgeUser := &auth.UserContext{Name: "eu", OwnerType: "EDGE", OwnerID: "alice", PlatformNodeID: "kuscia-system"}
	r := rbacRouter(db, fakePlatform{}, edgeUser, path, free)
	if code := postCode(t, r, path); code != errcode.AuthFailed.Code {
		t.Errorf("edge user without PRJ_CREATE should be denied, got %d", code)
	}
	if code := postCode(t, r, free); code != 0 {
		t.Errorf("unannotated route should pass, got %d", code)
	}

	edgeUser.APIResources = []string{"PRJ_CREATE"}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, edgeUser, path), path); code != 0 {
		t.Errorf("edge user with PRJ_CREATE should pass, got %d", code)
	}
	all := &auth.UserContext{Name: "x", OwnerType: "EDGE", OwnerID: "bob", APIResources: []string{auth.AllInterfaceResource}}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, all, path), path); code != 0 {
		t.Errorf("ALL_INTERFACE_RESOURCE should pass, got %d", code)
	}
	// Platform manager (ownerId == platformNodeId) passes.
	mgr := &auth.UserContext{Name: "admin", OwnerType: "EDGE", OwnerID: "alice", PlatformNodeID: "alice"}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, mgr, path), path); code != 0 {
		t.Errorf("platform manager should pass, got %d", code)
	}
	// Edge platform skips interface checks.
	noRes := &auth.UserContext{Name: "eu2", OwnerType: "EDGE", OwnerID: "alice"}
	if code := postCode(t, rbacRouter(db, fakePlatform{edge: true}, noRes, path), path); code != 0 {
		t.Errorf("edge platform should skip checks, got %d", code)
	}
	// CENTER users skip.
	center := &auth.UserContext{Name: "c", OwnerType: "CENTER", OwnerID: "kuscia-system"}
	if code := postCode(t, rbacRouter(db, fakePlatform{}, center, path), path); code != 0 {
		t.Errorf("center user should pass, got %d", code)
	}
}

func TestRBAC_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RBAC(nil, nil, RBACConfig{Enabled: false}))
	r.POST("/api/v1alpha1/user/create", func(c *gin.Context) { c.JSON(200, gin.H{"status": gin.H{"code": 0}}) })
	if code := postCode(t, r, "/api/v1alpha1/user/create"); code != 0 {
		t.Errorf("disabled RBAC should pass, got %d", code)
	}
}

func TestRequireDataResource(t *testing.T) {
	db := setupRBACTestDB(t)
	db.Create(&model.ProjectNodeDO{ProjectID: "p1", NodeID: "alice"})
	SetDataResourceAuth(service.NewDataResourceAuth(db, service.NewEnvService("CENTER", "kuscia-system", "", nil)))
	defer SetDataResourceAuth(nil)

	gin.SetMode(gin.TestMode)
	mk := func(u *auth.UserContext, kind service.DataResourceType, id string) int {
		r := gin.New()
		r.POST("/x", func(c *gin.Context) {
			SetUser(c, u)
			if !RequireDataResource(c, kind, id) {
				return
			}
			c.JSON(200, gin.H{"status": gin.H{"code": 0}})
		})
		return postCode(t, r, "/x")
	}
	edge := &auth.UserContext{Name: "eu", OwnerType: "EDGE", OwnerID: "alice"}
	if mk(edge, service.DataResourceNodeID, "alice") != 0 || mk(edge, service.DataResourceProjectID, "p1") != 0 {
		t.Error("own node/project should pass")
	}
	if mk(edge, service.DataResourceNodeID, "bob") != errcode.AuthFailed.Code {
		t.Error("foreign node should be denied")
	}
	if mk(edge, service.DataResourceProjectID, "p2") != errcode.AuthFailed.Code {
		t.Error("foreign project should be denied")
	}
}

func TestAPIResourceCodes_CoverJavaRoutes(t *testing.T) {
	for path, code := range map[string]string{
		"/api/logout":                             "AUTH_LOGOUT",
		"/api/v1alpha1/user/get":                  "USER_GET",
		"/api/v1alpha1/user/updatePwd":            "USER_UPDATE_PWD",
		"/api/v1alpha1/user/node/resetPassword":   "NODE_USER_RESET_PWD",
		"/api/v1alpha1/user/remote/resetPassword": "REMOTE_USER_RESET_PWD",
		"/api/v1alpha1/graph/start":               "GRAPH_START",
	} {
		if APIResourceCodes[path] != code {
			t.Errorf("%s -> %q, want %q", path, APIResourceCodes[path], code)
		}
	}
}
