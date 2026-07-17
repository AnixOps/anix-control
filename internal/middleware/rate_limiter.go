package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// tokenBucket implements a token bucket rate limiter for a single client.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// allow checks if a request is allowed, consuming one token if so.
func (b *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = math.Min(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// RateLimiter is a thread-safe per-IP rate limiter using token buckets.
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*tokenBucket
	rate        float64 // requests per second
	burst       float64 // max burst size
	ttl         time.Duration
	exemptPaths []string
	bypassTest  bool
}

// RateLimiterOption configures optional behaviour on a RateLimiter.
type RateLimiterOption func(*RateLimiter)

// WithTTL sets the expiry duration for unused buckets.
func WithTTL(ttl time.Duration) RateLimiterOption {
	return func(rl *RateLimiter) { rl.ttl = ttl }
}

// WithExemptPaths sets URL path prefixes that bypass rate limiting.
func WithExemptPaths(paths []string) RateLimiterOption {
	return func(rl *RateLimiter) { rl.exemptPaths = paths }
}

// WithTestModeBypass controls whether gin.TestMode bypasses this limiter.
// Existing API limiters keep the historical bypass; security-sensitive node
// downloads can exercise the exact limiter in integration tests.
func WithTestModeBypass(enabled bool) RateLimiterOption {
	return func(rl *RateLimiter) { rl.bypassTest = enabled }
}

// NewRateLimiter creates a new RateLimiter with the given rate and burst.
//   - rate: sustained requests per second
//   - burst: maximum burst size (initial tokens)
func NewRateLimiter(rate, burst float64, opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		buckets:    make(map[string]*tokenBucket),
		rate:       rate,
		burst:      burst,
		ttl:        5 * time.Minute,
		bypassTest: true,
	}
	for _, opt := range opts {
		opt(rl)
	}
	return rl
}

// isExempt returns true when the request path matches an exempt prefix.
func (rl *RateLimiter) isExempt(path string) bool {
	for _, p := range rl.exemptPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// cleanup removes expired buckets to bound memory usage.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for ip, b := range rl.buckets {
		if now.Sub(b.lastRefill) > rl.ttl {
			delete(rl.buckets, ip)
		}
	}
}

// StartCleanup starts a periodic cleanup goroutine.
// It should be called once at application startup.
func (rl *RateLimiter) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()
}

// Middleware returns a gin.HandlerFunc that enforces rate limiting per client IP.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return rl.middleware(func(c *gin.Context) string { return c.ClientIP() })
}

// MiddlewareForContextKey enforces independent buckets for an authenticated
// identity stored in Gin context, such as a node_id.
func (rl *RateLimiter) MiddlewareForContextKey(contextKey string) gin.HandlerFunc {
	return rl.middleware(func(c *gin.Context) string {
		if value, exists := c.Get(contextKey); exists {
			return contextKey + ":" + fmt.Sprint(value)
		}
		return "ip:" + c.ClientIP()
	})
}

func (rl *RateLimiter) middleware(keyFor func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Test mode: skip rate limiting entirely
		if gin.Mode() == gin.TestMode && rl.bypassTest {
			c.Next()
			return
		}

		// Exempt paths skip rate limiting entirely.
		if rl.isExempt(c.Request.URL.Path) {
			c.Next()
			return
		}

		key := keyFor(c)

		rl.mu.Lock()
		bucket, ok := rl.buckets[key]
		if !ok {
			bucket = &tokenBucket{
				tokens:     rl.burst,
				maxTokens:  rl.burst,
				refillRate: rl.rate,
				lastRefill: time.Now(),
			}
			rl.buckets[key] = bucket
		}
		allowed := bucket.allow()
		rl.mu.Unlock()

		if !allowed {
			retryAfter := int(math.Ceil(1.0 / rl.rate))
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Limit", strconv.FormatFloat(rl.rate, 'f', 0, 64))
			c.Header("X-RateLimit-Remaining", "0")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}

		c.Next()
	}
}
