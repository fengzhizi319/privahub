package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRBACTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	err = db.AutoMigrate(
		&model.SysUserPermissionRelDO{},
		&model.SysRoleResourceRelDO{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// TestRBAC_AdminGetsFullAccess verifies that the built-in admin user
// gets wildcard access even without explicit permission records.
func TestRBAC_AdminGetsFullAccess(t *testing.T) {
	db := setupRBACTestDB(t)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("username", "admin")
		c.Next()
	})
	r.Use(RBAC(db, RBACConfig{Enabled: true}))
	r.GET("/api/v1alpha1/projects", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/projects", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("admin should have access, got status %d", w.Code)
	}
}

// TestRBAC_NonAdminWithoutRolesDenied verifies that non-admin users
// without explicit permission records are denied access (least privilege).
func TestRBAC_NonAdminWithoutRolesDenied(t *testing.T) {
	db := setupRBACTestDB(t)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("username", "regular_user")
		c.Next()
	})
	r.Use(RBAC(db, RBACConfig{Enabled: true}))
	r.GET("/api/v1alpha1/projects", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/projects", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("non-admin without roles should be denied, got status %d", w.Code)
	}
}

// TestRBAC_UserWithExplicitRoleGranted verifies that users with explicit
// role-resource mappings are granted access.
func TestRBAC_UserWithExplicitRoleGranted(t *testing.T) {
	db := setupRBACTestDB(t)

	// Assign OPERATOR role to user
	db.Create(&model.SysUserPermissionRelDO{
		UserType:   "USER",
		UserKey:    "operator1",
		TargetType: "ROLE",
		TargetCode: "OPERATOR",
	})
	// Grant OPERATOR role access to the resource
	db.Create(&model.SysRoleResourceRelDO{
		RoleCode:     "OPERATOR",
		ResourceCode: "/api/v1alpha1/projects",
	})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("username", "operator1")
		c.Next()
	})
	r.Use(RBAC(db, RBACConfig{Enabled: true}))
	r.GET("/api/v1alpha1/projects", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/projects", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("user with explicit role should have access, got status %d", w.Code)
	}
}

// TestRBAC_UserWithRoleButNoResourceDenied verifies that users whose roles
// don't include the requested resource are denied.
func TestRBAC_UserWithRoleButNoResourceDenied(t *testing.T) {
	db := setupRBACTestDB(t)

	// Assign VIEWER role to user
	db.Create(&model.SysUserPermissionRelDO{
		UserType:   "USER",
		UserKey:    "viewer1",
		TargetType: "ROLE",
		TargetCode: "VIEWER",
	})
	// VIEWER only has access to /api/v1alpha1/jobs, not projects
	db.Create(&model.SysRoleResourceRelDO{
		RoleCode:     "VIEWER",
		ResourceCode: "/api/v1alpha1/jobs",
	})

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("username", "viewer1")
		c.Next()
	})
	r.Use(RBAC(db, RBACConfig{Enabled: true}))
	r.GET("/api/v1alpha1/projects", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/projects", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("user without resource permission should be denied, got status %d", w.Code)
	}
}

// TestRBAC_DisabledSkipsEnforcement verifies that RBAC disabled mode
// allows all requests through.
func TestRBAC_DisabledSkipsEnforcement(t *testing.T) {
	db := setupRBACTestDB(t)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("username", "anyone")
		c.Next()
	})
	r.Use(RBAC(db, RBACConfig{Enabled: false}))
	r.GET("/api/v1alpha1/projects", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1alpha1/projects", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RBAC disabled should allow all, got status %d", w.Code)
	}
}

// TestInvalidateUserCache verifies cache invalidation works.
func TestInvalidateUserCache(t *testing.T) {
	// Seed cache
	permCache.mu.Lock()
	permCache.entries["testuser"] = &cacheEntry{
		resources: map[string]bool{"*": true},
	}
	permCache.mu.Unlock()

	// Invalidate
	InvalidateUserCache("testuser")

	// Verify removed
	permCache.mu.RLock()
	_, exists := permCache.entries["testuser"]
	permCache.mu.RUnlock()

	if exists {
		t.Error("expected cache entry to be removed after invalidation")
	}
}
