package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

// RequestID propagates or creates a request identifier for tracing.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set("request_id", requestID)
		c.Writer.Header().Set(requestIDHeader, requestID)
		c.Next()
	}
}

// SecurityHeaders applies a conservative set of production-safe response headers.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Writer.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("X-XSS-Protection", "0") // modern browsers use CSP instead
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("X-Permitted-Cross-Domain-Policies", "none")
		headers.Set("Permissions-Policy", "accelerometer=(), autoplay=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")

		if isSecureRequest(c) {
			headers.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}

		// Cache-control for non-public API responses (prevents sensitive data caching)
		if !isCacheablePath(c.Request.URL.Path) {
			headers.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
			headers.Set("Pragma", "no-cache")
		}

		c.Next()
	}
}

// isCacheablePath returns true for static asset paths that may be cached.
func isCacheablePath(path string) bool {
	for _, ext := range []string{".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}
