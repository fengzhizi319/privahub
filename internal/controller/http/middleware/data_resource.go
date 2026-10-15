package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/fengzhizi319/privahub/internal/service"
	"github.com/gin-gonic/gin"
)

// DataResourceRule is one Java @DataResource(field, resourceType) annotation:
// the request-body field holding the resource id and the resource kind.
//
// Field is matched through CanonicalJSONKey, i.e. exactly the keys a handler
// binding `json:"projectId"` or `json:"project_id"` can receive. Extra lists
// further ids of the same request (Go-only routes carrying several ids); an
// Optional extra is only checked when the body carries it.
type DataResourceRule struct {
	Kind     service.DataResourceType
	Field    string
	Optional bool
	Extra    []DataResourceRule
}

func (r DataResourceRule) with(extra ...DataResourceRule) DataResourceRule {
	r.Extra = append(append([]DataResourceRule{}, r.Extra...), extra...)
	return r
}

func optional(r DataResourceRule) DataResourceRule {
	r.Optional = true
	return r
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
	// --- Go-only routes carrying node / project ids (default-deny audit) ---
	"/api/v1alpha1/datatable/register":   projectRule().with(nodeRule("nodeId")),
	"/api/v1alpha1/datatable/grant":      nodeRule("nodeId").with(optional(projectRule())),
	"/api/v1alpha1/datatable/fed/create": projectRule(),
	"/api/v1alpha1/datatable/pushToTee":  nodeRule("nodeId"),
	"/api/v1alpha1/node/route/create":    nodeRule("srcNodeId"),
	"/api/v1alpha1/node/route/list":      nodeRule("nodeId"),
	"/api/v1alpha1/node/route/delete":    nodeRule("srcNodeId"),
}

