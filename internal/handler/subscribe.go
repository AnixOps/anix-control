package handler

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SubscribeHandler 订阅处理器
type SubscribeHandler struct {
	subscriptionService *service.SubscriptionService
	configService       *service.SystemConfigService
	cfg                 *config.Config
}

// NewSubscribeHandler 创建订阅处理器
func NewSubscribeHandler(cfg *config.Config) *SubscribeHandler {
	return &SubscribeHandler{
		subscriptionService: service.NewSubscriptionService(),
		configService:       service.NewSystemConfigService(database.Get()),
		cfg:                 cfg,
	}
}

// GetSubscription 获取用户订阅
// GET /s/:token
// 支持 URL 参数:
//   - type: 指定输出格式 (v2ray, clash, stash, egern, surge, loon, shadowrocket, quantumultx, json, base64json)
//     也支持 type=auto 或 type=ua，强制按 User-Agent 自动识别格式
//   - include: 包含节点名关键词 (正则)
//   - exclude: 排除节点名关键词 (正则)
//   - groups: 指定分组 ID (逗号分隔)
func (h *SubscribeHandler) GetSubscription(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.String(http.StatusBadRequest, "invalid token")
		return
	}

	// 支持从扩展名检测格式 (例如 /s/TOKEN.yaml)
	ext := ""
	if lastDot := strings.LastIndex(token, "."); lastDot != -1 {
		ext = token[lastDot+1:]
		token = token[:lastDot] // 去掉扩展名的才是真正的 token
	}

	// 检测输出格式
	formatStr := strings.ToLower(strings.TrimSpace(c.Query("type")))
	forceUA := formatStr == "auto" || formatStr == "ua"

	if forceUA {
		formatStr = h.detectFormatFromUserAgent(c.GetHeader("User-Agent"))
	} else if formatStr == "" {
		if ext != "" {
			// 根据后缀映射
			switch strings.ToLower(ext) {
			case "yaml", "yml":
				formatStr = "clash"
			case "conf":
				formatStr = "surge"
			case "json":
				formatStr = "sing-box"
			case "txt":
				formatStr = "v2ray"
			}
		}

		if formatStr == "" {
			// 根据 User-Agent 自动检测
			formatStr = h.detectFormatFromUserAgent(c.GetHeader("User-Agent"))
		}
	}

	format := model.SubscriptionFormat(formatStr)

	// 解析分组 ID
	var groups []uint
	if groupsStr := c.Query("groups"); groupsStr != "" {
		for _, s := range strings.Split(groupsStr, ",") {
			if id, err := strconv.ParseUint(s, 10, 32); err == nil {
				groups = append(groups, uint(id))
			}
		}
	}

	// 构建请求
	subscribeURL := h.buildSubscribeURL(c)
	subscribeDomain := h.buildSubscribeDomain(c)
	req := &model.SubscriptionRequest{
		Token:           token,
		Format:          format,
		Groups:          groups,
		Include:         c.Query("include"),
		Exclude:         c.Query("exclude"),
		SubscribeURL:    subscribeURL,
		SubscribeDomain: subscribeDomain,
	}

	// 获取订阅
	resp, err := h.subscriptionService.GetUserSubscription(req)
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}

	// 设置响应头
	c.Header("Content-Type", resp.ContentType)
	c.Header("Content-Disposition", "attachment; filename="+resp.Filename)
	c.Header("Subscription-Userinfo", h.buildUserInfo(resp))
	c.Header("Profile-Update-Interval", "24") // 24 小时更新间隔
	c.Header("Profile-Title", "V2Board Subscription")

	// 返回订阅内容
	c.String(http.StatusOK, resp.Content)
}

// GetLegacySubscription keeps compatibility with the historical
// /api/v1/client/subscribe?token=TOKEN entrypoint.
func (h *SubscribeHandler) GetLegacySubscription(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.String(http.StatusBadRequest, "invalid token")
		return
	}

	c.Params = append(c.Params, gin.Param{Key: "token", Value: token})
	h.GetSubscription(c)
}

func (h *SubscribeHandler) buildSubscribeURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	if forwardedProto := c.GetHeader("X-Forwarded-Proto"); forwardedProto != "" {
		first := strings.TrimSpace(strings.Split(forwardedProto, ",")[0])
		if first != "" {
			scheme = strings.ToLower(first)
		}
	}

	host := c.Request.Host
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
		host = strings.TrimSpace(strings.Split(forwardedHost, ",")[0])
	}

	host = h.resolveSubscribeHost(host)
	return fmt.Sprintf("%s://%s%s", scheme, host, c.Request.URL.RequestURI())
}

