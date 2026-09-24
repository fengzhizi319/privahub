package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fengzhizi319/privahub/internal/audit"
	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AuditorPaths are the read-only security audit trail routes. They require
// the AUDITOR role or ADMIN/CENTER_ADMIN (checked by RBAC and again by the
// handler). There are deliberately no update/delete routes.
var AuditorPaths = map[string]bool{
	"/api/v1alpha1/audit/list":   true,
	"/api/v1alpha1/audit/verify": true,
}

// IsAuditor reports whether the (non-virtual) user may read the audit trail:
// AUDITOR, ADMIN or CENTER_ADMIN role.
func IsAuditor(ctx context.Context, db *gorm.DB, u *auth.UserContext) bool {
	if u == nil || u.IsVirtualNode() {
		return false
	}
	roles := loadRoles(ctx, db, u.Name)
	return roles[auth.RoleAuditor] || roles[auth.RoleAdmin] || roles[auth.RoleCenterAdmin]
}

// Gin context keys a handler / authz layer can set to name the verified
// resource of a request in the audit trail (preferred over body fields).
const (
	CtxAuditResourceType = "audit.resource_type"
	CtxAuditResourceID   = "audit.resource_id"
)

// SetAuditResource records the verified resource of the current request for
// the audit trail.
func SetAuditResource(c *gin.Context, resourceType, resourceID string) {
	c.Set(CtxAuditResourceType, resourceType)
	c.Set(CtxAuditResourceID, resourceID)
}

// readOnlyLastSegments are route endings of read-only POST endpoints (the
// Java-style API uses POST for reads); they are not audited unless listed in
// alwaysAuditPaths.
var readOnlyLastSegments = map[string]bool{
	"list": true, "get": true, "detail": true, "page": true, "info": true,
	"status": true, "logs": true, "log": true, "output": true, "i18n": true,
	"version": true, "current": true, "nodes": true, "listNode": true,
	"participants": true, "max_index": true, "pending": true, "id": true,
	"modelPartyPath": true, "getOutTable": true, "healthz": true, "env": true,
	"docs": true,
}

// alwaysAuditPaths are sensitive reads that are audited anyway.
var alwaysAuditPaths = map[string]bool{
	"/api/v1alpha1/audit/list":              true,
	"/api/v1alpha1/audit/verify":            true,
	"/api/v1alpha1/data/download":           true,
	"/api/v1alpha1/node/token":              true,
	"/api/v1alpha1/inst/node/token":         true,
	"/api/v1alpha1/edgeDataSync/log":        true,
	"/api/v1alpha1/user/list":               true,
	"/api/v1alpha1/nodeUser/list":           true,
	"/api/v1alpha1/inst/list":               true,
	"/api/v1alpha1/project/job/task/output": true,
}

// loginPaths carry a claimed (unauthenticated) user name worth recording.
var loginPaths = map[string]bool{
	"/api/login":               true,
	"/api/v1alpha1/user/login": true,
}

// auditIDFields are body fields holding resource ids, in primary-resource
// priority order. Only these fields are ever read from a body.
var auditIDFields = []string{
	"graphId", "jobId", "taskId", "datatableId", "datasourceId", "modelId",
	"servingId", "scheduleId", "scheduleTaskId", "routeId", "nodeRouteId",
	"voteId", "voteParticipantId", "instId", "nodeId", "projectId", "ownerId",
	"domainId", "userId",
}

// routeResourceField maps the first route segment to its primary id field.
var routeResourceField = map[string]string{
	"project": "projectId", "graph": "graphId", "node": "nodeId",
	"datatable": "datatableId", "datasource": "datasourceId", "inst": "instId",
	"model": "modelId", "scheduled": "scheduleId", "nodeRoute": "routeId",
	"approval": "voteId", "message": "voteId", "featureTable": "datatableId",
	"graphDatasource": "graphId", "data": "nodeId", "nodeUser": "nodeId",
}

const auditBodyPeek = 64 << 10

var sensitiveQueryKey = regexp.MustCompile(`(?i)token|passw|pwd|secret|key|sign|auth|cred|session`)

// shouldAudit decides whether a request is recorded.
func shouldAudit(method, path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return false
	}
	if alwaysAuditPaths[path] {
		return true
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
	default:
		return false
	}
	last := path[strings.LastIndexByte(path, '/')+1:]
	return !readOnlyLastSegments[last]
}

// AuditTrail records every state-changing (and sensitive read) /api request
// in the tamper-evident audit trail (internal/audit): actor, client IP,
// action (METHOD + route template), resource ids (verified context values or
// whitelisted body id fields), result derived from the HTTP status and the
// response status.code. Bodies, passwords and tokens are never recorded;
// sensitive query parameters are redacted.
func AuditTrail(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || !shouldAudit(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}
		start := time.Now()
		ids := peekBodyIDs(c)
		cw := &codeCaptureWriter{ResponseWriter: c.Writer}
		c.Writer = cw
		c.Next()
		c.Writer = cw.ResponseWriter

		route := c.FullPath()
		status := cw.Status()
		if route == "" {
			if status == http.StatusNotFound {
				return // unmatched route: nothing happened
			}
			route = c.Request.URL.Path
		}
		result, reason := auditResult(status, cw.code())
		e := audit.Event{
			Time:    start,
			IP:      c.ClientIP(),
			Action:  c.Request.Method + " " + route,
			Result:  result,
			Reason:  reason,
			TraceID: c.GetHeader("X-Trace-ID"),
			Extra:   map[string]string{"status": strconv.Itoa(status)},
		}
		if tid := c.Writer.Header().Get("X-Trace-ID"); tid != "" {
			e.TraceID = tid
		}
		if u := CurrentUser(c); u != nil {
			e.Actor, e.OwnerID = u.Name, u.OwnerID
			e.ActorType = audit.ActorUser
			if u.IsVirtualNode() {
				e.ActorType = audit.ActorNode
			}
		} else {
			e.ActorType = audit.ActorAnonymous
			if loginPaths[route] {
				if n := ids["name"]; n != "" {
					e.Extra["claimedUser"] = n
				}
			}
		}
		delete(ids, "name")
		e.ResourceType, e.ResourceID = auditResource(c, route, ids)
		for k, v := range ids {
			e.Extra[k] = v
		}
		if q := redactQuery(c.Request.URL.RawQuery); q != "" {
			e.Extra["query"] = q
		}
		if cw.code() != nil {
			e.Extra["code"] = strconv.FormatInt(*cw.code(), 10)
		}
		audit.Record(c.Request.Context(), e)
		log.Info("audit",
			zap.String("actor", e.Actor),
			zap.String("action", e.Action),
			zap.String("result", result),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(start)),
			zap.String("ip", e.IP),
		)
	}
}

