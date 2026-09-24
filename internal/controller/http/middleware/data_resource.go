package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/gin-gonic/gin"
)

// DataResourceRule is one Java @DataResource(field, resourceType) annotation:
// the request-body field holding the resource id and the resource kind.
type DataResourceRule struct {
	Kind  service.DataResourceType
	Field string
}

func projectRule() DataResourceRule {
	return DataResourceRule{Kind: service.DataResourceProjectID, Field: "projectId"}
}

func nodeRule(field string) DataResourceRule {
	return DataResourceRule{Kind: service.DataResourceNodeID, Field: field}
}

// DataResourceRoutes maps route paths to the Java @DataResource annotation of
// the corresponding controller method (plus Go alias / Go-only routes of the
// same resources). DataResourceGuard enforces them for every request, so a
// handler cannot forget the check.
var DataResourceRoutes = map[string]DataResourceRule{
	// --- ProjectController (15) ---
	"/api/v1alpha1/project/get":                projectRule(),
	"/api/v1alpha1/project/update":             projectRule(),
	"/api/v1alpha1/project/delete":             projectRule(),
	"/api/v1alpha1/project/inst/add":           projectRule(),
	"/api/v1alpha1/project/node/add":           projectRule(),
	"/api/v1alpha1/project/datatable/add":      projectRule(),
	"/api/v1alpha1/project/datatable/delete":   projectRule(),
	"/api/v1alpha1/project/datatable/get":      projectRule(),
	"/api/v1alpha1/project/job/list":           projectRule(),
	"/api/v1alpha1/project/job/get":            projectRule(),
	"/api/v1alpha1/project/job/stop":           projectRule(),
	"/api/v1alpha1/project/job/task/logs":      projectRule(),
	"/api/v1alpha1/project/job/task/output":    projectRule(),
	"/api/v1alpha1/project/update/tableConfig": projectRule(),
	"/api/v1alpha1/project/datasource/list":    projectRule(),
	// --- GraphController (13) ---
	"/api/v1alpha1/graph/create":         projectRule(),
	"/api/v1alpha1/graph/delete":         projectRule(),
	"/api/v1alpha1/graph/list":           projectRule(),
	"/api/v1alpha1/graph/meta/update":    projectRule(),
	"/api/v1alpha1/graph/update":         projectRule(),
	"/api/v1alpha1/graph/node/update":    projectRule(),
	"/api/v1alpha1/graph/start":          projectRule(),
	"/api/v1alpha1/graph/node/status":    projectRule(),
	"/api/v1alpha1/graph/stop":           projectRule(),
	"/api/v1alpha1/graph/detail":         projectRule(),
	"/api/v1alpha1/graph/node/output":    projectRule(),
	"/api/v1alpha1/graph/node/logs":      projectRule(),
	"/api/v1alpha1/graph/node/max_index": projectRule(),
	// --- ScheduledController (4) ---
	"/api/v1alpha1/scheduled/id":                 projectRule(),
	"/api/v1alpha1/scheduled/graph/once/success": projectRule(),
	"/api/v1alpha1/scheduled/graph/create":       projectRule(),
	"/api/v1alpha1/scheduled/job/list":           projectRule(),
	// --- ModelExportController (3) / ModelManagementController (4) ---
	"/api/v1alpha1/model/pack":           projectRule(),
	"/api/v1alpha1/model/status":         projectRule(),
	"/api/v1alpha1/model/modelPartyPath": projectRule(),
	"/api/v1alpha1/model/page":           projectRule(),
	"/api/v1alpha1/model/detail":         projectRule(),
	"/api/v1alpha1/model/info":           projectRule(),
	"/api/v1alpha1/model/serving/create": projectRule(),
	// --- CloudLog / FeatureDatasource / P2PProject ---
	"/api/v1alpha1/cloud_log/sls":                projectRule(),
	"/api/v1alpha1/feature_datasource/auth/list": projectRule(),
	"/api/v1alpha1/p2p/project/update":           projectRule(),
	// --- NodeController (8) ---
	"/api/v1alpha1/node/update":        nodeRule("nodeId"),
	"/api/v1alpha1/node/get":           nodeRule("nodeId"),
	"/api/v1alpha1/node/delete":        nodeRule("nodeId"),
	"/api/v1alpha1/node/token":         nodeRule("nodeId"),
	"/api/v1alpha1/node/newToken":      nodeRule("nodeId"),
	"/api/v1alpha1/node/refresh":       nodeRule("nodeId"),
	"/api/v1alpha1/node/result/list":   nodeRule("ownerId"),
	"/api/v1alpha1/node/result/detail": nodeRule("nodeId"),
	// --- NodeRouteController (1) ---
	"/api/v1alpha1/nodeRoute/page": nodeRule("ownerId"),
	// --- DataSourceController (5) ---
	"/api/v1alpha1/datasource/create": nodeRule("ownerId"),
	"/api/v1alpha1/datasource/delete": nodeRule("ownerId"),
	"/api/v1alpha1/datasource/list":   nodeRule("ownerId"),
	"/api/v1alpha1/datasource/detail": nodeRule("ownerId"),
	"/api/v1alpha1/datasource/nodes":  nodeRule("ownerId"),
	// --- DatatableController (4) ---
	"/api/v1alpha1/datatable/create": nodeRule("ownerId"),
	"/api/v1alpha1/datatable/list":   nodeRule("ownerId"),
	"/api/v1alpha1/datatable/get":    nodeRule("nodeId"),
	"/api/v1alpha1/datatable/delete": nodeRule("nodeId"),
	// --- DataController (2) / FeatureDatasourceController (1) ---
	"/api/v1alpha1/data/create":               nodeRule("nodeId"),
	"/api/v1alpha1/data/download":             nodeRule("nodeId"),
	"/api/v1alpha1/feature_datasource/create": nodeRule("ownerId"),
	// --- InstController (2): Java annotates instId as NODE_ID ---
	"/api/v1alpha1/inst/get":         nodeRule("instId"),
	"/api/v1alpha1/inst/node/delete": nodeRule("nodeId"),
	// --- Message (4) / Approval (1) ---
	"/api/v1alpha1/message/reply":   nodeRule("voteParticipantId"),
	"/api/v1alpha1/message/list":    nodeRule("ownerId"),
	"/api/v1alpha1/message/detail":  nodeRule("ownerId"),
	"/api/v1alpha1/message/pending": nodeRule("ownerId"),
	"/api/v1alpha1/approval/create": nodeRule("initiatorId"),
	// --- NodeUser / RemoteUser / P2PNode ---
	"/api/v1alpha1/user/node/resetPassword":   nodeRule("nodeId"),
	"/api/v1alpha1/user/remote/resetPassword": nodeRule("nodeId"),
	"/api/v1alpha1/p2p/node/delete":           nodeRule("nodeId"),

	// --- Go alias routes of the same Java endpoints ---
	"/api/v1alpha1/project/job/detail":   projectRule(),
	"/api/v1alpha1/project/job/task/log": projectRule(),
	"/api/v1alpha1/datatable/detail":     nodeRule("nodeId"),
	// --- Go-only routes operating on a project ---
	"/api/v1alpha1/project/job/create":        projectRule(),
	"/api/v1alpha1/graphDatasource/bind":      projectRule(),
	"/api/v1alpha1/graphDatasource/unbind":    projectRule(),
	"/api/v1alpha1/graphDatasource/list":      projectRule(),
	"/api/v1alpha1/featureTable/project/list": projectRule(),
}