// DataResourceExempt lists every authenticated /api route that is NOT in
// DataResourceRoutes, with the reason it needs no request-id data check.
// VerifyRouteCoverage refuses to start the server when a registered route is
// in neither map, so a new route carrying project / node / owner ids cannot
// silently skip the check (default deny).
var DataResourceExempt = map[string]string{
	// Public / session routes.
	"/api/login":                       "public login",
	"/api/logout":                      "own session only",
	"/api/user/refresh":                "public token refresh",
	"/api/v1alpha1/user/login":         "public login",
	"/api/v1alpha1/user/logout":        "own session only",
	"/api/v1alpha1/healthz":            "public health check",
	"/api/v1alpha1/env":                "public platform info",
	"/api/v1alpha1/docs":               "static OpenAPI document",
	"/api/v1alpha1/inst/node/register": "public; authenticated by the stored inst token of the node",
	// Current-user routes: scoped by the session identity, no id in the body.
	"/api/v1alpha1/user/current":   "caller's own user",
	"/api/v1alpha1/user/get":       "caller's own user",
	"/api/v1alpha1/user/updatePwd": "caller's own password",
	// Admin-only user / institution management (AdminOnlyPaths).
	"/api/v1alpha1/user/create":            "admin only",
	"/api/v1alpha1/user/list":              "admin only",
	"/api/v1alpha1/user/update":            "admin only",
	"/api/v1alpha1/user/delete":            "admin only",
	"/api/v1alpha1/user/reset-password":    "admin only",
	"/api/v1alpha1/nodeUser/create":        "admin only",
	"/api/v1alpha1/nodeUser/resetPassword": "admin only",
	"/api/v1alpha1/nodeUser/list":          "admin only",
	"/api/v1alpha1/inst/create":            "admin only",
	"/api/v1alpha1/inst/list":              "admin only",
	"/api/v1alpha1/edgeDataSync/log":       "admin only",
	"/api/v1alpha1/audit/list":             "auditor / admin only (AuditorPaths)",
	"/api/v1alpha1/audit/verify":           "auditor / admin only (AuditorPaths)",
	// Listing / creation scoped by the caller in the service layer.
	"/api/v1alpha1/project/create":           "java: no @DataResource; creator becomes the owner",
	"/api/v1alpha1/project/list":             "java: no @DataResource; service lists the caller's projects",
	"/api/v1alpha1/project/archive":          "java: no @DataResource (P2P archive vote)",
	"/api/v1alpha1/project/tee/list":         "java: no annotation",
	"/api/v1alpha1/project/getOutTable":      "java: no annotation",
	"/api/v1alpha1/node/create":              "java: no @DataResource (CENTER node management)",
	"/api/v1alpha1/node/list":                "java: no @DataResource; service scopes EDGE users to cooperating nodes",
	"/api/v1alpha1/node/page":                "java: no @DataResource",
	"/api/v1alpha1/datasource/test":          "connectivity check of the submitted config; no stored resource",
	"/api/v1alpha1/model/discard":            "java: no annotation (modelId)",
	"/api/v1alpha1/model/delete":             "java: no annotation (modelId)",
	"/api/v1alpha1/model/serving/detail":     "java: no annotation (servingId)",
	"/api/v1alpha1/model/serving/delete":     "java: no annotation (servingId)",
	"/api/v1alpha1/inst/node/list":           "service scopes to the caller's institution (ownerId)",
	"/api/v1alpha1/inst/node/add":            "service scopes to the caller's institution (ownerId)",
	"/api/v1alpha1/inst/node/token":          "service scopes to the caller's institution (findInstNode)",
	"/api/v1alpha1/inst/node/newToken":       "service scopes to the caller's institution (findInstNode)",
	"/api/v1alpha1/scheduled/page":           "java: no @DataResource",
	"/api/v1alpha1/scheduled/offline":        "java: no @DataResource",
	"/api/v1alpha1/scheduled/del":            "java: no @DataResource",
	"/api/v1alpha1/scheduled/info":           "java: no @DataResource",
	"/api/v1alpha1/scheduled/task/page":      "java: no @DataResource",
	"/api/v1alpha1/scheduled/task/stop":      "java: no @DataResource",
	"/api/v1alpha1/scheduled/task/rerun":     "java: no @DataResource",
	"/api/v1alpha1/scheduled/task/info":      "java: no @DataResource",
	"/api/v1alpha1/component/list":           "static component catalogue",
	"/api/v1alpha1/component/version":        "static component catalogue",
	"/api/v1alpha1/component/i18n":           "static component catalogue",
	"/api/v1alpha1/component/batch":          "static component catalogue",
	"/api/v1alpha1/version/list":             "static version info",
	"/api/v1alpha1/p2p/project/create":       "java: no @DataResource; creator institution becomes the owner",
	"/api/v1alpha1/p2p/project/list":         "java: no @DataResource; service lists the caller's projects",
	"/api/v1alpha1/p2p/project/archive":      "java: no @DataResource (archive vote)",
	"/api/v1alpha1/p2p/project/participants": "java: no @DataResource",
	"/api/v1alpha1/p2p/node/create":          "java: no @DataResource; node added to the caller's institution",
	"/api/v1alpha1/data/upload":              "multipart Node-Id checked in the handler (RequireNodeAccess)",
	"/api/v1alpha1/nodeRoute/get":            "java: no @DataResource (routeId)",
	"/api/v1alpha1/nodeRoute/update":         "java: no @DataResource (routeId)",
	"/api/v1alpha1/nodeRoute/listNode":       "java: no @DataResource",
	"/api/v1alpha1/nodeRoute/refresh":        "java: no @DataResource (routeId)",
	"/api/v1alpha1/nodeRoute/delete":         "java: no @DataResource (routeId)",
	"/api/v1alpha1/approval/pull/status":     "java: no annotation (voteId)",
	// Node-to-node (inner port, node virtual user).
	"/api/v1alpha1/vote_sync/create": "node rpc (Java VoteSyncController, no annotation); inner port only",
	"/api/v1alpha1/data/sync":        "node rpc; per-table ownership enforced by the P2P sync consumer",
}

// DefaultAutonomyWhitelist is deprecated and intentionally empty. A route
// allowlist must not bypass resource ownership checks for AUTONOMY callers.
// Keep the symbol for compatibility with integrations building this package.
var DefaultAutonomyWhitelist = []string{}

// CtxVerifiedResourceIDs is the gin context key holding the ids
// DataResourceGuard verified (map canonical field name -> id). Handlers may
// read them through VerifiedResourceID.
const CtxVerifiedResourceIDs = "data_resource_verified_ids"

// VerifiedResourceID returns the id DataResourceGuard verified for field
// (compared through CanonicalJSONKey), if any.
func VerifiedResourceID(c *gin.Context, field string) (string, bool) {
	v, ok := c.Get(CtxVerifiedResourceIDs)
	if !ok {
		return "", false
	}
	m, _ := v.(map[string]string)
	id, ok := m[CanonicalJSONKey(field)]
	return id, ok
}

