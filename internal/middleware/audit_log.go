package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	auditLogPrefixV2 = "/api/v2/admin/"
	auditLogPrefixV3 = "/api/v3/"
	// auditLogPrefixV4Forward is the forward package's v4 administrator API
	// (/api/v4/forward/*, forward-sdk.md F5a). Its bodies are routes and
	// node records, which carry no credential, and are kept redacted like
	// v2's.
	auditLogPrefixV4Forward = "/api/v4/forward/"
	// auditLogPrefixV4Admin is the console's administrator API owned by the
	// kernel (/api/v4/admin/*): its writes are audited like the /api/v2
	// admin routes'. A bulk request carries ids and an action, no credential.
	auditLogPrefixV4Admin = "/api/v4/admin/"
	maxBodyLogLength      = 512  // bytes to include in slog output
	maxBodyDBLength       = 4096 // bytes to persist in database
)

// auditBodyCapture copies at most limit bytes while the downstream handler
// consumes the original request stream. Unlike reading a limited prefix and
// replacing Body, it never truncates the payload seen by a JSON handler.
type auditBodyCapture struct {
	io.ReadCloser
	buf   bytes.Buffer
	limit int
}

func (r *auditBodyCapture) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	remaining := r.limit - r.buf.Len()
	if n > 0 && remaining > 0 {
		captured := n
		if captured > remaining {
			captured = remaining
		}
		_, _ = r.buf.Write(p[:captured])
	}
	return n, err
}

// AuditActionKey is the gin context key under which a handler names the
// action of its audit row ("bulk_ban"), instead of the one the method and
// path give.
const AuditActionKey = "audit_action"

// AuditLog returns a gin middleware that records all admin API operations
// to both structured slog output and the v2_audit_log database table.
// It applies to requests whose path starts with /api/v2/admin/, /api/v3/ or
// /api/v4/forward/, and to the user writes in auditedUserWrites.
func AuditLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		// The v3 control kernel is administrator-only at the router boundary.
		// Keep v2's narrower admin prefix so public and user APIs remain out of
		// the audit stream.
		userWrite := auditedUserWrite(c.Request.Method, c.Request.URL.Path)
		if !userWrite && !strings.HasPrefix(c.Request.URL.Path, auditLogPrefixV2) && !strings.HasPrefix(c.Request.URL.Path, auditLogPrefixV3) &&
			!strings.HasPrefix(c.Request.URL.Path, auditLogPrefixV4Forward) && !strings.HasPrefix(c.Request.URL.Path, auditLogPrefixV4Admin) {
			c.Next()
			return
		}

		// Skip in test mode to avoid database race conditions
		if gin.Mode() == gin.TestMode {
			c.Next()
			return
		}

		// Only log write operations (POST, PUT, DELETE), and the reads that
		// reveal a secret.
		method := c.Request.Method
		if method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete &&
			!auditedRead(method, c.Request.URL.Path) {
			c.Next()
			return
		}

		// A forward route preview stores nothing and the route editor sends
		// one a second after each pause in typing (F5b D4, D13): it is logged
		// at debug level and never written to the audit table.
		if auditExempt(method, c.Request.URL.Path) {
			c.Next()
			slog.Debug("admin audit exempt", slog.String("method", method), slog.String("path", c.Request.URL.Path),
				slog.Int("status_code", c.Writer.Status()), slog.String("user_id", userIDToString(c.Value("user_id"))))
			return
		}

		// Kernel payloads can contain large declarative configurations. Preserve
		// their body exactly and never persist their contents in audit records;
		// secrets must only be referenced by ID in v3 contracts. An audited
		// user write's body is its re-authentication (a password or a
		// one-time code), so it is not kept either.
		isV3 := strings.HasPrefix(c.Request.URL.Path, auditLogPrefixV3)
		var capture *auditBodyCapture
		if !isV3 && !userWrite && c.Request.Body != nil && c.Request.ContentLength != 0 {
			capture = &auditBodyCapture{ReadCloser: c.Request.Body, limit: auditCaptureLimit}
			c.Request.Body = capture
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

		// Admin bodies carry passwords, gateway keys and tokens: only the
		// redacted form reaches the log and the audit table.
		var reqBody string
		if capture != nil {
			reqBody = redactAuditBody(capture.buf.Bytes(), c.Request.URL.Path)
		}

		duration := time.Since(startTime).Milliseconds()
		statusCode := c.Writer.Status()

		// Derive module and action from the path; a handler that did more
		// than the method says (a bulk action) names its action.
		module, action := extractModuleAndAction(c.Request.URL.Path, method)
		if named, ok := c.Get(AuditActionKey); ok {
			if name, ok := named.(string); ok && name != "" {
				action = name
			}
		}

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

		// Capture the current database handle before starting the goroutine. This
		// prevents asynchronous audit persistence from racing with test or shutdown
		// code that replaces the process-wide database handle.
		if db := database.GetDB(); db != nil {
			go persistAuditLog(db, uidPtr, emailStr(email), method, c.Request.URL.Path, module, action, clientIP, userAgent, requestID, reqBody, statusCode, duration, errMsg)
		}
	}
}