func (h *SubscribeHandler) buildSubscribeDomain(c *gin.Context) string {
	host := c.Request.Host
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
		host = strings.TrimSpace(strings.Split(forwardedHost, ",")[0])
	}

	host = h.resolveSubscribeHost(host)

	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}

	// IPv6 host without port can appear as [::1]
	return strings.Trim(host, "[]")
}

func (h *SubscribeHandler) resolveSubscribeHost(requestHost string) string {
	settings := service.GetSubscriptionSettings(h.configService, h.cfg)
	requestHost = strings.TrimSpace(requestHost)
	if len(settings.SubscribeDomains) == 0 {
		return requestHost
	}

	normalized := normalizeSubscriptionRequestHost(requestHost)
	for _, configured := range settings.SubscribeDomains {
		if normalizeSubscriptionRequestHost(configured) == normalized {
			return configured
		}
	}

	return settings.SubscribeDomains[0]
}

func normalizeSubscriptionRequestHost(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(strings.Trim(value, "[]")); err == nil {
		return host
	}
	return strings.Trim(value, "[]")
}

// detectFormatFromUserAgent 根据 User-Agent 检测输出格式
func (h *SubscribeHandler) detectFormatFromUserAgent(ua string) string {
	ua = strings.ToLower(ua)

	patterns := map[string]string{
		`clash`:        "clash",
		`stash`:        "stash",
		`egern`:        "egern",
		`surge`:        "surge",
		`loon`:         "loon",
		`shadowrocket`: "shadowrocket",
		`quantumult`:   "quantumultx",
		`v2rayng`:      "v2ray",
		`v2rayn`:       "v2ray",
		`v2box`:        "v2ray",
		`nekoray`:      "v2ray",
		`sing-box`:     "sing-box",
		`s-box`:        "sing-box",
		`nekobox`:      "sing-box",
	}

	for pattern, format := range patterns {
		if regexp.MustCompile(pattern).MatchString(ua) {
			return format
		}
	}

	return "v2ray" // 默认 V2Ray Base64
}

// buildUserInfo 构建用户信息响应头 (Clash 订阅格式)
func (h *SubscribeHandler) buildUserInfo(resp *model.SubscriptionResponse) string {
	// 格式: upload=xxx; download=xxx; total=xxx; expire=xxx
	parts := []string{
		"upload=0",
		"download=" + strconv.FormatInt(resp.UsedTraffic, 10),
		"total=" + strconv.FormatInt(resp.TotalTraffic, 10),
	}

	if resp.ExpireAt > 0 {
		parts = append(parts, "expire="+strconv.FormatInt(resp.ExpireAt, 10))
	}

	return strings.Join(parts, "; ")
}

// ============ 管理员接口 ============

// SubscriptionAdminHandler 订阅管理处理器
type SubscriptionAdminHandler struct {
	subscriptionService *service.SubscriptionService
}

// NewSubscriptionAdminHandler 创建订阅管理处理器
func NewSubscriptionAdminHandler() *SubscriptionAdminHandler {
	return &SubscriptionAdminHandler{
		subscriptionService: service.NewSubscriptionService(),
	}
}

// GetGroups 获取所有订阅分组
// GET /api/v2/admin/subscription/groups
func (h *SubscriptionAdminHandler) GetGroups(c *gin.Context) {
	groups, err := h.subscriptionService.GetGroups()
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, groups)
}

// CreateGroup 创建订阅分组
// POST /api/v2/admin/subscription/groups
func (h *SubscriptionAdminHandler) CreateGroup(c *gin.Context) {
	var group model.SubscriptionGroup
	if err := c.ShouldBindJSON(&group); err != nil {
		panelError(c, err.Error())
		return
	}

	if err := h.subscriptionService.CreateGroup(&group); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, group)
}

// GetGroup 获取订阅分组详情
// GET /api/v2/admin/subscription/groups/:id
func (h *SubscriptionAdminHandler) GetGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	group, err := h.subscriptionService.GetGroup(uint(id))
	if err != nil {
		panelError(c, "分组不存在")
		return
	}

	panelSuccess(c, group)
}

