package middleware

import (
	"bytes"
	"context"
	"github.com/fengzhizi319/privahub/pkg/innerauth"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/fengzhizi319/privahub/pkg/errcode"
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
	// Sessions validates the caller's login before forwarding.
	Sessions SessionValidator
	// Client overrides the proxy HTTP client (default 60s timeout).
	Client *http.Client
	Log    *zap.Logger
}

// hopHeaders are not forwarded.
var hopHeaders = map[string]bool{
	"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Host": true, "Accept-Encoding": true, "Content-Length": true, "Kuscia-Origin-Source": true,
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
			if !edgeCheckLogin(c, cfg.Sessions) {
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

// edgeCheckLogin mirrors EdgeRequestFilter.checkUserLogin.
func edgeCheckLogin(c *gin.Context, sessions SessionValidator) bool {
	if sessions == nil {
		return true
	}
	token := ExtractToken(c)
	if token == "" {
		response.FailArgs(c, errcode.AuthFailed, "Cannot find token in db, user not login in.")
		c.Abort()
		return false
	}
	u, err := sessions.ValidateSession(c.Request.Context(), token)
	if err != nil || u == nil {
		response.FailArgs(c, errcode.AuthFailed, "The login session is expire, please login again.")
		c.Abort()
		return false
	}
	return true
}

func edgeProxy(c *gin.Context, client *http.Client, gw string, cfg EdgeForwardConfig, log *zap.Logger) {
	var body []byte
	if c.Request.Body != nil {
		body, _ = io.ReadAll(c.Request.Body)
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
		response.FailArgs(c, errcode.SystemRemoteCallError, err.Error())
		c.Abort()
		return
	}
	for k, vs := range c.Request.Header {
		if hopHeaders[http.CanonicalHeaderKey(k)] {
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
		response.FailArgs(c, errcode.SystemRemoteCallError, err.Error())
		c.Abort()
		return
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json; charset=utf-8"
	}
	c.Data(resp.StatusCode, ct, out)
	c.Abort()
}
