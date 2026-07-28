package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"gorm.io/gorm"
)

// RBACConfig holds configuration for the RBAC middleware.
type RBACConfig struct {
	Enabled       bool          // Whether RBAC enforcement is active
	CacheDuration time.Duration // How long to cache permission lookups
}

// permissionCache stores cached user permission sets.
type permissionCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	resources map[string]bool
	expiresAt time.Time
}

var permCache = &permissionCache{
	entries: make(map[string]*cacheEntry),
	ttl:     60 * time.Second,
}

// RBAC creates a middleware that enforces role-based access control.
// It checks whether the authenticated user has permission to access the requested API resource.
func RBAC(db *gorm.DB, cfg RBACConfig) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	if cfg.CacheDuration > 0 {
		permCache.ttl = cfg.CacheDuration
	}

	return func(c *gin.Context) {
		username, exists := c.Get("username")
		if !exists {
			c.Next() // No auth context — skip RBAC
			return
		}

		userKey := username.(string)
		resourceCode := c.FullPath()
		if resourceCode == "" {
			c.Next()
			return
		}

		// Check permission
		if !hasPermission(db, userKey, resourceCode) {
			c.JSON(http.StatusForbidden, gin.H{
				"status": gin.H{"code": 202011503, "msg": "permission denied"},
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// hasPermission checks if a user has access to a specific resource code.
func hasPermission(db *gorm.DB, userKey, resourceCode string) bool {
	// Check cache first
	permCache.mu.RLock()
	if entry, ok := permCache.entries[userKey]; ok && time.Now().Before(entry.expiresAt) {
		allowed := entry.resources[resourceCode] || entry.resources["*"]
		permCache.mu.RUnlock()
		return allowed
	}
	permCache.mu.RUnlock()

	// Load permissions from DB
	resources := loadUserResources(db, userKey)

	// Update cache
	permCache.mu.Lock()
	permCache.entries[userKey] = &cacheEntry{
		resources: resources,
		expiresAt: time.Now().Add(permCache.ttl),
	}
	permCache.mu.Unlock()

	return resources[resourceCode] || resources["*"]
}

// loadUserResources loads all resource codes accessible by a user through their roles.
func loadUserResources(db *gorm.DB, userKey string) map[string]bool {
	result := make(map[string]bool)

	// Find user's roles
	var permissions []model.SysUserPermissionRelDO
	if err := db.Where("user_key = ? AND target_type = ?", userKey, "ROLE").
		Find(&permissions).Error; err != nil {
		return result
	}

	if len(permissions) == 0 {
		// No explicit permissions — grant all (admin default)
		result["*"] = true
		return result
	}

	// Collect role codes
	roleCodes := make([]string, 0, len(permissions))
	for _, p := range permissions {
		roleCodes = append(roleCodes, p.TargetCode)
	}

	// Find resources for those roles
	var roleResources []model.SysRoleResourceRelDO
	if err := db.Where("role_code IN ?", roleCodes).
		Find(&roleResources).Error; err != nil {
		return result
	}

	for _, rr := range roleResources {
		result[rr.ResourceCode] = true
	}

	return result
}

// InvalidateUserCache removes a user's cached permissions (call after role changes).
func InvalidateUserCache(userKey string) {
	permCache.mu.Lock()
	delete(permCache.entries, userKey)
	permCache.mu.Unlock()
}
