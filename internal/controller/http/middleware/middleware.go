// Package middleware provides Gin HTTP middleware for SecretPad-Go.
package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/logger"
	"github.com/fengzhizi319/privahub/pkg/metrics"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TraceID reads the trace ID from `Trace-Id` (Java SecretPad / frontend) or
// `X-Trace-ID`, generating one when absent, injects it into the request
// context and echoes it in both response headers.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("Trace-Id")
		if traceID == "" {
			traceID = c.GetHeader("X-Trace-ID")
		}
		if traceID == "" {
			traceID = uuid.New().String()
		}
		ctx := logger.ContextWithTraceID(c.Request.Context(), traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Trace-ID", traceID)
		c.Header("Trace-Id", traceID)
		c.Next()
	}
}

// Recovery recovers from panics and logs the error.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("Recovered from panic",
					zap.Any("panic", r),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(200, gin.H{
					"status": gin.H{"code": errcode.SystemUnknownError.Code, "msg": "system unknown error"},
				})
			}
		}()
		c.Next()
	}
}

// Metrics records HTTP request metrics for Prometheus.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			// Bug51 fix: use a fixed label for unmatched routes to prevent
			// unbounded Prometheus label cardinality from arbitrary 404 paths.
			path = "__unmatched__"
		}

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer.Status()

		metrics.HTTPRequestsTotal.WithLabelValues(
			c.Request.Method, path, strconv.Itoa(status),
		).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(
			c.Request.Method, path,
		).Observe(duration)
	}
}

// CORS configures Cross-Origin Resource Sharing headers with explicit allowed
// origins (server.cors_allowed_origins). "*" allows any origin (the request
// origin is echoed so credentials keep working).
func CORS(allowedOrigins ...string) gin.HandlerFunc {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:8000", "http://127.0.0.1:8000"}
	}
	originSet := make(map[string]bool, len(allowedOrigins))
	anyOrigin := false
	for _, o := range allowedOrigins {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "*" {
			anyOrigin = true
		}
		originSet[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		} else if origin != "" && anyOrigin {
			// "*" never grants credentials: with cookie sessions that would
			// let any site read authenticated responses.
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Trace-ID, Trace-Id, User-Token, token, Accept-Language, X-CSRF-Token, X-Privahub-Session")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// DefaultContentSecurityPolicy is the CSP sent by the server. It matches the
// <meta> CSP injected by the SPA build (web/apps/secretpad/vite.config.ts)
// and adds the header-only directive frame-ancestors. style-src keeps
// 'unsafe-inline' for runtime-inserted styles only; scripts are strictly
// 'self' (no inline scripts, no eval).
const DefaultContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// SecurityHeaderOptions configures SecurityHeadersWith.
type SecurityHeaderOptions struct {
	// HSTS sends Strict-Transport-Security on every response (main port
	// served over TLS or behind a declared TLS proxy). Requests that arrive
	// over TLS directly always get it.
	HSTS bool
	// CSP overrides DefaultContentSecurityPolicy ("-" disables the header).
	CSP string
}

// SecurityHeaders adds security-related HTTP response headers.
func SecurityHeaders() gin.HandlerFunc {
	return SecurityHeadersWith(SecurityHeaderOptions{})
}

// SecurityHeadersWith adds the security response headers: CSP,
// X-Content-Type-Options, X-Frame-Options, Referrer-Policy,
// Permissions-Policy, Cross-Origin-Opener-Policy, Cache-Control and (when
// served over TLS) HSTS. The deprecated X-XSS-Protection header is not sent.
func SecurityHeadersWith(o SecurityHeaderOptions) gin.HandlerFunc {
	csp := o.CSP
	if csp == "" {
		csp = DefaultContentSecurityPolicy
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		if csp != "-" {
			h.Set("Content-Security-Policy", csp)
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		if o.HSTS || c.Request.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// ExtraHeaders adds configured response headers to every response (Java
// secretpad.response.extra-headers, e.g. Content-Security-Policy).
func ExtraHeaders(headers map[string]string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for k, v := range headers {
			if k != "" {
				c.Header(k, v)
			}
		}
		c.Next()
	}
}

// AuditLog records every state-changing /api request in the tamper-evident
// security audit trail (see AuditTrail in audit_trail.go) and logs it.
func AuditLog(log *zap.Logger) gin.HandlerFunc {
	return AuditTrail(log)
}

// IPWhitelist restricts access to requests from allowed CIDR ranges.
// If allowedCIDRs is empty, all IPs are allowed.
func IPWhitelist(allowedCIDRs ...string) gin.HandlerFunc {
	var networks []*net.IPNet
	for _, cidr := range allowedCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try as single IP
			ip := net.ParseIP(cidr)
			if ip != nil {
				mask := net.CIDRMask(32, 32)
				if ip.To4() == nil {
					mask = net.CIDRMask(128, 128)
				}
				ipNet = &net.IPNet{IP: ip, Mask: mask}
			} else {
				continue
			}
		}
		networks = append(networks, ipNet)
	}

	return func(c *gin.Context) {
		if len(networks) == 0 {
			c.Next()
			return
		}

		clientIP := net.ParseIP(c.ClientIP())
		if clientIP == nil {
			c.AbortWithStatusJSON(403, gin.H{
				"status": gin.H{"code": 403, "msg": "forbidden: invalid client IP"},
			})
			return
		}

		for _, ipNet := range networks {
			if ipNet.Contains(clientIP) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(403, gin.H{
			"status": gin.H{"code": 403, "msg": "forbidden: IP not in whitelist"},
		})
	}
}

// --- Rate Limiter (token bucket per IP) ---

type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func (b *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimiter limits requests per IP using a token bucket algorithm.
// maxTokens is the burst size, refillRate is tokens added per second.
func RateLimiter(maxTokens float64, refillRate float64) gin.HandlerFunc {
	var mu sync.Mutex
	buckets := make(map[string]*tokenBucket)

	// Periodic cleanup of stale entries (every 5 minutes)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, b := range buckets {
				if b.lastRefill.Before(cutoff) {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		b, exists := buckets[ip]
		if !exists {
			b = &tokenBucket{
				tokens:     maxTokens,
				maxTokens:  maxTokens,
				refillRate: refillRate,
				lastRefill: time.Now(),
			}
			buckets[ip] = b
		}
		allowed := b.allow()
		mu.Unlock()

		if !allowed {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"status": gin.H{"code": 429, "msg": "too many requests"},
			})
			return
		}
		c.Next()
	}
}

// BodyLimit restricts the maximum request body size.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return BodyLimitWithOverrides(maxBytes, nil)
}

// BodyLimitWithOverrides restricts the request body size to defaultMax, with
// per-path overrides (exact URL path match). A limit <= 0 means unlimited, so
// upload routes can be exempted from the global limit.
func BodyLimitWithOverrides(defaultMax int64, overrides map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := defaultMax
		if v, ok := overrides[c.Request.URL.Path]; ok {
			maxBytes = v
		}
		if maxBytes <= 0 {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"status": gin.H{"code": 413, "msg": "request body too large"},
			})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
