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
		headers.Set("X-Frame-Options", "SAMEORIGIN")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("X-Permitted-Cross-Domain-Policies", "none")
		headers.Set("Permissions-Policy", "accelerometer=(), autoplay=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")

		if isSecureRequest(c) {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}
