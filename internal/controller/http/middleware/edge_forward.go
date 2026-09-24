package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/pkg/auth"
	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"github.com/fengzhizi319/privahub/pkg/response"
)

// EdgeForwardConfig configures EdgeForward (Java EdgeRequestFilter).
type EdgeForwardConfig struct {
	// Forward are the write endpoints proxied to the center (edge.forward).
	Forward []string
	// Include are the /api/v1alpha1 endpoints served locally (edge.include);
	// every other /api/v1alpha1 path answers 404.
	Include []string
	// Gateway is the local Kuscia gateway (secretpad.gateway).
	Gateway string
	// CenterHost is the Host header routed to the center
	// (secretpad.center-platform-service).
	CenterHost string
	// NodeID is this edge node (kuscia-origin-source).
	NodeID string
	// Sessions validates the caller's login before forwarding (nil disables
	// the check, i.e. auth.enabled=false).
	Sessions SessionValidator
	// JWT verifies access-token signature / expiry / type like SessionAuth
	// (optional; the server-side session is checked regardless).
	JWT *auth.JWTManager
	// Client overrides the proxy HTTP client (default 60s timeout).
	Client *http.Client
	// MaxResponseBytes caps the proxied center response (default
	// DefaultEdgeForwardMaxResponse); larger responses fail with a generic error.
	MaxResponseBytes int64
	// MaxRequestBytes caps the forwarded request body (default
	// DefaultEdgeForwardMaxResponse); EdgeForward runs before the global
	// body limit.
	MaxRequestBytes int64
	Log             *zap.Logger
}

// DefaultEdgeForwardMaxResponse is the default cap of a forwarded response.
const DefaultEdgeForwardMaxResponse int64 = 32 << 20

// hopHeaders are not forwarded: hop-by-hop headers, the caller's credentials
// (the center serves the request as this node's virtual user, identified by
// kuscia-origin-source, so a user session must never leave the edge), and
// client-address / identity headers the center must not trust.
var hopHeaders = map[string]bool{
	"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Host": true, "Accept-Encoding": true, "Content-Length": true, "Kuscia-Origin-Source": true,
	// Credentials.
	"User-Token": true, "Token": true, "Authorization": true, "Cookie": true, "X-Csrf-Token": true, "X-Xsrf-Token": true,
	"X-Privahub-Inner-Token": true,
	// Client address / proxy identity.
	"Forwarded": true, "X-Forwarded-For": true, "X-Forwarded-Host": true, "X-Forwarded-Proto": true,
	"X-Real-Ip": true,
}

// skipForwardHeader reports whether an incoming header must not be copied to
// the center request (static list plus headers named in Connection).
func skipForwardHeader(h http.Header, key string) bool {
	k := http.CanonicalHeaderKey(key)
	if hopHeaders[k] {
		return true
	}
	for _, v := range h.Values("Connection") {
		for _, f := range strings.Split(v, ",") {
			if http.CanonicalHeaderKey(strings.TrimSpace(f)) == k {
				return true
			}
		}
	}
	return false
}

func pathSet(paths []string) map[string]bool {
	m := make(map[string]bool, len(paths))
	for _, p := range paths {
		if p = strings.TrimSpace(p); p != "" {
			m[strings.TrimRight(p, "/")] = true
		}
	}
	return m
}

// EdgeForward is the EDGE request filter (Java EdgeRequestFilter, highest
// precedence): Forward paths are proxied to the center through the Kuscia
// gateway (after checking the caller's session) with Host=CenterHost and
// kuscia-origin-source=NodeID; other /api/v1alpha1 paths not in Include answer
// 404; everything else continues.
func EdgeForward(cfg EdgeForwardConfig) gin.HandlerFunc {
	forward := pathSet(cfg.Forward)
	include := pathSet(cfg.Include)
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	log := cfg.Log
	if log == nil {
		log = zap.NewNop()
	}
	gw := strings.TrimRight(strings.TrimSpace(cfg.Gateway), "/")
	if gw != "" && !strings.HasPrefix(gw, "http://") && !strings.HasPrefix(gw, "https://") {
		gw = "http://" + gw
	}
	return func(c *gin.Context) {
		p := strings.TrimRight(c.Request.URL.Path, "/")
		if forward[p] {
			if c.Request.Method == http.MethodOptions {
				c.Next()
				return
			}
			if !edgeCheckLogin(c, cfg.JWT, cfg.Sessions) {
				return
			}
			edgeProxy(c, client, gw, cfg, log)
			return
		}
		if strings.HasPrefix(p, "/api/v1alpha1") && !include[p] {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.Next()
	}
}

// edgeCheckLogin mirrors EdgeRequestFilter.checkUserLogin, but applies the
// full SessionAuth rule set (authenticateSession): the forwarded write runs on
// the center as this node, so an ambient cookie without CSRF, an expired or
// refresh JWT, a stale password version or a must_change_password session must
// be rejected here — the center cannot tell them apart.
func edgeCheckLogin(c *gin.Context, jwtManager *auth.JWTManager, sessions SessionValidator) bool {
	if sessions == nil {
		return true
	}
	_, _, _, ok := authenticateSession(c, jwtManager, sessions)
	return ok
}

func edgeProxy(c *gin.Context, client *http.Client, gw string, cfg EdgeForwardConfig, log *zap.Logger) {
	var body []byte
	if c.Request.Body != nil {
		maxReq := cfg.MaxRequestBytes
		if maxReq <= 0 {
			maxReq = DefaultEdgeForwardMaxResponse
		}
		var err error
		body, err = io.ReadAll(io.LimitReader(c.Request.Body, maxReq+1))
		if err != nil || int64(len(body)) > maxReq {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"status": gin.H{"code": 413, "msg": "request body too large or unreadable"},
			})
			return
		}
	}
	target := gw + c.Request.URL.Path
	if c.Request.URL.RawQuery != "" {
		target += "?" + c.Request.URL.RawQuery
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	// Java always forwards as POST.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		log.Warn("edge forward: build request failed", zap.String("uri", c.Request.URL.Path), zap.Error(err))
		edgeForwardFailed(c)
		return
	}
	for k, vs := range c.Request.Header {
		if skipForwardHeader(c.Request.Header, k) {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if cfg.CenterHost != "" {
		req.Host = cfg.CenterHost
	}
	req.Header.Set("kuscia-origin-source", cfg.NodeID)
	innerauth.Apply(req)
	log.Info("edge forward", zap.String("uri", c.Request.URL.Path), zap.String("target", target))
	resp, err := client.Do(req)
	if err != nil {
		log.Warn("edge forward failed", zap.String("uri", c.Request.URL.Path), zap.Error(err))
		edgeForwardFailed(c)
		return
	}
	defer resp.Body.Close()
	limit := cfg.MaxResponseBytes
	if limit <= 0 {
		limit = DefaultEdgeForwardMaxResponse
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(out)) > limit {
		log.Warn("edge forward: center response unreadable or too large",
			zap.String("uri", c.Request.URL.Path), zap.Int64("limit", limit), zap.Error(err))
		edgeForwardFailed(c)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json; charset=utf-8"
	}
	c.Data(resp.StatusCode, ct, out)
	c.Abort()
}

// edgeForwardFailed answers a generic remote-call error; details are logged
// server-side only (no transport errors / internal addresses leak to clients).
func edgeForwardFailed(c *gin.Context) {
	response.FailArgs(c, errcode.SystemRemoteCallError, "request forwarding to the center failed")
	c.Abort()
}
