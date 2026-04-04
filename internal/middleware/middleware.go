package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/utils"
	"github.com/gin-gonic/gin"
)

// NodeAuth 鑺傜偣璁よ瘉涓棿浠?(鏃х増鍏煎)
func NodeAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing token",
			})
			return
		}

		cfg := config.Get()

		// 1) Legacy global token
		if cfg != nil && cfg.App.APIToken != "" && token == cfg.App.APIToken {
			c.Next()
			return
		}

		// 2) Node-scoped API key token (V2bX uses token + node_id)
		nodeIDStr := c.Query("node_id")
		if nodeIDStr != "" {
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

			tokenHash := sha256Hash(token)
			valid := node.APIKeyHash == tokenHash || (node.APIKeyHash == "" && node.APIKey == token)
			if !valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "invalid token",
				})
				return
			}

			c.Set("node_id", node.ID)
			c.Set("node", &node)
			c.Next()
			return
		}

		// 3) Keep compatibility for tests/dev when APIToken is empty.
		if cfg == nil || cfg.App.APIToken == "" {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "invalid token",
		})
		return
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
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Response-Format, If-None-Match, X-API-Key, X-Signature, X-Timestamp, X-Nonce")
		c.Header("Access-Control-Expose-Headers", "ETag")

		if c.Request.Method == "OPTIONS" {
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