// UpdateGroup 更新订阅分组
// PUT /api/v2/admin/subscription/groups/:id
func (h *SubscriptionAdminHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	var group model.SubscriptionGroup
	if err := c.ShouldBindJSON(&group); err != nil {
		panelError(c, err.Error())
		return
	}

	group.ID = uint(id)

	if err := h.subscriptionService.UpdateGroup(&group); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, group)
}

// DeleteGroup 删除订阅分组
// DELETE /api/v2/admin/subscription/groups/:id
func (h *SubscriptionAdminHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	if err := h.subscriptionService.DeleteGroup(uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "分组不存在")
			return
		}
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// GetTemplates 获取分组的模板列表
// GET /api/v2/admin/subscription/groups/:id/templates
func (h *SubscriptionAdminHandler) GetTemplates(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	templates, err := h.subscriptionService.GetTemplatesByGroup(uint(id))
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, templates)
}

// GetGroupProtocols 获取属于该分组的物理节点协议
// GET /api/v2/admin/subscription/groups/:id/protocols
func (h *SubscriptionAdminHandler) GetGroupProtocols(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	nodeService := service.NewNodeService()
	protocols, err := nodeService.GetProtocolsByGroup(uint(id))
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, protocols)
}

// UpdateGroupProtocols 更新分组关联的物理节点协议
// POST /api/v2/admin/subscription/groups/:id/protocols
func (h *SubscriptionAdminHandler) UpdateGroupProtocols(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	var req struct {
		ProtocolIDs []uint `json:"protocol_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	nodeService := service.NewNodeService()
	if err := nodeService.AssignProtocolsToGroup(uint(id), req.ProtocolIDs); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			panelError(c, "分组不存在")
		case errors.Is(err, service.ErrNodeProtocolNotFound):
			panelError(c, "协议不存在")
		default:
			panelError(c, err.Error())
		}
		return
	}

	panelSuccess(c, gin.H{"message": "更新成功"})
}

// GetAvailableProtocols 获取所有可用的物理节点协议 (Protocol Pool)
// GET /api/v2/admin/subscription/protocols/available
func (h *SubscriptionAdminHandler) GetAvailableProtocols(c *gin.Context) {
	nodeService := service.NewNodeService()
	protocols, err := nodeService.GetAllAvailableProtocols()
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, protocols)
}

// CreateTemplate 创建订阅模板
// POST /api/v2/admin/subscription/groups/:id/templates
func (h *SubscriptionAdminHandler) CreateTemplate(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "分组 ID 无效")
		return
	}

	var template model.SubscriptionTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		panelError(c, err.Error())
		return
	}

	template.GroupID = uint(groupID)

	if err := h.subscriptionService.CreateTemplate(&template); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, template)
}

// GetTemplate 获取订阅模板详情
// GET /api/v2/admin/subscription/templates/:id
func (h *SubscriptionAdminHandler) GetTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	template, err := h.subscriptionService.GetTemplate(uint(id))
	if err != nil {
		panelError(c, "模板不存在")
		return
	}

	panelSuccess(c, template)
}

// UpdateTemplate 更新订阅模板
// PUT /api/v2/admin/subscription/templates/:id
func (h *SubscriptionAdminHandler) UpdateTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		panelError(c, err.Error())
		return
	}

	if len(updates) == 0 {
		panelError(c, "参数错误")
		return
	}

	normalizeTemplateUpdatePayload(updates)

	if err := h.subscriptionService.UpdateTemplateFields(uint(id), updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "模板不存在")
			return
		}
		panelError(c, err.Error())
		return
	}

	template, err := h.subscriptionService.GetTemplate(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "模板不存在")
			return
		}
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, template)
}

func normalizeTemplateUpdatePayload(updates map[string]any) {
	intFields := []string{"group_id", "port", "tls", "enable", "sort"}
	for _, field := range intFields {
		val, exists := updates[field]
		if !exists {
			continue
		}
		switch v := val.(type) {
		case float64:
			updates[field] = int(v)
		case float32:
			updates[field] = int(v)
		case int64:
			updates[field] = int(v)
		case int32:
			updates[field] = int(v)
		case bool:
			if v {
				updates[field] = 1
			} else {
				updates[field] = 0
			}
		}
	}
}

// DeleteTemplate 删除订阅模板
// DELETE /api/v2/admin/subscription/templates/:id
func (h *SubscriptionAdminHandler) DeleteTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		panelError(c, "ID 无效")
		return
	}

	if err := h.subscriptionService.DeleteTemplate(uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			panelError(c, "模板不存在")
			return
		}
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, gin.H{"message": "删除成功"})
}

// AssignGroupToUser 为用户分配订阅分组
// POST /api/v2/admin/subscription/users/:user_id/groups
func (h *SubscriptionAdminHandler) AssignGroupToUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		panelError(c, "用户 ID 无效")
		return
	}

	var req struct {
		GroupID        uint   `json:"group_id" binding:"required"`
		ExpireAt       *int64 `json:"expire_at"`
		TransferEnable *int64 `json:"transfer_enable"`  // bytes
		NextRenewPrice *int64 `json:"next_renew_price"` // 单位分
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	if err := h.subscriptionService.AssignGroupToUser(uint(userID), req.GroupID, req.ExpireAt, req.TransferEnable, req.NextRenewPrice); err != nil {
		panelSubscriptionBindingError(c, "分配失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "分配成功"})
}

// RemoveGroupFromUser 移除用户的订阅分组
// DELETE /api/v2/admin/subscription/users/:user_id/groups/:group_id
func (h *SubscriptionAdminHandler) RemoveGroupFromUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		panelError(c, "用户 ID 无效")
		return
	}

	groupID, err := strconv.ParseUint(c.Param("group_id"), 10, 32)
	if err != nil {
		panelError(c, "分组 ID 无效")
		return
	}

	if err := h.subscriptionService.RemoveGroupFromUser(uint(userID), uint(groupID)); err != nil {
		panelSubscriptionBindingError(c, "移除失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "移除成功"})
}

// GetUserGroups 获取用户的订阅分组
// GET /api/v2/admin/subscription/users/:user_id/groups
func (h *SubscriptionAdminHandler) GetUserGroups(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		panelError(c, "用户 ID 无效")
		return
	}

	groups, err := h.subscriptionService.GetUserGroups(uint(userID))
	if err != nil {
		panelSubscriptionBindingError(c, "获取失败", err)
		return
	}

	panelSuccess(c, groups)
}

// AssignGroupToPlan 为套餐分配订阅分组
// POST /api/v2/admin/subscription/plans/:plan_id/groups
func (h *SubscriptionAdminHandler) AssignGroupToPlan(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("plan_id"), 10, 32)
	if err != nil {
		panelError(c, "套餐 ID 无效")
		return
	}

	var req struct {
		GroupID uint `json:"group_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, "参数错误: "+err.Error())
		return
	}

	if err := h.subscriptionService.AssignGroupToPlan(uint(planID), req.GroupID); err != nil {
		panelSubscriptionBindingError(c, "分配失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "分配成功"})
}

// RemoveGroupFromPlan 移除套餐的订阅分组
// DELETE /api/v2/admin/subscription/plans/:plan_id/groups/:group_id
func (h *SubscriptionAdminHandler) RemoveGroupFromPlan(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("plan_id"), 10, 32)
	if err != nil {
		panelError(c, "套餐 ID 无效")
		return
	}

	groupID, err := strconv.ParseUint(c.Param("group_id"), 10, 32)
	if err != nil {
		panelError(c, "分组 ID 无效")
		return
	}

	if err := h.subscriptionService.RemoveGroupFromPlan(uint(planID), uint(groupID)); err != nil {
		panelSubscriptionBindingError(c, "移除失败", err)
		return
	}

	panelSuccess(c, gin.H{"message": "移除成功"})
}

// GetPlanGroups 获取套餐的订阅分组
// GET /api/v2/admin/subscription/plans/:plan_id/groups
func (h *SubscriptionAdminHandler) GetPlanGroups(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("plan_id"), 10, 32)
	if err != nil {
		panelError(c, "套餐 ID 无效")
		return
	}

	groups, err := h.subscriptionService.GetPlanGroups(uint(planID))
	if err != nil {
		panelSubscriptionBindingError(c, "获取失败", err)
		return
	}

	panelSuccess(c, groups)
}

