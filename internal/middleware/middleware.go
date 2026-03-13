package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/utils"
	"github.com/gin-gonic/gin"
)

// NodeAuth 节点认证中间件 (旧版兼容)
func NodeAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")

		// 写入调试文件
		debugFile, _ := os.OpenFile("C:/tmp/middleware.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if debugFile != nil {
			debugFile.WriteString(fmt.Sprintf("NodeAuth: token=%s, path=%s\n", token, c.Request.URL.Path))
			debugFile.Close()
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing token",
			})
			return
		}

		cfg := config.Get()
		if cfg.App.APIToken != "" && token != cfg.App.APIToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			return
		}

		c.Next()
	}
}

// NodeAPIKeyAuth 节点 API Key 认证中间件 (新版)
func NodeAPIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Header 或 Query 获取 API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "缺少 API Key",
			})
			return
		}

		// 通过 API Key Hash 查找节点
		keyHash := sha256Hash(apiKey)
		var node model.Node
		db := database.GetDB()
		if err := db.Where("api_key_hash = ?", keyHash).First(&node).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "API Key 无效",
			})
			return
		}

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
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "未登录或登录已过期",
			})
			return
		}

		// 支持 "Bearer token" 和 直接 "token" 两种格式
		token := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}

		claims, err := utils.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "登录已过期，请重新登录",
			})
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("is_admin", claims.IsAdmin)

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

// Logger 日志中间件
func Logger() gin.HandlerFunc {
	return gin.Logger()
}

// Recovery 恢复中间件
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
