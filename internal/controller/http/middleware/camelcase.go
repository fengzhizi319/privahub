package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

// toSnakeCase converts a camelCase / PascalCase identifier to snake_case.
// Identifiers without uppercase letters are returned unchanged.
func toSnakeCase(s string) string {
	if s == "" || !strings.ContainsFunc(s, unicode.IsUpper) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// toCamelCase converts a snake_case identifier to camelCase.
// Identifiers without underscores are returned unchanged.
func toCamelCase(s string) string {
	if s == "" || !strings.Contains(s, "_") {
		return s
	}
	parts := strings.Split(s, "_")
	var b strings.Builder
	b.Grow(len(s))
	first := true
	for _, p := range parts {
		if p == "" {
			continue
		}
		if first {
			b.WriteString(p)
			first = false
			continue
		}
		r := []rune(p)
		b.WriteRune(unicode.ToUpper(r[0]))
		if len(r) > 1 {
			b.WriteString(string(r[1:]))
		}
	}
	return b.String()
}

// expandKeys recursively walks a decoded JSON value and, for every object key,
// adds a twin key in the opposite naming convention (camelCase <-> snake_case).
// The operation is additive: original keys are never removed, so existing
// bindings on either convention keep working.
func expandKeys(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		// Collect additions separately to avoid mutating the map during iteration.
		type kv struct {
			k string
			v interface{}
		}
		var adds []kv
		for k, val := range t {
			t[k] = expandKeys(val)
			if snake := toSnakeCase(k); snake != k {
				adds = append(adds, kv{snake, t[k]})
			}
			if camel := toCamelCase(k); camel != k {
				adds = append(adds, kv{camel, t[k]})
			}
		}
		for _, a := range adds {
			if _, exists := t[a.k]; !exists {
				t[a.k] = a.v
			}
		}
		return t
	case []interface{}:
		for i, item := range t {
			t[i] = expandKeys(item)
		}
		return t
	default:
		return v
	}
}

// camelCaseResponseKeys recursively walks a decoded JSON value and adds a
// camelCase twin for every snake_case object key (additive, original kept).
func camelCaseResponseKeys(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		type kv struct {
			k string
			v interface{}
		}
		var adds []kv
		for k, val := range t {
			t[k] = camelCaseResponseKeys(val)
			if camel := toCamelCase(k); camel != k {
				adds = append(adds, kv{camel, t[k]})
			}
		}
		for _, a := range adds {
			if _, exists := t[a.k]; !exists {
				t[a.k] = a.v
			}
		}
		return t
	case []interface{}:
		for i, item := range t {
			t[i] = camelCaseResponseKeys(item)
		}
		return t
	default:
		return v
	}
}

// isJSONContentType reports whether a Content-Type header denotes JSON.
func isJSONContentType(ct string) bool {
	return strings.Contains(strings.ToLower(ct), "application/json")
}

// caseSkipPaths are path prefixes that must bypass response rewriting
// (streaming / non-JSON / infra endpoints).
var caseSkipPaths = []string{
	"/api/v1alpha1/sync", // SSE center-edge sync
	"/metrics",
	"/api/v1alpha1/healthz",
	"/assets",
	"/favicon.ico",
}

func shouldSkipCasePath(path string) bool {
	for _, p := range caseSkipPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// CaseKeysRequest normalizes incoming JSON request bodies so that handlers
// binding either snake_case or camelCase struct tags both receive their fields.
//
// The frontend contract is uniformly camelCase while many legacy Go handlers
// bind snake_case (often with binding:"required"); encoding/json does NOT match
// "projectId" against a `json:"project_id"` tag, which previously caused
// widespread ParamError responses. This middleware additively injects the twin
// key for every object key, reconciling both conventions without breaking any
// existing endpoint. Only application/json bodies are processed; multipart and
// empty bodies are left untouched.
func CaseKeysRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isJSONContentType(c.GetHeader("Content-Type")) {
			c.Next()
			return
		}
		if c.Request.Body == nil || c.Request.ContentLength == 0 {
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		_ = c.Request.Body.Close()
		if err != nil || len(bytes.TrimSpace(body)) == 0 {
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			c.Next()
			return
		}

		// Only JSON objects carry keys worth expanding; arrays of scalars and
		// non-JSON payloads are passed through unchanged.
		trimmed := bytes.TrimSpace(body)
		if trimmed[0] == '{' {
			var v interface{}
			if json.Unmarshal(body, &v) == nil {
				if expanded, mErr := json.Marshal(expandKeys(v)); mErr == nil {
					body = expanded
				}
			}
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Request.Header.Set("Content-Length", strconv.Itoa(len(body)))
		c.Next()
	}
}

// caseResponseWriter buffers the response body so it can be rewritten with
// camelCase twin keys before being flushed to the underlying writer.
type caseResponseWriter struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (w *caseResponseWriter) Write(data []byte) (int, error) {
	return w.buf.Write(data)
}

func (w *caseResponseWriter) WriteString(s string) (int, error) {
	return w.buf.WriteString(s)
}

// Flush converts the buffered payload and writes it through, then flushes the
// underlying writer (required for streaming/SSE, though those are skipped).
func (w *caseResponseWriter) Flush() {
	w.flushConverted()
	w.ResponseWriter.Flush()
}

// flushConverted drains the buffer, applies snake_case -> camelCase key
// expansion for JSON payloads, and writes the result to the underlying writer.
// Content-Length is dropped so net/http recomputes it for the new payload size.
func (w *caseResponseWriter) flushConverted() {
	if w.buf.Len() == 0 {
		return
	}
	data := w.buf.Bytes()
	w.buf.Reset()

	if isJSONContentType(w.Header().Get("Content-Type")) {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var v interface{}
			if json.Unmarshal(data, &v) == nil {
				if converted, err := json.Marshal(camelCaseResponseKeys(v)); err == nil {
					data = converted
				}
			}
		}
	}

	w.Header().Del("Content-Length")
	_, _ = w.ResponseWriter.Write(data)
}

// CaseKeysResponse rewrites JSON response bodies so that every snake_case key
// also appears in camelCase. The frontend Zod schemas / TS types are uniformly
// camelCase while legacy Go DTOs marshal snake_case; without this, strictly
// validated endpoints throw (missing required camelCase fields) and the rest
// surface empty data. The transform is additive (original keys preserved) and
// skips streaming / non-JSON / infra endpoints.
func CaseKeysResponse() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// Only API responses carry the snake_case DTOs the frontend mis-reads;
		// static assets / SPA fallback / infra endpoints are left untouched to
		// avoid buffering large non-JSON payloads.
		if !strings.HasPrefix(path, "/api/") || shouldSkipCasePath(path) {
			c.Next()
			return
		}
		cw := &caseResponseWriter{ResponseWriter: c.Writer}
		c.Writer = cw
		c.Next()
		// Fallback: handlers that never call Flush (the common c.JSON path)
		// still get their buffered payload converted and written here, before
		// net/http finalizes the response.
		cw.flushConverted()
	}
}

// ensure caseResponseWriter satisfies gin.ResponseWriter at compile time.
var _ gin.ResponseWriter = (*caseResponseWriter)(nil)

// Status passthrough is inherited; explicitly guard WriteHeader so buffered
// writes keep control of when headers hit the wire.
func (w *caseResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

// Hijack / CloseNotify / Pusher are inherited via embedding; no override needed.
var _ http.Flusher = (*caseResponseWriter)(nil)