func panelSubscriptionBindingError(c *gin.Context, fallback string, err error) {
	switch {
	case errors.Is(err, service.ErrSubscriptionUserNotFound),
		errors.Is(err, service.ErrSubscriptionPlanNotFound),
		errors.Is(err, service.ErrSubscriptionGroupNotFound),
		errors.Is(err, service.ErrSubscriptionUserGroupNotFound),
		errors.Is(err, service.ErrSubscriptionPlanGroupNotFound):
		panelError(c, err.Error())
	default:
		panelError(c, fallback+": "+err.Error())
	}
}

// GetSubscriptionFormats 获取支持的订阅格式
// GET /api/v2/admin/subscription/formats
func (h *SubscriptionAdminHandler) GetSubscriptionFormats(c *gin.Context) {
	formats := []map[string]string{
		{"id": "auto", "name": "Auto (UA)", "description": "根据客户端 User-Agent 自动识别订阅格式"},
		{"id": "v2ray", "name": "V2Ray Base64", "description": "适用于 V2RayN, V2RayNG, V2Box 等"},
		{"id": "clash", "name": "Clash YAML", "description": "适用于 Clash, ClashX 等"},
		{"id": "stash", "name": "Stash YAML", "description": "适用于 Stash（Clash YAML 兼容）"},
		{"id": "egern", "name": "Egern YAML", "description": "适用于 Egern（Clash YAML 兼容）"},
		{"id": "surge", "name": "Surge", "description": "适用于 Surge iOS/macOS"},
		{"id": "loon", "name": "Loon", "description": "适用于 Loon iOS"},
		{"id": "shadowrocket", "name": "Shadowrocket", "description": "适用于 Shadowrocket iOS"},
		{"id": "quantumultx", "name": "Quantumult X", "description": "适用于 Quantumult X iOS"},
		{"id": "json", "name": "JSON", "description": "原始 JSON 格式"},
		{"id": "base64json", "name": "Base64 JSON", "description": "Base64 编码的分组 JSON 格式"},
		{"id": "sing-box", "name": "Sing-box", "description": "适用于 Sing-box, Nekobox 等"},
	}

	panelSuccess(c, formats)
}

