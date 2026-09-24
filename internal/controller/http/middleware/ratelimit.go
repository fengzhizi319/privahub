package middleware

import (
	"sync"
	"time"

	"github.com/fengzhizi319/privahub/pkg/errcode"
	"github.com/fengzhizi319/privahub/pkg/response"
	"github.com/gin-gonic/gin"
)

// KeyedLimiter is a set of token buckets keyed by an arbitrary string (user
// name, "login:"+IP ...). It mirrors Java RateLimitUtil: one Guava
// RateLimiter per key created with rate permits/window, evicted one hour
// after the last access. With burst 1 the behaviour matches Guava's
// SmoothBursty limiter (the first request passes, then one request per
// window/permits interval).
type KeyedLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     float64 // tokens per second
	burst    float64
	idleTTL  time.Duration
	lastScan time.Time
	now      func() time.Time
}

// NewKeyedLimiter returns a limiter allowing permits per window per key with
// the given burst (<1 is treated as 1). permits<=0 or window<=0 returns nil,
// which allows everything.
func NewKeyedLimiter(permits float64, window time.Duration, burst float64) *KeyedLimiter {
	if permits <= 0 || window <= 0 {
		return nil
	}
	if burst < 1 {
		burst = 1
	}
	return &KeyedLimiter{
		buckets: map[string]*tokenBucket{},
		rate:    permits / window.Seconds(),
		burst:   burst,
		idleTTL: time.Hour,
		now:     time.Now,
	}
}

// Allow consumes a permit for key and reports whether it was available.
func (l *KeyedLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastScan) > l.idleTTL {
		for k, b := range l.buckets {
			if now.Sub(b.lastRefill) > l.idleTTL {
				delete(l.buckets, k)
			}
		}
		l.lastScan = now
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &tokenBucket{tokens: l.burst, maxTokens: l.burst, refillRate: l.rate, lastRefill: now}
		l.buckets[key] = b
	}
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

// abortTooFrequent answers like Java SecretpadException(REQUEST_FREQUENCY_ERROR):
// HTTP 200 with the business error code.
func abortTooFrequent(c *gin.Context) {
	c.AbortWithStatusJSON(200, response.Body{Status: &response.Status{
		Code: errcode.SystemRequestFrequencyError.Code,
		Msg:  response.Message(c, errcode.SystemRequestFrequencyError),
	}})
}

// UserRateLimit applies the per-user limiter (Java RateLimitUtil.verifyRate())
// to the given route paths. It must run after authentication; requests
// without a user context are keyed by client IP. All paths share one bucket
// per user, like Java's single limiter per user name.
func UserRateLimit(l *KeyedLimiter, paths []string) gin.HandlerFunc {
	guarded := make(map[string]bool, len(paths))
	for _, p := range paths {
		guarded[p] = true
	}
	return func(c *gin.Context) {
		if l == nil || len(guarded) == 0 {
			c.Next()
			return
		}
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if !guarded[path] {
			c.Next()
			return
		}
		key := "ip:" + c.ClientIP()
		if u := CurrentUser(c); u != nil && u.Name != "" {
			key = "user:" + u.Name
		}
		if !l.Allow(key) {
			abortTooFrequent(c)
			return
		}
		c.Next()
	}
}

// LoginRateLimit limits login attempts per client IP (Java AuthController:
// verifyRate("login:" + remoteHost, loginRateLimitPerMinute, 60)).
func LoginRateLimit(l *KeyedLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if l != nil && !l.Allow("login:"+c.ClientIP()) {
			abortTooFrequent(c)
			return
		}
		c.Next()
	}
}