func auditResource(c *gin.Context, route string, ids map[string]string) (string, string) {
	if v, ok := c.Get(CtxAuditResourceID); ok {
		if id, _ := v.(string); id != "" {
			t, _ := c.Get(CtxAuditResourceType)
			ts, _ := t.(string)
			return ts, id
		}
	}
	seg := strings.TrimPrefix(route, "/api/v1alpha1/")
	seg = strings.TrimPrefix(seg, "/api/")
	seg = strings.TrimPrefix(seg, "p2p/")
	if i := strings.IndexByte(seg, '/'); i >= 0 {
		seg = seg[:i]
	}
	if f, ok := routeResourceField[seg]; ok && ids[f] != "" {
		return seg, ids[f]
	}
	for _, f := range auditIDFields {
		if ids[f] != "" {
			return strings.TrimSuffix(f, "Id"), ids[f]
		}
	}
	return seg, ""
}

func auditResult(status int, code *int64) (string, string) {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return audit.ResultDenied, "http " + strconv.Itoa(status)
	case status >= 400:
		return audit.ResultFailure, "http " + strconv.Itoa(status)
	case code == nil || *code == 0:
		return audit.ResultSuccess, ""
	case *code == int64(errcode.AuthFailed.Code):
		return audit.ResultDenied, "code " + strconv.FormatInt(*code, 10)
	default:
		return audit.ResultFailure, "code " + strconv.FormatInt(*code, 10)
	}
}

// peekBodyIDs reads (at most 64KiB of) a JSON body without consuming it and
// returns only whitelisted id fields (camelCase or snake_case) — never the
// body itself. "name" is returned for login routes (claimed user).
func peekBodyIDs(c *gin.Context) map[string]string {
	out := map[string]string{}
	req := c.Request
	if req.Body == nil || req.Body == http.NoBody {
		return out
	}
	ct := req.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(strings.ToLower(ct), "json") {
		return out
	}
	orig := req.Body
	buf := make([]byte, auditBodyPeek+1)
	n, err := io.ReadFull(orig, buf)
	buf = buf[:n]
	complete := errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
	req.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(buf), orig), orig}
	if !complete || n > auditBodyPeek {
		return out
	}
	trimmed := bytes.TrimSpace(buf)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return out
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(trimmed, &m) != nil {
		return out
	}
	get := func(field string) string {
		for _, key := range []string{field, toSnakeCase(field)} {
			raw, ok := m[key]
			if !ok {
				continue
			}
			var s string
			if json.Unmarshal(raw, &s) == nil {
				return strings.TrimSpace(s)
			}
			var num json.Number
			if json.Unmarshal(raw, &num) == nil {
				return num.String()
			}
		}
		return ""
	}
	for _, f := range auditIDFields {
		if v := get(f); v != "" {
			out[f] = v
		}
	}
	if p := c.Request.URL.Path; loginPaths[p] {
		for _, f := range []string{"name", "username", "userName"} {
			if v := get(f); v != "" {
				out["name"] = v
				break
			}
		}
	}
	return out
}

func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return "[unparsable]"
	}
	for k := range vals {
		if sensitiveQueryKey.MatchString(k) {
			vals[k] = []string{"***"}
		}
	}
	q := vals.Encode()
	if len(q) > 512 {
		q = q[:512]
	}
	return q
}

// statusCodeRe matches {"status":{"code":N,...}} (response.Body); topCodeRe
// matches a top-level {"code":N,...} body.
var (
	statusCodeRe = regexp.MustCompile(`"status"\s*:\s*\{[^{}]*?"code"\s*:\s*(-?\d+)`)
	topCodeRe    = regexp.MustCompile(`^\s*\{\s*"code"\s*:\s*(-?\d+)`)
)

// codeCaptureWriter keeps the first bytes of the response only to extract
// the JSON status.code; nothing of the body is recorded.
type codeCaptureWriter struct {
	gin.ResponseWriter
	head []byte
}

func (w *codeCaptureWriter) capture(b []byte) {
	if room := 512 - len(w.head); room > 0 {
		if len(b) > room {
			b = b[:room]
		}
		w.head = append(w.head, b...)
	}
}

func (w *codeCaptureWriter) Write(b []byte) (int, error) {
	w.capture(b)
	return w.ResponseWriter.Write(b)
}

func (w *codeCaptureWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *codeCaptureWriter) code() *int64 {
	if !strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "json") {
		return nil
	}
	m := statusCodeRe.FindSubmatch(w.head)
	if m == nil {
		m = topCodeRe.FindSubmatch(w.head)
	}
	if m == nil {
		return nil
	}
	v, err := strconv.ParseInt(string(m[1]), 10, 64)
	if err != nil {
		return nil
	}
	return &v
}
