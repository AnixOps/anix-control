package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	// SignatureHeader 签名头
	SignatureHeader = "X-Signature"
	// TimestampHeader 时间戳头
	TimestampHeader = "X-Timestamp"
	// NonceHeader 随机数头 (防重放)
	NonceHeader = "X-Nonce"
	// MaxTimeDiff 最大时间差 (秒)
	MaxTimeDiff = 300 // 5分钟
)

// SignatureAuth 请求签名验证中间件
// 签名算法: HMAC-SHA256(timestamp + method + path + body, secret)
// 需要配合 NodeAPIKeyAuth 使用，依赖 context 中的 node 信息
func SignatureAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取签名相关头部
		signature := c.GetHeader(SignatureHeader)
		timestampStr := c.GetHeader(TimestampHeader)
		nonce := c.GetHeader(NonceHeader)

		// 如果没有签名头，跳过验证（向后兼容）
		// 生产环境可以改为强制验证
		if signature == "" {
			c.Next()
			return
		}

		// 验证时间戳
		if timestampStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"message": "缺少时间戳",
				"code":    "MISSING_TIMESTAMP",
			})
			return
		}

		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"message": "时间戳格式错误",
				"code":    "INVALID_TIMESTAMP",
			})
			return
		}

		// 检查时间差
		now := time.Now().Unix()
		diff := now - timestamp
		if diff < 0 {
			diff = -diff
		}
		if diff > MaxTimeDiff {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"message": "请求已过期",
				"code":    "REQUEST_EXPIRED",
			})
			return
		}

		// 获取节点信息（需要先通过 NodeAPIKeyAuth）
		nodeInterface, exists := c.Get("node")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "未授权",
				"code":    "UNAUTHORIZED",
			})
			return
		}
		node := nodeInterface.(*model.Node)

		// 读取请求体
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			// 重新设置请求体，供后续处理使用
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 构建签名字符串: timestamp + method + path + body
		signData := timestampStr + c.Request.Method + c.Request.URL.Path + string(bodyBytes)

		// 计算期望签名
		expectedSig := calculateHMAC(signData, node.Secret)

		// 验证签名 (使用常量时间比较防止时序攻击)
		if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "签名验证失败",
				"code":    "INVALID_SIGNATURE",
			})
			return
		}

		// 防重放攻击：检查 nonce（可选，需要 Redis 支持）
		if nonce != "" {
			if isNonceUsed(nonce) {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"message": "请求已处理",
					"code":    "DUPLICATE_REQUEST",
				})
				return
			}
			markNonceUsed(nonce)
		}

		c.Next()
	}
}

// calculateHMAC 计算 HMAC-SHA256
func calculateHMAC(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// nonce 缓存 (简单实现，生产环境建议用 Redis)
var nonceCache = make(map[string]int64)

// isNonceUsed 检查 nonce 是否已使用
func isNonceUsed(nonce string) bool {
	expireAt, exists := nonceCache[nonce]
	if !exists {
		return false
	}
	// 已过期的 nonce 视为未使用
	if time.Now().Unix() > expireAt {
		delete(nonceCache, nonce)
		return false
	}
	return true
}

// markNonceUsed 标记 nonce 已使用
func markNonceUsed(nonce string) {
	// 清理过期的 nonce
	now := time.Now().Unix()
	for k, v := range nonceCache {
		if now > v {
			delete(nonceCache, k)
		}
	}
	// 添加新的 nonce，有效期与 MaxTimeDiff 相同
	nonceCache[nonce] = now + MaxTimeDiff
}

// StrictSignatureAuth 严格签名验证（必须有签名）
func StrictSignatureAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		signature := c.GetHeader(SignatureHeader)
		if signature == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "缺少请求签名",
				"code":    "MISSING_SIGNATURE",
			})
			return
		}

		// 继续到 SignatureAuth 处理
		SignatureAuth()(c)
	}
}

// VerifyNodeSignature 验证节点签名（独立函数，用于特殊场景）
func VerifyNodeSignature(apiKey, timestamp, method, path, body, signature string) bool {
	// 查找节点
	keyHash := sha256Hash(apiKey)
	var node model.Node
	db := database.GetDB()
	if err := db.Where("api_key_hash = ?", keyHash).First(&node).Error; err != nil {
		return false
	}

	// 构建签名字符串
	signData := timestamp + method + path + body

	// 计算期望签名
	expectedSig := calculateHMAC(signData, node.Secret)

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}
