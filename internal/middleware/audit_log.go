package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	auditLogPrefix     = "/api/v2/admin/"
	maxBodyCaptureSize = 4096 // bytes to capture from request body
	maxBodyLogLength   = 512  // bytes to include in slog output
	maxBodyDBLength    = 4096 // bytes to persist in database
)

// AuditLog returns a gin middleware that records all admin API operations
// to both structured slog output and the v2_audit_log database table.
// It only applies to requests whose path starts with /api/v2/admin/.
func AuditLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only audit admin API paths
		if !strings.HasPrefix(c.Request.URL.Path, auditLogPrefix) {
			c.Next()
			return
		}

		// Only log write operations (POST, PUT, DELETE)
		method := c.Request.Method
		if method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete {
			c.Next()
			return
		}

		// Capture request body and restore it for downstream handlers
		var reqBody string
		if c.Request.Body != nil && c.Request.ContentLength > 0 {
			bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyCaptureSize))
			if err == nil && len(bodyBytes) > 0 {
				reqBody = string(bodyBytes)
				// Restore the body so downstream handlers can read it
				c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
		}

		// Extract user info from JWT context (set by JWTAuth middleware)
		userID, _ := c.Get("user_id")
		email, _ := c.Get("email")

		var uidPtr *uint
		if id, ok := userID.(uint); ok {
			uidPtr = &id
		}

		userAgent := c.GetHeader("User-Agent")
		clientIP := c.ClientIP()
		requestID := c.Writer.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = c.GetHeader("X-Request-ID")
		}

		startTime := time.Now()

		// Let the actual handler run
		c.Next()

		duration := time.Since(startTime).Milliseconds()
		statusCode := c.Writer.Status()

		// Derive module and action from the path
		module, action := extractModuleAndAction(c.Request.URL.Path, method)

		// Build error message for failed requests
		var errMsg string
		if statusCode >= 400 && len(c.Errors) > 0 {
			errMsg = c.Errors.Last().Error()
		}

		// Log via slog with structured attributes
		logAttrs := []any{
			slog.Bool("audit", true),
			slog.String("user_id", userIDToString(userID)),
			slog.String("email", emailStr(email)),
			slog.String("method", method),
			slog.String("path", c.Request.URL.Path),
			slog.String("module", module),
			slog.String("action", action),
			slog.String("ip", clientIP),
			slog.Int("status_code", statusCode),
			slog.Int64("duration_ms", duration),
			slog.String("request_id", requestID),
		}

		if reqBody != "" {
			logAttrs = append(logAttrs, slog.String("request_body", truncate(reqBody, maxBodyLogLength)))
		}
		if errMsg != "" {
			logAttrs = append(logAttrs, slog.String("error", errMsg))
		}

		if statusCode >= 500 {
			slog.Error("admin audit log", logAttrs...)
		} else {
			slog.Info("admin audit log", logAttrs...)
		}

		// Persist to database (non-blocking, fire-and-forget)
		go persistAuditLog(uidPtr, emailStr(email), method, c.Request.URL.Path, module, action, clientIP, userAgent, requestID, reqBody, statusCode, duration, errMsg)
	}
}

// persistAuditLog writes an audit record to the database.
// Runs in a goroutine so it never blocks the request flow.
func persistAuditLog(userID *uint, email, method, path, module, action, ip, userAgent, requestID, reqBody string, statusCode int, durationMs int64, errMsg string) {
	db := database.GetDB()
	if db == nil {
		return
	}

	entry := model.AuditLog{
		UserID:       userID,
		Email:        email,
		Method:       method,
		Path:         path,
		Module:       module,
		Action:       action,
		IP:           ip,
		UserAgent:    userAgent,
		RequestID:    requestID,
		RequestBody:  truncate(reqBody, maxBodyDBLength),
		StatusCode:   statusCode,
		DurationMS:   durationMs,
		ErrorMessage: errMsg,
	}

	// Ignore errors: audit log should never affect the main request flow
	_ = db.Create(&entry)
}

// extractModuleAndAction derives a human-readable module and action from the URL path and HTTP method.
func extractModuleAndAction(path, method string) (module, action string) {
	// Strip the /api/v2/admin/ prefix
	trimmed := strings.TrimPrefix(path, auditLogPrefix)
	if trimmed == "" {
		return "admin", strings.ToLower(method)
	}

	// First path segment is the module
	parts := strings.SplitN(trimmed, "/", 2)
	module = parts[0]

	// Determine action from HTTP method and path patterns
	switch method {
	case http.MethodPost:
		action = "create"
		// Override with semantic action names for common patterns
		if strings.Contains(path, "/ban") {
			action = "ban"
		} else if strings.Contains(path, "/unban") {
			action = "unban"
		} else if strings.Contains(path, "/reset-traffic") {
			action = "reset_traffic"
		} else if strings.Contains(path, "/paid") {
			action = "mark_paid"
		} else if strings.Contains(path, "/cancel") {
			action = "cancel"
		} else if strings.Contains(path, "/sync") {
			action = "sync"
		} else if strings.Contains(path, "/toggle") {
			action = "toggle"
		} else if strings.Contains(path, "/check") {
			action = "check"
		} else if strings.Contains(path, "/assign") {
			action = "assign"
		} else if strings.Contains(path, "/reply") {
			action = "reply"
		} else if strings.Contains(path, "/close") {
			action = "close"
		} else if strings.Contains(path, "/test") {
			action = "test"
		} else if strings.Contains(path, "/process") {
			action = "process"
		} else if strings.Contains(path, "/restore") {
			action = "restore"
		} else if strings.Contains(path, "/execute") {
			action = "execute"
		} else if strings.Contains(path, "/diagnose") {
			action = "diagnose"
		} else if strings.Contains(path, "/validate") {
			action = "validate"
		} else if strings.Contains(path, "/pause") {
			action = "pause"
		} else if strings.Contains(path, "/resume") {
			action = "resume"
		} else if strings.Contains(path, "/preview") {
			action = "preview"
		}
	case http.MethodPut:
		action = "update"
	case http.MethodDelete:
		action = "delete"
	default:
		action = strings.ToLower(method)
	}

	return module, action
}

// userIDToString safely converts a JWT user_id context value to string.
func userIDToString(v any) string {
	if v == nil {
		return "unknown"
	}
	switch id := v.(type) {
	case uint:
		return strconv.FormatUint(uint64(id), 10)
	case uint64:
		return strconv.FormatUint(id, 10)
	case int:
		return strconv.FormatInt(int64(id), 10)
	}
	return "unknown"
}

// emailStr safely converts a JWT email context value to string.
func emailStr(v any) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return "unknown"
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