// GetProtocolTypes 获取支持的协议类型
// GET /api/v2/admin/subscription/protocols
func (h *SubscriptionAdminHandler) GetProtocolTypes(c *gin.Context) {
	protocols := []map[string]any{
		{"id": "vmess", "name": "VMess", "description": "V2Ray VMess 协议", "supports_tls": true, "supports_reality": false},
		{"id": "vless", "name": "VLESS", "description": "V2Ray VLESS 协议", "supports_tls": true, "supports_reality": true},
		{"id": "trojan", "name": "Trojan", "description": "Trojan 协议", "supports_tls": true, "supports_reality": false},
		{"id": "shadowsocks", "name": "Shadowsocks", "description": "Shadowsocks 协议", "supports_tls": false, "supports_reality": false},
		{"id": "hysteria2", "name": "Hysteria2", "description": "Hysteria2 协议", "supports_tls": true, "supports_reality": false},
		{"id": "tuic", "name": "TUIC", "description": "TUIC 协议", "supports_tls": true, "supports_reality": false},
	}

	panelSuccess(c, protocols)
}

// GetGroupStats 获取订阅分组统计数据
// GET /api/v2/admin/subscription/stats
func (h *SubscriptionAdminHandler) GetGroupStats(c *gin.Context) {
	stats, err := h.subscriptionService.GetGroupStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取统计失败", "error": err.Error()})
		return
	}
	panelSuccess(c, stats)
}

// PreviewSubscription 预览订阅内容
// POST /api/v2/admin/subscription/preview
func (h *SubscriptionAdminHandler) PreviewSubscription(c *gin.Context) {
	var req struct {
		UserID   uint                     `json:"user_id"`
		Format   model.SubscriptionFormat `json:"format" binding:"omitempty"`
		Groups   []uint                   `json:"groups"`
		GroupIDs []uint                   `json:"group_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	userID := req.UserID
	if userID == 0 {
		var ok bool
		userID, ok = currentPanelUserID(c)
		if !ok {
			return
		}
	}

	groups := req.Groups
	if len(groups) == 0 {
		groups = req.GroupIDs
	}

	// 获取用户
	userService := service.NewUserService()
	user, err := userService.GetByID(userID)
	if err != nil {
		panelError(c, "用户不存在")
		return
	}

	// 构建请求
	subReq := &model.SubscriptionRequest{
		Token:  user.Token,
		Format: req.Format,
		Groups: groups,
	}

	// 获取订阅
	resp, err := h.subscriptionService.GetUserSubscription(subReq)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, gin.H{
		"content":       resp.Content,
		"content_type":  resp.ContentType,
		"filename":      resp.Filename,
		"used_traffic":  resp.UsedTraffic,
		"total_traffic": resp.TotalTraffic,
	})
}
