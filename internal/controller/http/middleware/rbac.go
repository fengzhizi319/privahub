package middleware

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/internal/dao/model"
	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// APIResourceCodes maps route paths to Java ApiResourceCodeConstants codes
// (extracted from the @ApiResource annotations of the Java controllers, plus
// the Go-only alias routes). Routes without a code are not interface-checked,
// like Java controller methods without @ApiResource.
var APIResourceCodes = map[string]string{
	// --- Java @ApiResource annotations ---
	"/api/logout":                                "AUTH_LOGOUT",
	"/api/v1alpha1/cloud_log/sls":                "GRAPH_NODE_LOGS",
	"/api/v1alpha1/component/batch":              "GRAPH_COMM_BATH",
	"/api/v1alpha1/component/i18n":               "GRAPH_COMM_I18N",
	"/api/v1alpha1/component/list":               "GRAPH_COMM_LIST",
	"/api/v1alpha1/data/create":                  "DATA_CREATE",
	"/api/v1alpha1/data/download":                "DATA_DOWNLOAD",
	"/api/v1alpha1/data/upload":                  "DATA_UPLOAD",
	"/api/v1alpha1/datatable/create":             "DATATABLE_CREATE",
	"/api/v1alpha1/datatable/delete":             "DATATABLE_DELETE",
	"/api/v1alpha1/datatable/get":                "DATATABLE_GET",
	"/api/v1alpha1/datatable/list":               "DATATABLE_LIST",
	"/api/v1alpha1/graph/create":                 "GRAPH_CREATE",
	"/api/v1alpha1/graph/delete":                 "GRAPH_DELETE",
	"/api/v1alpha1/graph/detail":                 "GRAPH_DETAIL",
	"/api/v1alpha1/graph/list":                   "GRAPH_LIST",
	"/api/v1alpha1/graph/meta/update":            "GRAPH_META_UPDATE",
	"/api/v1alpha1/graph/node/logs":              "GRAPH_NODE_LOGS",
	"/api/v1alpha1/graph/node/max_index":         "GRAPH_NODE_UPDATE",
	"/api/v1alpha1/graph/node/output":            "GRAPH_NODE_OUTPUT",
	"/api/v1alpha1/graph/node/status":            "GRAPH_NODE_STATUS",
	"/api/v1alpha1/graph/node/update":            "GRAPH_NODE_UPDATE",
	"/api/v1alpha1/graph/start":                  "GRAPH_START",
	"/api/v1alpha1/graph/stop":                   "GRAPH_STOP",
	"/api/v1alpha1/graph/update":                 "GRAPH_UPDATE",
	"/api/v1alpha1/inst/get":                     "INST_GET",
	"/api/v1alpha1/inst/node/add":                "NODE_CREATE",
	"/api/v1alpha1/inst/node/delete":             "NODE_DELETE",
	"/api/v1alpha1/inst/node/list":               "NODE_LIST",
	"/api/v1alpha1/inst/node/newToken":           "NODE_NEW_TOKEN",
	"/api/v1alpha1/inst/node/register":           "DATA_UPLOAD",
	"/api/v1alpha1/inst/node/token":              "NODE_TOKEN",
	"/api/v1alpha1/node/create":                  "NODE_CREATE",
	"/api/v1alpha1/node/delete":                  "NODE_DELETE",
	"/api/v1alpha1/node/get":                     "NODE_GET",
	"/api/v1alpha1/node/list":                    "NODE_LIST",
	"/api/v1alpha1/node/newToken":                "NODE_NEW_TOKEN",
	"/api/v1alpha1/node/page":                    "NODE_PAGE",
	"/api/v1alpha1/node/refresh":                 "NODE_REFRESH",
	"/api/v1alpha1/node/result/detail":           "NODE_RESULT_DETAIL",
	"/api/v1alpha1/node/result/list":             "NODE_RESULT_LIST",
	"/api/v1alpha1/node/token":                   "NODE_TOKEN",
	"/api/v1alpha1/node/update":                  "NODE_UPDATE",
	"/api/v1alpha1/nodeRoute/delete":             "NODE_ROUTE_DELETE",
	"/api/v1alpha1/nodeRoute/get":                "NODE_ROUTE_GET",
	"/api/v1alpha1/nodeRoute/listNode":           "NODE_ROUTE_LIST_NODE",
	"/api/v1alpha1/nodeRoute/page":               "NODE_ROUTE_PAGE",
	"/api/v1alpha1/nodeRoute/refresh":            "NODE_ROUTE_REFRESH",
	"/api/v1alpha1/nodeRoute/update":             "NODE_ROUTE_UPDATE",
	"/api/v1alpha1/p2p/node/create":              "NODE_CREATE",
	"/api/v1alpha1/p2p/node/delete":              "NODE_DELETE",
	"/api/v1alpha1/p2p/project/archive":          "PRJ_ARCHIVE",
	"/api/v1alpha1/p2p/project/create":           "PRJ_CREATE",
	"/api/v1alpha1/p2p/project/list":             "PRJ_LIST",
	"/api/v1alpha1/p2p/project/participants":     "PRJ_PARTICIPANTS",
	"/api/v1alpha1/p2p/project/update":           "PRJ_UPDATE",
	"/api/v1alpha1/project/create":               "PRJ_CREATE",
	"/api/v1alpha1/project/datasource/list":      "PRJ_GET",
	"/api/v1alpha1/project/datatable/add":        "PRJ_ADD_TABLE",
	"/api/v1alpha1/project/datatable/delete":     "PRJ_DATATABLE_DELETE",
	"/api/v1alpha1/project/datatable/get":        "PRJ_DATATABLE_GET",
	"/api/v1alpha1/project/delete":               "PRJ_DELETE",
	"/api/v1alpha1/project/get":                  "PRJ_GET",
	"/api/v1alpha1/project/inst/add":             "PRJ_ADD_INST",
	"/api/v1alpha1/project/job/get":              "PRJ_JOB_GET",
	"/api/v1alpha1/project/job/list":             "PRJ_JOB_LIST",
	"/api/v1alpha1/project/job/stop":             "PRJ_JOB_STOP",
	"/api/v1alpha1/project/job/task/logs":        "PRJ_TASK_LOGS",
	"/api/v1alpha1/project/job/task/output":      "PRJ_TASK_OUTPUT",
	"/api/v1alpha1/project/list":                 "PRJ_LIST",
	"/api/v1alpha1/project/node/add":             "PRJ_ADD_NODE",
	"/api/v1alpha1/project/update":               "PRJ_UPDATE",
	"/api/v1alpha1/project/update/tableConfig":   "PRJ_UPDATE",
	"/api/v1alpha1/scheduled/del":                "SCHEDULED_OFFLINE",
	"/api/v1alpha1/scheduled/graph/create":       "SCHEDULED_CREATE",
	"/api/v1alpha1/scheduled/graph/once/success": "SCHEDULED_CREATE",
	"/api/v1alpha1/scheduled/id":                 "SCHEDULED_ID",
	"/api/v1alpha1/scheduled/info":               "SCHEDULED_PAGE",
	"/api/v1alpha1/scheduled/job/list":           "SCHEDULED_PAGE",
	"/api/v1alpha1/scheduled/offline":            "SCHEDULED_OFFLINE",
	"/api/v1alpha1/scheduled/page":               "SCHEDULED_PAGE",
	"/api/v1alpha1/scheduled/task/info":          "SCHEDULED_PAGE",
	"/api/v1alpha1/scheduled/task/page":          "SCHEDULED_PAGE",
	"/api/v1alpha1/scheduled/task/rerun":         "SCHEDULED_PAGE",
	"/api/v1alpha1/scheduled/task/stop":          "SCHEDULED_PAGE",
	"/api/v1alpha1/user/get":                     "USER_GET",
	"/api/v1alpha1/user/node/resetPassword":      "NODE_USER_RESET_PWD",
	"/api/v1alpha1/user/remote/resetPassword":    "REMOTE_USER_RESET_PWD",
	"/api/v1alpha1/user/updatePwd":               "USER_UPDATE_PWD",
	"/api/v1alpha1/version/list":                 "COMPONENT_VERSION_LIST",
	// --- Go alias routes of the same Java endpoints ---
	"/api/v1alpha1/user/logout":          "AUTH_LOGOUT",
	"/api/v1alpha1/project/job/detail":   "PRJ_JOB_GET",
	"/api/v1alpha1/project/job/task/log": "PRJ_TASK_LOGS",
	"/api/v1alpha1/project/archive":      "PRJ_ARCHIVE",
	"/api/v1alpha1/datatable/detail":     "DATATABLE_GET",
	"/api/v1alpha1/node/route/create":    "NODE_ROUTE_CREATE",
	"/api/v1alpha1/node/route/list":      "NODE_ROUTE_PAGE",
	"/api/v1alpha1/node/route/delete":    "NODE_ROUTE_DELETE",
	"/api/v1alpha1/component/version":    "COMPONENT_VERSION_LIST",
	"/api/v1alpha1/user/current":         "USER_GET",
	// --- Go-only routes mapped to the closest Java resource ---
	"/api/v1alpha1/project/job/create":        "GRAPH_START",
	"/api/v1alpha1/datatable/register":        "DATATABLE_CREATE",
	"/api/v1alpha1/datatable/fed/create":      "DATATABLE_CREATE",
	"/api/v1alpha1/datatable/grant":           "DATATABLE_CREATE",
	"/api/v1alpha1/graphDatasource/bind":      "GRAPH_UPDATE",
	"/api/v1alpha1/graphDatasource/unbind":    "GRAPH_UPDATE",
	"/api/v1alpha1/graphDatasource/list":      "GRAPH_DETAIL",
	"/api/v1alpha1/featureTable/project/list": "PRJ_DATATABLE_GET",
}

