package middleware

import (
	"bytes"
	"io"
	"log"
	"time"

	"github.com/anixops/v2board/internal/utils"
	"github.com/gin-gonic/gin"
)

// SecureLogger 安全日志中间件
// 自动脱敏敏感信息，记录请求详情
func SecureLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 读取请求体（用于日志，需要脱敏）
		var requestBody string
		if c.Request.Body != nil {
			bodyBytes, _ := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			// 脱敏处理
			requestBody = utils.RedactJSON(string(bodyBytes))
			// 限制长度
			if len(requestBody) > 1000 {
				requestBody = requestBody[:1000] + "...[truncated]"
			}
		}

		// 处理请求
		c.Next()

		// 计算耗时
		latency := time.Since(start)

		// 构建安全日志
		logEntry := utils.NewLogSafe().
			SetRaw("method", c.Request.Method).
			SetRaw("path", path).
			SetRaw("query", query).
			SetRaw("status", c.Writer.Status()).
			SetRaw("latency", latency.String()).
			SetIP("client_ip", c.ClientIP()).
			SetRaw("user_agent", c.Request.UserAgent())

		// 添加用户信息（如果已认证）
		if userID, exists := c.Get("user_id"); exists {
			logEntry.SetRaw("user_id", userID)
		}
		if nodeID, exists := c.Get("node_id"); exists {
			logEntry.SetRaw("node_id", nodeID)
		}

		// 记录错误
		if len(c.Errors) > 0 {
			logEntry.SetRaw("errors", c.Errors.String())
		}

		// 只在调试模式下记录请求体
		if gin.Mode() == gin.DebugMode && requestBody != "" {
			logEntry.SetRaw("body", requestBody)
		}

		log.Printf("[AUDIT] %s", logEntry.String())
	}
}

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

// AuditLog 审计日志结构
type AuditLog struct {
	Timestamp   time.Time              `json:"timestamp"`
	Action      string                 `json:"action"`
	UserID      uint                   `json:"user_id,omitempty"`
	NodeID      uint                   `json:"node_id,omitempty"`
	IP          string                 `json:"ip"`
	UserAgent   string                 `json:"user_agent"`
	Path        string                 `json:"path"`
	Method      string                 `json:"method"`
	StatusCode  int                    `json:"status_code"`
	Latency     time.Duration          `json:"latency"`
	RequestBody string                 `json:"request_body,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// WriteAuditLog 写入审计日志（可扩展为写入数据库或外部系统）
func WriteAuditLog(entry *AuditLog) {
	// 脱敏 IP
	entry.IP = utils.RedactIP(entry.IP)

	// 当前简单实现：写入标准日志
	// 可扩展为：写入数据库、发送到日志收集系统等
	logEntry := utils.NewLogSafe().
		SetRaw("timestamp", entry.Timestamp.Format(time.RFC3339)).
		SetRaw("action", entry.Action).
		SetRaw("user_id", entry.UserID).
		SetRaw("node_id", entry.NodeID).
		SetRaw("ip", entry.IP).
		SetRaw("path", entry.Path).
		SetRaw("method", entry.Method).
		SetRaw("status", entry.StatusCode).
		SetRaw("latency_ms", entry.Latency.Milliseconds())

	log.Printf("[AUDIT] %s", logEntry.String())
}
