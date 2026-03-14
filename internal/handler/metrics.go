package handler

import (
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MetricsHandler Prometheus 指标处理器
type MetricsHandler struct {
	db *gorm.DB

	// 计数器
	requestCount    uint64
	errorCount      uint64
	successCount    uint64

	// 启动时间
	startTime time.Time
}

// NewMetricsHandler 创建指标处理器
func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{
		db:        database.Get(),
		startTime: time.Now(),
	}
}

// RecordRequest 记录请求
func (h *MetricsHandler) RecordRequest(success bool) {
	atomic.AddUint64(&h.requestCount, 1)
	if success {
		atomic.AddUint64(&h.successCount, 1)
	} else {
		atomic.AddUint64(&h.errorCount, 1)
	}
}

// GetMetrics godoc
// @Summary Prometheus 指标
// @Description 获取 Prometheus 格式的系统指标
// @Tags 系统
// @Produce plain
// @Success 200 {string} string "Prometheus metrics"
// @Router /metrics [get]
func (h *MetricsHandler) GetMetrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取数据库统计
	var userCount, nodeCount, orderCount int64
	h.db.Model(&User{}).Count(&userCount)
	h.db.Model(&Node{}).Count(&nodeCount)
	h.db.Model(&Order{}).Count(&orderCount)

	// 获取在线用户数
	var onlineUsers int64
	h.db.Model(&User{}).Where("t > 0").Count(&onlineUsers)

	// 构建指标
	metrics := `# HELP v2board_info Application information
# TYPE v2board_info gauge
v2board_info{version="2.0.0",go_version="` + runtime.Version() + `"} 1

# HELP v2board_uptime_seconds Application uptime in seconds
# TYPE v2board_uptime_seconds gauge
v2board_uptime_seconds ` + formatFloat(time.Since(h.startTime).Seconds()) + `

# HELP v2board_requests_total Total number of HTTP requests
# TYPE v2board_requests_total counter
v2board_requests_total ` + formatUint(atomic.LoadUint64(&h.requestCount)) + `

# HELP v2board_requests_success_total Total number of successful requests
# TYPE v2board_requests_success_total counter
v2board_requests_success_total ` + formatUint(atomic.LoadUint64(&h.successCount)) + `

# HELP v2board_requests_error_total Total number of error requests
# TYPE v2board_requests_error_total counter
v2board_requests_error_total ` + formatUint(atomic.LoadUint64(&h.errorCount)) + `

# HELP v2board_users_total Total number of users
# TYPE v2board_users_total gauge
v2board_users_total ` + formatInt(userCount) + `

# HELP v2board_nodes_total Total number of nodes
# TYPE v2board_nodes_total gauge
v2board_nodes_total ` + formatInt(nodeCount) + `

# HELP v2board_orders_total Total number of orders
# TYPE v2board_orders_total gauge
v2board_orders_total ` + formatInt(orderCount) + `

# HELP v2board_online_users Number of online users
# TYPE v2board_online_users gauge
v2board_online_users ` + formatInt(onlineUsers) + `

# HELP v2board_go_goroutines Number of goroutines
# TYPE v2board_go_goroutines gauge
v2board_go_goroutines ` + formatInt(int64(runtime.NumGoroutine())) + `

# HELP v2board_go_mem_alloc_bytes Number of bytes allocated in heap
# TYPE v2board_go_mem_alloc_bytes gauge
v2board_go_mem_alloc_bytes ` + formatUint(m.Alloc) + `

# HELP v2board_go_mem_sys_bytes Number of bytes obtained from system
# TYPE v2board_go_mem_sys_bytes gauge
v2board_go_mem_sys_bytes ` + formatUint(m.Sys) + `

# HELP v2board_go_mem_heap_objects Number of heap objects
# TYPE v2board_go_mem_heap_objects gauge
v2board_go_mem_heap_objects ` + formatUint(m.HeapObjects) + `

# HELP v2board_go_gc_duration_seconds GC duration in seconds
# TYPE v2board_go_gc_duration_seconds gauge
v2board_go_gc_duration_seconds ` + formatFloat(float64(m.PauseTotalNs)/1e9) + `
`

	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(metrics))
}

// 简化的模型定义（用于计数查询）
type User struct{}
type Node struct{}
type Order struct{}

func (User) TableName() string   { return "v2_user" }
func (Node) TableName() string   { return "v2_node" }
func (Order) TableName() string { return "v2_order" }

func formatInt(v int64) string {
	return formatUint(uint64(v))
}

func formatUint(v uint64) string {
	var buf [20]byte
	i := len(buf)
	for v >= 10 {
		i--
		buf[i] = byte(v%10) + '0'
		v /= 10
	}
	i--
	buf[i] = byte(v) + '0'
	return string(buf[i:])
}

func formatFloat(v float64) string {
	// 简单的浮点数格式化，保留2位小数
	intPart := int64(v)
	fracPart := int64((v - float64(intPart)) * 100)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	return formatInt(intPart) + "." + formatInt(fracPart)
}