// DefaultAutonomyWhitelist is Java application-p2p.yaml white-list.paths:
// AUTONOMY (P2P) users skip the data-resource check on these routes (their
// services scope the result by institution / project participation).
var DefaultAutonomyWhitelist = []string{
	"/api/v1alpha1/p2p/node/delete",
	"/api/v1alpha1/p2p/project/update",
	"/api/v1alpha1/graph/list",
	"/api/v1alpha1/project/get",
	"/api/v1alpha1/project/job/list",
	"/api/v1alpha1/project/job/get",
	"/api/v1alpha1/project/datatable/get",
	"/api/v1alpha1/project/job/task/output",
	"/api/v1alpha1/project/job/task/logs",
	"/api/v1alpha1/data/create",
	"/api/v1alpha1/datatable/list",
	"/api/v1alpha1/graph/node/status",
	"/api/v1alpha1/graph/detail",
	"/api/v1alpha1/graph/node/output",
	"/api/v1alpha1/graph/node/logs",
	"/api/v1alpha1/inst/get",
	"/api/v1alpha1/inst/node/list",
	"/api/v1alpha1/inst/node/add",
	"/api/v1alpha1/inst/node/token",
	"/api/v1alpha1/inst/node/newToken",
	"/api/v1alpha1/inst/node/delete",
	"/api/v1alpha1/inst/node/register",
}

// DataResourceGuard enforces DataResourceRoutes (Java DataResourceAspect):
//   - EDGE platform: only NODE_ID rules are checked (DataResourceAuth.Check);
//   - AUTONOMY users skip whitelisted paths (Java white-list.paths);
//   - CENTER owner users are never checked;
//   - otherwise the id is read from the JSON body field (camelCase or
//     snake_case) and checked; a missing id is denied like Java (null id).
//
// Denials answer HTTP 200 with AUTH_FAILED. Install after authentication.
func DataResourceGuard(autonomyWhitelist []string) gin.HandlerFunc {
	white := make(map[string]bool, len(autonomyWhitelist))
	for _, p := range autonomyWhitelist {
		white[p] = true
	}
	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		rule, ok := DataResourceRoutes[path]
		if !ok {
			c.Next()
			return
		}
		u := CurrentUser(c)
		if u == nil || u.IsCenterOwner() {
			c.Next()
			return
		}
		if u.PlatformType == auth.PlatformTypeAutonomy && white[path] {
			c.Next()
			return
		}
		if !RequireDataResource(c, rule.Kind, bodyField(c, rule.Field)) {
			return
		}
		c.Next()
	}
}

// bodyField reads a top-level string field from the JSON request body without
// consuming it (the body is restored for the handler). Both the camelCase
// name and its snake_case twin are accepted.
func bodyField(c *gin.Context, field string) string {
	if c.Request == nil || c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(trimmed, &m) != nil {
		return ""
	}
	for _, key := range []string{field, toSnakeCase(field)} {
		raw, ok := m[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
