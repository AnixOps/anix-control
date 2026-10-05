package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/AnixOps/anix-control/v4/internal/adminapitoken"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/logging"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

// NodeAuth enforces node-scoped auth via required `node_id + X-API-Key`.
func NodeAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}
		if apiKey == "" {
			apiKey = c.Query("token")
		}
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing api key",
			})
			return
		}

		nodeIDStr := c.Query("node_id")
		if nodeIDStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing node_id",
			})
			return
		}

		nodeID, err := strconv.ParseUint(nodeIDStr, 10, 32)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "invalid node_id",
			})
			return
		}

		var node model.Node
		db := database.GetDB()
		if err := db.First(&node, uint(nodeID)).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid node",
			})
			return
		}

		// The key is checked through the node credential split, which
		// applies the table's phase and never accepts a tombstone.
		if !nodesecrets.NodeAPIKeyMatches(db, &node, apiKey) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid api key",
			})
			return
		}

		// A node an administrator disabled gets no configuration and no
		// users, as on the /node API and the gRPC listener. Its polling is
		// refused before the heartbeat, so it is not recorded as seen either.
		if node.Status == model.NodeStatusDisabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "node disabled",
			})
			return
		}

		c.Set("node_id", node.ID)
		c.Set("node", &node)

		// 刷新节点心跳: 所有 UniProxy 轮询请求 (config/user/push/alive) 都经过本中间件,
		// 在此统一更新 last_check_at, 避免节点带 node_type 时心跳不更新导致误判离线。
		// 心跳不会把管理员禁用的节点改回在线 (禁用节点已在上面被拒绝)。
		db.Model(&model.Node{}).Where("id = ?", node.ID).Updates(map[string]any{
			"last_check_at": time.Now().Unix(),
			"status":        service.NodeHeartbeatStatus(),
		})

		c.Next()
	}
}

// NodeAPIKeyAuth 节点 API Key 认证中间件 (新版)
func NodeAPIKeyAuth() gin.HandlerFunc {
	return nodeAPIKeyAuth(true)
}

// NodeAPIKeyHeaderAuth is the package-download authentication boundary. API
// keys in URLs leak through logs, browser history and proxy caches, so this
// variant accepts only X-API-Key while preserving legacy query fallback on
// the older node APIs.
func NodeAPIKeyHeaderAuth() gin.HandlerFunc {
	return nodeAPIKeyAuth(false)
}

func nodeAPIKeyAuth(allowQuery bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Header 或 Query 获取 API Key
		apiKey := c.GetHeader("X-API-Key")
		if allowQuery && apiKey == "" {
			apiKey = c.Query("api_key")
		}
		if allowQuery && apiKey == "" {
			apiKey = c.Query("token")
		}
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "缺少 API Key",
			})
			return
		}

		// The node is looked up by its key's hash, or, for an old row without
		// a hash, by the key itself, through the node credential split.
		found, err := nodesecrets.NodeByAPIKey(database.GetDB(), apiKey, true)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "API Key 无效",
			})
			return
		}
		node := *found

		// 检查节点状态
		if node.Status == model.NodeStatusDisabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "节点已被禁用",
			})
			return
		}

		// 将节点信息存入上下文
		c.Set("node_id", node.ID)
		c.Set("node", &node)

		c.Next()
	}
}

// sha256Hash 计算 SHA256 哈希
func sha256Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

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

		// WebSocket 握手无法自定义 header, 允许从 query 参数 token 回退获取
		if authHeader == "" {
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

// CORS 跨域中间件
func AppTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		expected := ""
		if cfg != nil {
			expected = strings.TrimSpace(cfg.App.APIToken)
		}
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"message": "internal api token is not configured",
			})
			return
		}

		token := strings.TrimSpace(c.Query("secret"))
		if token == "" {
			token = strings.TrimSpace(c.GetHeader("X-API-Key"))
		}
		if token == "" {
			authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = strings.TrimSpace(authHeader[7:])
			} else {
				token = authHeader
			}
		}
		if token == "" || token != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "invalid internal api token",
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
	return gin.Logger()
}

// Recovery 恢复中间件
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
