package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DestinationStatsHandler serves each user's traffic by destination and
// category (internal/destinationstats, docs/reference/destination-stats.md):
// to administrators for any user or node, with the settings, and to each
// user for their own traffic.
type DestinationStatsHandler struct {
	db  func() *gorm.DB
	now func() time.Time
}

// NewDestinationStatsHandler reads the kernel database.
func NewDestinationStatsHandler() *DestinationStatsHandler {
	return &DestinationStatsHandler{db: database.Get, now: time.Now}
}

// DestinationStatsAnswer is a summary with the settings it was recorded
// under.
type DestinationStatsAnswer struct {
	destinationstats.Summary
	Settings destinationstats.Settings `json:"settings"`
}

// queryInt reads an optional non-negative integer parameter.
func queryInt(c *gin.Context, name string, max int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > max {
		kernelError(c, http.StatusBadRequest, "invalid_request", name+" must be an integer from 0 to "+strconv.Itoa(max))
		return 0, false
	}
	return value, true
}

// summarize answers the summary of the last hours for userID (0: all) and
// nodeID (0: all).
func (h *DestinationStatsHandler) summarize(c *gin.Context, userID, nodeID uint) {
	hours, ok := queryInt(c, "hours", destinationstats.MaxHours)
	if !ok {
		return
	}
	limit, ok := queryInt(c, "limit", destinationstats.MaxLimit)
	if !ok {
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	db = db.WithContext(c.Request.Context())
	settings, err := destinationstats.LoadSettings(db)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	query := destinationstats.LastHours(hours, h.now())
	query.UserID, query.NodeID, query.Limit = userID, nodeID, limit
	summary, err := destinationstats.Summarize(db, query)
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, DestinationStatsAnswer{Summary: summary, Settings: settings})
}

// AdminSummary godoc
// @Summary 访问统计 (按目标域名与流量类型)
// @Description 最近 hours 小时 (默认 24, 最多 4320) 各目标域名与流量类型 (视频、音乐、通讯、社交、游戏、AI、下载、网站、未知) 的上下行字节与连接数, 来自节点 Agent 的 destinations.v1 上报, 按 UTC 小时汇总。user_id、node_id 为 0 或省略时统计全部用户或节点。domains 按字节从大到小, 最多 limit 个 (默认 50, 最多 500), domain_count 为总数; 空域名是超出 Agent 每窗口上限的其余流量。类型按当前规则判定。settings 为统计开关与保留天数。字节为原始字节, 不乘节点倍率。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Param hours query int false "小时数, 1 到 4320"
// @Param user_id query int false "用户 ID"
// @Param node_id query int false "节点 ID"
// @Param limit query int false "域名条数, 1 到 500"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/traffic/destinations [get]
func (h *DestinationStatsHandler) AdminSummary(c *gin.Context) {
	userID, ok := queryInt(c, "user_id", 1<<31-1)
	if !ok {
		return
	}
	nodeID, ok := queryInt(c, "node_id", 1<<31-1)
	if !ok {
		return
	}
	h.summarize(c, uint(userID), uint(nodeID))
}

// UserSummary godoc
// @Summary 我的访问统计
// @Description 当前用户最近 hours 小时 (默认 24, 最多 4320) 各目标域名与流量类型的上下行字节与连接数, 字段同 GET /api/v4/admin/traffic/destinations。settings.enabled 为 false 时节点不记录新的统计。
// @Tags 用户端
// @Produce json
// @Security BearerAuth
// @Param hours query int false "小时数, 1 到 4320"
// @Param limit query int false "域名条数, 1 到 500"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/user/traffic/destinations [get]
func (h *DestinationStatsHandler) UserSummary(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		kernelError(c, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	h.summarize(c, userID, 0)
}

// GetSettings godoc
// @Summary 访问统计设置
// @Description 访问统计的开关 (默认关闭) 与保留天数 (默认 30, 7 到 180)。关闭时节点上报的统计被丢弃。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/destinations/settings [get]
func (h *DestinationStatsHandler) GetSettings(c *gin.Context) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	settings, err := destinationstats.LoadSettings(db.WithContext(c.Request.Context()))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, settings)
}

// PutSettings godoc
// @Summary 修改访问统计设置
// @Description 设置访问统计的开关与保留天数 (7 到 180)。开关立即生效; 超出保留天数的记录在下一次清理 (每小时) 时删除。
// @Tags 管理端-流量
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body destinationstats.Settings true "设置"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/traffic/destinations/settings [put]
func (h *DestinationStatsHandler) PutSettings(c *gin.Context) {
	var body struct {
		Enabled       *bool `json:"enabled"`
		RetentionDays *int  `json:"retention_days"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil || body.RetentionDays == nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", "enabled and retention_days are required")
		return
	}
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	settings := destinationstats.Settings{Enabled: *body.Enabled, RetentionDays: *body.RetentionDays}
	if err := destinationstats.SaveSettings(db.WithContext(c.Request.Context()), settings); err != nil {
		if errors.Is(err, destinationstats.ErrRetentionDays) {
			kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		kernelDBError(c, err)
		return
	}
	kernelData(c, http.StatusOK, settings)
}