// NoInterfaceCheckPaths are authenticated routes that Java deliberately left
// without @ApiResource (their controllers only carry @DataResource checks, or
// are node-to-node endpoints). They are listed explicitly so that the
// route-coverage test can tell "intentionally unchecked" from "forgotten".
var NoInterfaceCheckPaths = map[string]string{
	// ModelManagementController / ModelExportController: @DataResource(projectId) only.
	"/api/v1alpha1/model/pack":           "java: @DataResource only",
	"/api/v1alpha1/model/status":         "java: @DataResource only",
	"/api/v1alpha1/model/modelPartyPath": "java: @DataResource only",
	"/api/v1alpha1/model/page":           "java: @DataResource only",
	"/api/v1alpha1/model/detail":         "java: @DataResource only",
	"/api/v1alpha1/model/info":           "java: @DataResource only",
	"/api/v1alpha1/model/discard":        "java: no annotation",
	"/api/v1alpha1/model/delete":         "java: no annotation",
	"/api/v1alpha1/model/serving/create": "java: @DataResource only",
	"/api/v1alpha1/model/serving/detail": "java: no annotation",
	"/api/v1alpha1/model/serving/delete": "java: no annotation",
	// DataSourceController: @DataResource(ownerId) only.
	"/api/v1alpha1/datasource/create": "java: @DataResource only",
	"/api/v1alpha1/datasource/delete": "java: @DataResource only",
	"/api/v1alpha1/datasource/list":   "java: @DataResource only",
	"/api/v1alpha1/datasource/detail": "java: @DataResource only",
	"/api/v1alpha1/datasource/nodes":  "java: @DataResource only",
	"/api/v1alpha1/datasource/test":   "go-only connectivity check, same scope as datasource/create",
	// MessageController / ApprovalController: @DataResource(NODE_ID) only.
	"/api/v1alpha1/message/list":                 "java: @DataResource only",
	"/api/v1alpha1/message/detail":               "java: @DataResource only",
	"/api/v1alpha1/message/reply":                "java: @DataResource only",
	"/api/v1alpha1/message/pending":              "java: @DataResource only",
	"/api/v1alpha1/approval/create":              "java: @DataResource only",
	"/api/v1alpha1/approval/pull/status":         "java: no annotation",
	"/api/v1alpha1/datatable/pushToTee":          "java: no annotation",
	"/api/v1alpha1/project/tee/list":             "java: no annotation",
	"/api/v1alpha1/project/getOutTable":          "java: no annotation",
	"/api/v1alpha1/feature_datasource/create":    "java: @DataResource only",
	"/api/v1alpha1/feature_datasource/auth/list": "java: @DataResource only",
	// Node-to-node endpoints (inner port, node virtual user; Java VoteSyncController / DataSyncController).
	"/api/v1alpha1/vote_sync/create": "java: node rpc, no annotation",
	"/api/v1alpha1/data/sync":        "java: node rpc, no annotation",
	// OpenAPI document (gated by server.api_docs config).
	"/api/v1alpha1/docs": "api docs",
}