// DataResourceGuard enforces DataResourceRoutes (Java DataResourceAspect):
//   - the JSON body is validated first (no duplicate / aliased keys, exactly
//     one JSON value), so the id read here is the id every handler binding
//     receives;
//   - EDGE platform: only NODE_ID rules are checked (DataResourceAuth.Check);
//   - AUTONOMY callers must pass the same ownership checks; legacy
//     route-wide bypasses are deliberately unsupported;
//   - CENTER owner users are never checked (Java: ownerType == CENTER; EDGE /
//     P2P accounts on a CENTER platform are checked);
//   - otherwise the id is read from the JSON body through CanonicalJSONKey and
//     checked; a missing id is denied like Java (null id).
//
// Verified ids are stored under CtxVerifiedResourceIDs. Denials answer HTTP
// 200 with AUTH_FAILED. Install after authentication.
func DataResourceGuard(_ []string) gin.HandlerFunc {
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
		obj, err := readBodyObject(c)
		if err != nil {
			rejectBody(c, err)
			return
		}
		u := CurrentUser(c)
		if u == nil {
			if c.Request.Method == http.MethodOptions {
				c.Next()
				return
			}
			RequireDataResource(c, rule.Kind, "") // denies: no identity
			return
		}
		if u.IsCenterOwner() {
			c.Next()
			return
		}
		verified := map[string]string{}
		for _, r := range append([]DataResourceRule{rule}, rule.Extra...) {
			id, present, err := ruleValue(obj, r.Field)
			if err != nil {
				rejectBody(c, err)
				return
			}
			if r.Optional && !present {
				continue
			}
			if !RequireDataResource(c, r.Kind, id) {
				return
			}
			verified[CanonicalJSONKey(r.Field)] = id
		}
		c.Set(CtxVerifiedResourceIDs, verified)
		c.Next()
	}
}

// readBodyObject reads the request body without consuming it (it is restored
// for the handler) and decodes a top-level JSON object. Bodies that look like
// JSON must pass CheckJSONKeys. A missing / non-object body yields nil.
func readBodyObject(c *gin.Context) (map[string]json.RawMessage, error) {
	if c.Request == nil || c.Request.Body == nil || c.Request.Body == http.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(c.Request.Body)
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if isFormContentType(c.GetHeader("Content-Type")) || !looksLikeJSON(body) {
		return nil, nil
	}
	// Without CaseKeysRequest in front (inner port) the raw body is checked
	// here; after it, the body legitimately carries same-valued twins, whose
	// consistency ruleValue verifies for the guarded fields.
	if !c.GetBool(ctxJSONKeysChecked) {
		if err := CheckJSONKeys(body); err != nil {
			return nil, err
		}
	}
	trimmed := bytes.TrimSpace(body)
	if trimmed[0] != '{' {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, ErrInvalidJSON
	}
	return m, nil
}

// ruleValue returns the id of field in obj. Every key with the field's
// canonical name (the original key and CaseKeysRequest twins) must carry the
// same value, otherwise the body is ambiguous and rejected. A non-string value
// yields "" (denied, like a Java null id). null counts as absent.
func ruleValue(obj map[string]json.RawMessage, field string) (id string, present bool, err error) {
	vals := canonicalFieldValues(obj, field)
	for i, raw := range vals {
		var v string
		isNull := bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		if !isNull {
			if json.Unmarshal(raw, &v) != nil {
				v = ""
			}
			present = true
		}
		if i > 0 && v != id {
			return "", false, ErrDuplicateJSONKey
		}
		id = v
	}
	return id, present, nil
}

// VerifyRouteCoverage is the default-deny startup check: every registered
// /api route must be classified for interface RBAC (RouteClassified) and for
// data access (DataResourceRoutes or DataResourceExempt with a reason).
// cmd/server refuses to start when it returns an error.
func VerifyRouteCoverage(routes gin.RoutesInfo) error {
	var problems []string
	seen := map[string]bool{}
	for _, ri := range routes {
		p := ri.Path
		if !strings.HasPrefix(p, "/api/") || seen[p] {
			continue
		}
		seen[p] = true
		if !RouteClassified(p) {
			problems = append(problems, ri.Method+" "+p+": not classified for RBAC (APIResourceCodes / AdminOnlyPaths / NoInterfaceCheckPaths / PublicPaths)")
		}
		_, guarded := DataResourceRoutes[p]
		reason, exempt := DataResourceExempt[p]
		switch {
		case guarded && exempt:
			problems = append(problems, ri.Method+" "+p+": both in DataResourceRoutes and DataResourceExempt")
		case !guarded && (!exempt || strings.TrimSpace(reason) == ""):
			problems = append(problems, ri.Method+" "+p+": not in DataResourceRoutes and not exempted with a reason in DataResourceExempt")
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("route authorization coverage check failed:\n  %s", strings.Join(problems, "\n  "))
}
