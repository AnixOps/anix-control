package middleware

import (
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
)

// NodeSecureLogger 节点专用安全日志
// 记录节点通信详情，用于审计和问题排查
func NodeSecureLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 处理请求
		c.Next()

		latency := time.Since(start)

		// 获取节点信息
		nodeID, _ := c.Get("node_id")

		logEntry := utils.NewLogSafe().
			SetRaw("type", "node_api").
			SetRaw("method", c.Request.Method).
			SetRaw("path", c.Request.URL.Path).
			SetRaw("status", c.Writer.Status()).
			SetRaw("latency_ms", latency.Milliseconds()).
			SetRaw("node_id", nodeID).
			SetIP("node_ip", c.ClientIP())

		// 检查是否有签名
		if sig := c.GetHeader(SignatureHeader); sig != "" {
			logEntry.SetRaw("signed", true)
		} else {
			logEntry.SetRaw("signed", false)
		}

		log.Printf("[NODE] %s", logEntry.String())
	}
}