// PublicPaths are /api routes served without authentication (Java
// LoginConfiguration excludes).
var PublicPaths = map[string]bool{
	"/api/login":                       true,
	"/api/user/refresh":                true,
	"/api/v1alpha1/user/login":         true,
	"/api/v1alpha1/inst/node/register": true, // authenticated by the inst token
	"/api/v1alpha1/healthz":            true,
	"/api/v1alpha1/env":                true,
}

// AdminOnlyPaths are Go-only user management routes restricted to users with
// the ADMIN (or CENTER_ADMIN) role.
var AdminOnlyPaths = map[string]bool{
	"/api/v1alpha1/user/create":            true,
	"/api/v1alpha1/user/list":              true,
	"/api/v1alpha1/user/update":            true,
	"/api/v1alpha1/user/delete":            true,
	"/api/v1alpha1/user/reset-password":    true,
	"/api/v1alpha1/nodeUser/create":        true,
	"/api/v1alpha1/nodeUser/resetPassword": true,
	"/api/v1alpha1/nodeUser/list":          true,
	"/api/v1alpha1/inst/create":            true,
	"/api/v1alpha1/inst/list":              true,
	"/api/v1alpha1/edgeDataSync/log":       true,
}

// PlatformProvider exposes the platform facts needed by RBAC (service.EnvService).
type PlatformProvider interface {
	IsEdge() bool
}

