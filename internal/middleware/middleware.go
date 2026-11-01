package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v5/internal/authn"
	"github.com/AnixOps/anix-control/v5/internal/logging"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/gin-gonic/gin"
)

// JWTAuth JWT 认证中间件
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// AdminAPIToken, in front of this middleware on the administrator
		// APIs, already authenticated an admin API token; the context key is
		// set on the server side only. A JWT is checked as always, and an
		// API token on any other route is no JWT and fails below.
		if c.GetString(adminapitoken.ContextKeyAuthMethod) == adminapitoken.AuthMethodAPIToken {
			c.Next()
			return
		}
		authHeader := c.GetHeader("Authorization")

		// WebSocket 握手无法自定义 header, 只有 WebSocket 升级请求才允许
		// 从 query 参数 token 回退获取。普通请求不接受 URL 里的 JWT：URL 会
		// 进访问日志、浏览器历史和 Referer，凭据不该出现在那里。
		if authHeader == "" && isWebSocketUpgrade(c.Request) {
			if queryToken := c.Query("token"); queryToken != "" {
				authHeader = queryToken
			}
		}

		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "not logged in or session expired",
			})
			return
		}

		// 支持 "Bearer token" 和 直接 "token" 两种格式
		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		claims, err := authn.Default().Verify(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "session expired, please login again",
			})
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("is_admin", claims.IsAdmin)
		c.Set("session_id", claims.SessionID)
		if claims.ExpiresAt != nil {
			c.Set("token_expires_at", claims.ExpiresAt.Time)
		}
		if claims.IssuedAt != nil {
			c.Set("token_issued_at", claims.IssuedAt.Time)
		}

		c.Next()
	}
}

// AdminAuth 管理员认证中间件
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, exists := c.Get("is_admin")
		if !exists || isAdmin != true {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "权限不足",
			})
			return
		}

		c.Next()
	}
}

// CORS 跨域中间件 — 支持配置允许的源，默认仅允许配置列表中的源
func CORS() gin.HandlerFunc {
	cfg := config.Get()

	allowedOrigins := []string{}
	allowedMethods := []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"}
	allowedHeaders := []string{"Content-Type", "Authorization", "X-Response-Format", "If-None-Match", "X-API-Key", "X-Signature", "X-Timestamp", "X-Nonce", "X-Request-ID"}
	exposeHeaders := []string{
		"ETag", "X-Request-ID", "X-AnixOps-Operation-ID", "X-AnixOps-Operation-Chain",
	}
	allowCredentials := false
	maxAge := 86400 // 24 hours

	if cfg != nil {
		if len(cfg.Server.CORS.AllowedOrigins) > 0 {
			allowedOrigins = cfg.Server.CORS.AllowedOrigins
		}
		if len(cfg.Server.CORS.AllowedMethods) > 0 {
			allowedMethods = cfg.Server.CORS.AllowedMethods
		}
		if len(cfg.Server.CORS.AllowedHeaders) > 0 {
			allowedHeaders = cfg.Server.CORS.AllowedHeaders
		}
		allowCredentials = cfg.Server.CORS.AllowCredentials
		if cfg.Server.CORS.MaxAge > 0 {
			maxAge = cfg.Server.CORS.MaxAge
		}
	}

	allowAllOrigins := len(allowedOrigins) == 0

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		isPreflight := c.Request.Method == "OPTIONS"

		var allowedOrigin string
		matchedByWildcard := false
		if allowAllOrigins {
			// No origins configured: echo the request Origin back
			allowedOrigin = origin
			matchedByWildcard = true
		} else {
			// Match against the configured allowlist
			for _, o := range allowedOrigins {
				if o == "*" {
					allowedOrigin = origin
					matchedByWildcard = true
					break
				}
				if o == origin {
					allowedOrigin = origin
					break
				}
			}
		}

		// Reject requests with no Origin header or unlisted origin
		if origin == "" || allowedOrigin == "" {
			c.Header("Vary", "Origin")
			if isPreflight {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		// Never combine wildcard origin with credentials (RFC 6454 / CORS spec)
		reflectCredentials := allowCredentials && !matchedByWildcard

		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
		c.Header("Access-Control-Expose-Headers", strings.Join(exposeHeaders, ", "))
		c.Header("Vary", "Origin")

		if reflectCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if isPreflight {
			c.Header("Access-Control-Max-Age", strconv.Itoa(maxAge))
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Logger 日志中间件
func Logger() gin.HandlerFunc {
	if logging.JSON() {
		return logging.AccessLog()
	}
	// gin.Logger() 默认把完整查询串写进日志；节点 api_key/token、
	// WebSocket 的 JWT 和订阅 token 都可能在查询串里，所以文本日志只记路径，
	// 与 JSON 访问日志一致。
	return gin.LoggerWithFormatter(textAccessLogFormatter)
}

// textAccessLogFormatter 与 gin 的默认文本格式相同，但不带查询串。
func textAccessLogFormatter(param gin.LogFormatterParams) string {
	path := param.Path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if param.Latency > time.Minute {
		param.Latency = param.Latency.Truncate(time.Second)
	}
	return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		param.StatusCode,
		param.Latency,
		param.ClientIP,
		param.Method,
		path,
		param.ErrorMessage,
	)
}

// isWebSocketUpgrade 判断请求是不是 WebSocket 握手（RFC 6455 §4.1）。
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, part := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
			return true
		}
	}
	return false
}

// Recovery 恢复中间件
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