// AuditItem is one of the things a single request did: the single-item
// request it stands for.
type AuditItem struct {
	Method string
	Path   string
	// Status is the HTTP status the single-item request would have had.
	Status int
	// Error is why it failed; empty when it did not.
	Error string
}

// RecordAuditItems writes an audit row for each item, as the audit log would
// have written it had the item been requested alone: the item's method and
// path (so module and action follow the same rules), with the acting
// administrator, address, user agent and request id of c, which ties the rows
// to the request that did them. A bulk request is audited as a whole by the
// middleware; this keeps each target findable by its own path. It is
// synchronous, a single batch insert, and best effort like the middleware's
// row: an error is logged, not returned.
func RecordAuditItems(c *gin.Context, items []AuditItem) {
	db := database.GetDB()
	if db == nil || len(items) == 0 {
		return
	}
	userID, _ := c.Get("user_id")
	email, _ := c.Get("email")
	var actor *uint
	if id, ok := userID.(uint); ok {
		actor = &id
	}
	requestID := c.Writer.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = c.GetHeader("X-Request-ID")
	}
	rows := make([]model.AuditLog, 0, len(items))
	for _, item := range items {
		module, action := extractModuleAndAction(item.Path, item.Method)
		rows = append(rows, model.AuditLog{
			UserID: actor, Email: emailStr(email), Method: item.Method, Path: item.Path, Module: module, Action: action,
			IP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"), RequestID: requestID,
			StatusCode: item.Status, ErrorMessage: truncate(item.Error, maxBodyDBLength),
		})
	}
	if err := db.CreateInBatches(&rows, 100).Error; err != nil {
		slog.Warn("admin audit items were not recorded", slog.Int("items", len(items)), slog.String("error", err.Error()))
	}
}

// persistAuditLog writes an audit record to the database.
// Runs in a goroutine so it never blocks the request flow.
func persistAuditLog(db *gorm.DB, userID *uint, email, method, path, module, action, ip, userAgent, requestID, reqBody string, statusCode int, durationMs int64, errMsg string) {
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

// revealPath matches the administrator reads that answer a secret in
// clear, one item at a time: a proxy node's API key and shared secret
// (GET /api/v2/admin/nodes/:id/credentials). Every other answer masks node
// secrets, so these reads are recorded like writes, as "reveal".
var revealPath = regexp.MustCompile(`^/api/v2/admin/nodes/[^/]+/credentials/?$`)

// auditedUserWrites are the user API writes recorded in the audit log like
// the administrator's: a user's reset of their own subscription link, the
// self-service twin of POST /api/v2/admin/users/:id/reset-subscribe.
var auditedUserWrites = map[string]struct{ module, action string }{
	http.MethodPost + " /api/v2/user/subscription/reset": {module: "user", action: "reset_subscribe"},
}

// auditedUserWrite reports whether a user API request is recorded in the
// audit log.
func auditedUserWrite(method, path string) bool {
	_, ok := auditedUserWrites[method+" "+strings.TrimSuffix(path, "/")]
	return ok
}

// auditExemptWrites are writes under an audited prefix that change
// nothing: the forward route preview plans a route without storing it.
var auditExemptWrites = map[string]struct{}{
	http.MethodPost + " " + auditLogPrefixV4Forward + "routes/preview": {},
}

// auditExempt reports whether a write under an audited prefix is left out
// of the audit log.
func auditExempt(method, path string) bool {
	_, ok := auditExemptWrites[method+" "+strings.TrimSuffix(path, "/")]
	return ok
}

// auditedRead reports whether a read is recorded in the audit log: one
// that reveals a secret.
func auditedRead(method, path string) bool {
	return method == http.MethodGet && revealPath.MatchString(path)
}

// extractModuleAndAction derives a human-readable module and action from the URL path and HTTP method.
func extractModuleAndAction(path, method string) (module, action string) {
	if write, ok := auditedUserWrites[method+" "+strings.TrimSuffix(path, "/")]; ok {
		return write.module, write.action
	}
	// Strip the relevant administrator API prefix.
	prefix := auditLogPrefixV2
	switch {
	case strings.HasPrefix(path, auditLogPrefixV3):
		prefix = auditLogPrefixV3
	case strings.HasPrefix(path, auditLogPrefixV4Forward):
		// The module is "forward".
		prefix = "/api/v4/"
	case strings.HasPrefix(path, auditLogPrefixV4Admin):
		prefix = auditLogPrefixV4Admin
	}
	trimmed := strings.TrimPrefix(path, prefix)
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
		if auditedRead(method, path) {
			action = "reveal"
		}
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