// RBACConfig holds configuration for the RBAC middleware.
type RBACConfig struct {
	Enabled       bool          // Whether RBAC enforcement is active
	CacheDuration time.Duration // Deprecated: privileged checks deliberately bypass TTL caches.
}

// InvalidateUserCache is retained for handler compatibility. Privileged role
// checks now read the authoritative DB on every request: process-local TTLs
// delayed revocation and could share grants between databases with the same
// username. No positive grant survives a failed query or a role deletion.
func InvalidateUserCache(_ string) {}

func loadRoles(ctx context.Context, db *gorm.DB, userKey string) map[string]bool {
	roles := map[string]bool{}
	if db == nil || userKey == "" {
		return roles
	}
	var rels []model.SysUserPermissionRelDO
	if err := db.WithContext(ctx).Where("user_key = ? AND target_type = ? AND user_type IN ?", userKey, "ROLE", []string{auth.PermissionUserTypeUser, auth.PermissionUserTypeEdgeUser}).Find(&rels).Error; err != nil {
		return roles
	}
	for _, rel := range rels {
		roles[rel.TargetCode] = true
	}
	return roles
}

// IsAdmin reports whether the (non-virtual) user holds the ADMIN or
// CENTER_ADMIN role.
func IsAdmin(ctx context.Context, db *gorm.DB, u *auth.UserContext) bool {
	if u == nil || u.IsVirtualNode() {
		return false
	}
	roles := loadRoles(ctx, db, u.Name)
	return roles[auth.RoleAdmin] || roles[auth.RoleCenterAdmin]
}

// CheckInterfaceResource implements Java InterfaceResourceAspect +
// DefaultApiResourceAuth for a resource code.
func CheckInterfaceResource(ctx context.Context, db *gorm.DB, platform PlatformProvider, u *auth.UserContext, code string) bool {
	if code == "" {
		return true
	}
	if u == nil {
		return false
	}
	if platform != nil && platform.IsEdge() {
		return true // legacy edge-platform interface policy, authenticated only
	}
	if u.IsCenterOwner() {
		return true // ignore check for center user
	}
	// Current platform manager.
	if u.OwnerID != "" && u.OwnerID == u.PlatformNodeID {
		return true
	}
	if u.OwnerType == auth.OwnerTypeP2P && nodeInInst(ctx, db, u.PlatformNodeID, u.OwnerID) {
		return true
	}
	return u.ContainInterfaceResource(auth.AllInterfaceResource) || u.ContainInterfaceResource(code)
}

func nodeInInst(ctx context.Context, db *gorm.DB, nodeID, instID string) bool {
	if db == nil || nodeID == "" || instID == "" || !db.Migrator().HasColumn(&model.NodeDO{}, "inst_id") {
		return false
	}
	var n int64
	db.WithContext(ctx).Model(&model.NodeDO{}).Where("node_id = ? AND inst_id = ?", nodeID, instID).Count(&n)
	return n > 0
}

// RBAC enforces interface permissions on authenticated routes:
//   - AdminOnlyPaths require the ADMIN role;
//   - other routes with a Java resource code are checked like Java's
//     InterfaceResourceAspect (edge platform / CENTER users skip; others need
//     the code in their session apiResources).
//
// Denials answer HTTP 200 with AUTH_FAILED (Java behaviour).
func RBAC(db *gorm.DB, platform PlatformProvider, cfg RBACConfig) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.Next() // no auth context (public route)
			return
		}
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if AuditorPaths[path] {
			// Security audit trail: AUDITOR / ADMIN only (audit_trail.go).
			if !IsAuditor(c.Request.Context(), db, u) {
				denyInterface(c, path, u)
				return
			}
			c.Next()
			return
		}
		if AdminOnlyPaths[path] {
			if !IsAdmin(c.Request.Context(), db, u) {
				denyInterface(c, path, u)
				return
			}
			c.Next()
			return
		}
		code, mapped := APIResourceCodes[path]
		if !mapped && strings.HasPrefix(path, "/api/") && !RouteClassified(path) {
			// Default deny: an authenticated route nobody classified.
			denyInterface(c, "UNCLASSIFIED:"+path, u)
			return
		}
		if !CheckInterfaceResource(c.Request.Context(), db, platform, u, code) {
			denyInterface(c, code, u)
			return
		}
		c.Next()
	}
}

