package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/utils"
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

		tokenHash := sha256Hash(apiKey)
		valid := node.APIKeyHash == tokenHash || (node.APIKeyHash == "" && node.APIKey == apiKey)
		if !valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid api key",
			})
			return
		}

		c.Set("node_id", node.ID)
		c.Set("node", &node)

		// 刷新节点心跳: 所有 UniProxy 轮询请求 (config/user/push/alive) 都经过本中间件,
		// 在此统一更新 last_check_at, 避免节点带 node_type 时心跳不更新导致误判离线
		db.Model(&model.Node{}).Where("id = ?", node.ID).Updates(map[string]any{
			"last_check_at": time.Now().Unix(),
			"status":        model.NodeStatusOnline,
		})

		c.Next()
	}
}

// NodeAPIKeyAuth 鑺傜偣 API Key 璁よ瘉涓棿浠?(鏂扮増)
func NodeAPIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 浠?Header 鎴?Query 鑾峰彇 API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}
		if apiKey == "" {
			apiKey = c.Query("token")
		}
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "缂哄皯 API Key",
			})
			return
		}

		// 閫氳繃 API Key Hash 鏌ユ壘鑺傜偣锛屽吋瀹规棫鏁版嵁鍥炶惤鍒?api_key 鏄庢枃鍖归厤
		keyHash := sha256Hash(apiKey)
		var node model.Node
		db := database.GetDB()
		if err := db.Where("api_key_hash = ?", keyHash).First(&node).Error; err != nil {
			if err := db.Where("api_key = ?", apiKey).First(&node).Error; err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"message": "API Key 鏃犳晥",
				})
				return
			}
		}

		// 妫€鏌ヨ妭鐐圭姸鎬?
		if node.Status == model.NodeStatusDisabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "鑺傜偣宸茶绂佺敤",
			})
			return
		}

		// 灏嗚妭鐐逛俊鎭瓨鍏ヤ笂涓嬫枃
		c.Set("node_id", node.ID)
		c.Set("node", &node)

		c.Next()
	}
}

// sha256Hash 璁＄畻 SHA256 鍝堝笇
func sha256Hash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// JWTAuth JWT 璁よ瘉涓棿浠?
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		// 鏀寔 "Bearer token" 鍜?鐩存帴 "token" 涓ょ鏍煎紡
		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		claims, err := utils.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "session expired, please login again",
			})
			return
		}

		// 灏嗙敤鎴蜂俊鎭瓨鍏ヤ笂涓嬫枃
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("is_admin", claims.IsAdmin)

		c.Next()
	}
}

// AdminAuth 绠＄悊鍛樿璇佷腑闂翠欢
func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, exists := c.Get("is_admin")
		if !exists || isAdmin != true {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"message": "鏉冮檺涓嶈冻",
			})
			return
		}

		c.Next()
	}
}

// CORS 璺ㄥ煙涓棿浠?
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
	exposeHeaders := []string{"ETag", "X-Request-ID"}
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

// Logger 鏃ュ織涓棿浠?
func Logger() gin.HandlerFunc {
	return gin.Logger()
}

// Recovery 鎭㈠涓棿浠?
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
