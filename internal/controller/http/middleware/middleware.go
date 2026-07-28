// Package middleware provides Gin HTTP middleware for SecretPad-Go.
package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/fengzhizi319/privahub/pkg/logger"
	"github.com/fengzhizi319/privahub/pkg/metrics"
	"go.uber.org/zap"
)

// TraceID generates a unique trace ID for each request and injects it into context.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.New().String()
		}
		ctx := logger.ContextWithTraceID(c.Request.Context(), traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Trace-ID", traceID)
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
					"status": gin.H{"code": 202011500, "msg": "system unknown error"},
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
			path = c.Request.URL.Path
		}

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer.Status()

		metrics.HTTPRequestsTotal.WithLabelValues(
			c.Request.Method, path, string(rune(status)),
		).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(
			c.Request.Method, path,
		).Observe(duration)
	}
}

// CORS configures Cross-Origin Resource Sharing headers with explicit allowed origins.
func CORS(allowedOrigins ...string) gin.HandlerFunc {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:8000", "http://127.0.0.1:8000"}
	}
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Trace-ID")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// SecurityHeaders adds security-related HTTP response headers.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// AuditLog records structured audit information for all write operations
// (POST/PUT/DELETE/PATCH) to /api/v1alpha1/ endpoints.
func AuditLog(log *zap.Logger) gin.HandlerFunc {
	writeMethods := map[string]bool{
		"POST": true, "PUT": true, "DELETE": true, "PATCH": true,
	}
	return func(c *gin.Context) {
		if !writeMethods[c.Request.Method] || !strings.HasPrefix(c.Request.URL.Path, "/api/v1alpha1/") {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		userID, _ := c.Get("userID")
		log.Info("audit",
			zap.Any("userID", userID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
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
	return func(c *gin.Context) {
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