// RouteClassified reports whether path is known to the interface RBAC tables
// (resource code, admin-only, explicitly unchecked or public).
func RouteClassified(path string) bool {
	if _, ok := APIResourceCodes[path]; ok {
		return true
	}
	if _, ok := NoInterfaceCheckPaths[path]; ok {
		return true
	}
	return AdminOnlyPaths[path] || PublicPaths[path] || AuditorPaths[path]
}

func denyInterface(c *gin.Context, code string, u *auth.UserContext) {
	msg := fmt.Sprintf("No permission to access the interface(%s). owner_type(%s), owner_id(%s)", code, u.OwnerType, u.OwnerID)
	recordDenial(c, u, "interface", code, msg)
	c.AbortWithStatusJSON(200, response.Body{Status: &response.Status{
		Code: errcode.AuthFailed.Code, Msg: response.Message(c, errcode.AuthFailed, msg),
	}})
}

// ---------------------------------------------------------------------------
// Data resource (Java DataResourceAspect)
// ---------------------------------------------------------------------------

var (
	dataAuthMu sync.RWMutex
	dataAuth   *service.DataResourceAuth
)

// SetDataResourceAuth installs the global DataResource checker (done in wire).
func SetDataResourceAuth(a *service.DataResourceAuth) {
	dataAuthMu.Lock()
	dataAuth = a
	dataAuthMu.Unlock()
}

// CanAccessData reports whether the current user may access the resource.
// Without a configured checker only CENTER users are allowed.
func CanAccessData(c *gin.Context, kind service.DataResourceType, id string) bool {
	u := CurrentUser(c)
	dataAuthMu.RLock()
	a := dataAuth
	dataAuthMu.RUnlock()
	if a == nil {
		return u != nil && (u.IsCenterOwner() || (kind == service.DataResourceNodeID && u.OwnerID == id))
	}
	return a.Check(c.Request.Context(), u, kind, id)
}

// RequireDataResource checks access and, when denied, writes the Java
// AUTH_FAILED response (HTTP 200) and returns false. Handlers should return
// immediately when it returns false:
//
//	if !middleware.RequireDataResource(c, service.DataResourceNodeID, req.NodeID) { return }
func RequireDataResource(c *gin.Context, kind service.DataResourceType, id string) bool {
	if CanAccessData(c, kind, id) {
		return true
	}
	u := CurrentUser(c)
	ownerType, ownerID := "", ""
	if u != nil {
		ownerType, ownerID = u.OwnerType, u.OwnerID
	}
	msg := fmt.Sprintf("No permission to access the data %s(%s). owner_type(%s), owner_id(%s)", kind, id, ownerType, ownerID)
	recordDenial(c, u, string(kind), id, msg)
	c.AbortWithStatusJSON(200, response.Body{Status: &response.Status{
		Code: errcode.AuthFailed.Code, Msg: response.Message(c, errcode.AuthFailed, msg),
	}})
	return false
}

// RequireNodeAccess is RequireDataResource for NODE_ID.
func RequireNodeAccess(c *gin.Context, nodeID string) bool {
	return RequireDataResource(c, service.DataResourceNodeID, nodeID)
}

// RequireProjectAccess is RequireDataResource for PROJECT_ID.
func RequireProjectAccess(c *gin.Context, projectID string) bool {
	return RequireDataResource(c, service.DataResourceProjectID, projectID)
}

// recordDenial writes an authorization denial to the security audit trail.
func recordDenial(c *gin.Context, u *auth.UserContext, resourceType, resourceID, reason string) {
	e := audit.Event{
		IP:           c.ClientIP(),
		Action:       audit.ActionAuthzDenied,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       audit.ResultDenied,
		Reason:       reason,
		Extra:        map[string]string{"method": c.Request.Method, "path": c.Request.URL.Path},
	}
	if u != nil {
		e.Actor, e.OwnerID = u.Name, u.OwnerID
		e.ActorType = audit.ActorUser
		if u.IsVirtualNode() {
			e.ActorType = audit.ActorNode
		}
	}
	audit.Record(c.Request.Context(), e)
}
