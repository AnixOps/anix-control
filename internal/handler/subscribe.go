package handler

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// SubscribeHandler 订阅处理器
type SubscribeHandler struct {
	subscriptionService *service.SubscriptionService
	cfg                 *config.Config
}

// NewSubscribeHandler 创建订阅处理器
func NewSubscribeHandler(cfg *config.Config) *SubscribeHandler {
	return &SubscribeHandler{
		subscriptionService: service.NewSubscriptionService(),
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

	return fmt.Sprintf("%s://%s%s", scheme, c.Request.Host, c.Request.URL.RequestURI())
}

func (h *SubscribeHandler) buildSubscribeDomain(c *gin.Context) string {
	host := c.Request.Host
	if forwardedHost := c.GetHeader("X-Forwarded-Host"); forwardedHost != "" {
		host = strings.TrimSpace(strings.Split(forwardedHost, ",")[0])
	}

	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}

	// IPv6 host without port can appear as [::1]
	return strings.Trim(host, "[]")
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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取分组失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": groups})
}

// CreateGroup 创建订阅分组
// POST /api/v2/admin/subscription/groups
func (h *SubscriptionAdminHandler) CreateGroup(c *gin.Context) {
	var group model.SubscriptionGroup
	if err := c.ShouldBindJSON(&group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if err := h.subscriptionService.CreateGroup(&group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建分组失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "data": group})
}

// GetGroup 获取订阅分组详情
// GET /api/v2/admin/subscription/groups/:id
func (h *SubscriptionAdminHandler) GetGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	group, err := h.subscriptionService.GetGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "分组不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": group})
}

// UpdateGroup 更新订阅分组
// PUT /api/v2/admin/subscription/groups/:id
func (h *SubscriptionAdminHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	var group model.SubscriptionGroup
	if err := c.ShouldBindJSON(&group); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	group.ID = uint(id)

	if err := h.subscriptionService.UpdateGroup(&group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新分组失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "data": group})
}

// DeleteGroup 删除订阅分组
// DELETE /api/v2/admin/subscription/groups/:id
func (h *SubscriptionAdminHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	if err := h.subscriptionService.DeleteGroup(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除分组失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetTemplates 获取分组的模板列表
// GET /api/v2/admin/subscription/groups/:id/templates
func (h *SubscriptionAdminHandler) GetTemplates(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	templates, err := h.subscriptionService.GetTemplatesByGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取模板失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// GetGroupProtocols 获取属于该分组的物理节点协议
// GET /api/v2/admin/subscription/groups/:id/protocols
func (h *SubscriptionAdminHandler) GetGroupProtocols(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	nodeService := service.NewNodeService()
	protocols, err := nodeService.GetProtocolsByGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取协议失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": protocols})
}

// UpdateGroupProtocols 更新分组关联的物理节点协议
// POST /api/v2/admin/subscription/groups/:id/protocols
func (h *SubscriptionAdminHandler) UpdateGroupProtocols(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	var req struct {
		ProtocolIDs []uint `json:"protocol_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	nodeService := service.NewNodeService()
	if err := nodeService.AssignProtocolsToGroup(uint(id), req.ProtocolIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// GetAvailableProtocols 获取所有可用的物理节点协议 (Protocol Pool)
// GET /api/v2/admin/subscription/protocols/available
func (h *SubscriptionAdminHandler) GetAvailableProtocols(c *gin.Context) {
	nodeService := service.NewNodeService()
	protocols, err := nodeService.GetAllAvailableProtocols()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": protocols})
}

// CreateTemplate 创建订阅模板
// POST /api/v2/admin/subscription/groups/:id/templates
func (h *SubscriptionAdminHandler) CreateTemplate(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "分组 ID 无效"})
		return
	}

	var template model.SubscriptionTemplate
	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	template.GroupID = uint(groupID)

	if err := h.subscriptionService.CreateTemplate(&template); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "创建模板失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "创建成功", "data": template})
}

// GetTemplate 获取订阅模板详情
// GET /api/v2/admin/subscription/templates/:id
func (h *SubscriptionAdminHandler) GetTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	template, err := h.subscriptionService.GetTemplate(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "模板不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": template})
}

// UpdateTemplate 更新订阅模板
// PUT /api/v2/admin/subscription/templates/:id
func (h *SubscriptionAdminHandler) UpdateTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误"})
		return
	}

	normalizeTemplateUpdatePayload(updates)

	if err := h.subscriptionService.UpdateTemplateFields(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新模板失败", "error": err.Error()})
		return
	}

	template, err := h.subscriptionService.GetTemplate(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "更新成功但读取失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "data": template})
}

func normalizeTemplateUpdatePayload(updates map[string]interface{}) {
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
		c.JSON(http.StatusBadRequest, gin.H{"message": "ID 无效"})
		return
	}

	if err := h.subscriptionService.DeleteTemplate(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "删除模板失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// AssignGroupToUser 为用户分配订阅分组
// POST /api/v2/admin/subscription/users/:user_id/groups
func (h *SubscriptionAdminHandler) AssignGroupToUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户 ID 无效"})
		return
	}

	var req struct {
		GroupID        uint   `json:"group_id" binding:"required"`
		ExpireAt       *int64 `json:"expire_at"`
		TransferEnable *int64 `json:"transfer_enable"`  // bytes
		NextRenewPrice *int64 `json:"next_renew_price"` // 单位分
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if err := h.subscriptionService.AssignGroupToUser(uint(userID), req.GroupID, req.ExpireAt, req.TransferEnable, req.NextRenewPrice); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "分配失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "分配成功"})
}

// RemoveGroupFromUser 移除用户的订阅分组
// DELETE /api/v2/admin/subscription/users/:user_id/groups/:group_id
func (h *SubscriptionAdminHandler) RemoveGroupFromUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户 ID 无效"})
		return
	}

	groupID, err := strconv.ParseUint(c.Param("group_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "分组 ID 无效"})
		return
	}

	if err := h.subscriptionService.RemoveGroupFromUser(uint(userID), uint(groupID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "移除失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "移除成功"})
}

// GetUserGroups 获取用户的订阅分组
// GET /api/v2/admin/subscription/users/:user_id/groups
func (h *SubscriptionAdminHandler) GetUserGroups(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户 ID 无效"})
		return
	}

	groups, err := h.subscriptionService.GetUserGroups(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": groups})
}

// AssignGroupToPlan 为套餐分配订阅分组
// POST /api/v2/admin/subscription/plans/:plan_id/groups
func (h *SubscriptionAdminHandler) AssignGroupToPlan(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("plan_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐 ID 无效"})
		return
	}

	var req struct {
		GroupID uint `json:"group_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	if err := h.subscriptionService.AssignGroupToPlan(uint(planID), req.GroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "分配失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "分配成功"})
}

// RemoveGroupFromPlan 移除套餐的订阅分组
// DELETE /api/v2/admin/subscription/plans/:plan_id/groups/:group_id
func (h *SubscriptionAdminHandler) RemoveGroupFromPlan(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("plan_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐 ID 无效"})
		return
	}

	groupID, err := strconv.ParseUint(c.Param("group_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "分组 ID 无效"})
		return
	}

	if err := h.subscriptionService.RemoveGroupFromPlan(uint(planID), uint(groupID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "移除失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "移除成功"})
}

// GetPlanGroups 获取套餐的订阅分组
// GET /api/v2/admin/subscription/plans/:plan_id/groups
func (h *SubscriptionAdminHandler) GetPlanGroups(c *gin.Context) {
	planID, err := strconv.ParseUint(c.Param("plan_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "套餐 ID 无效"})
		return
	}

	groups, err := h.subscriptionService.GetPlanGroups(uint(planID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": groups})
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

	c.JSON(http.StatusOK, gin.H{"data": formats})
}

// GetProtocolTypes 获取支持的协议类型
// GET /api/v2/admin/subscription/protocols
func (h *SubscriptionAdminHandler) GetProtocolTypes(c *gin.Context) {
	protocols := []map[string]interface{}{
		{"id": "vmess", "name": "VMess", "description": "V2Ray VMess 协议", "supports_tls": true, "supports_reality": false},
		{"id": "vless", "name": "VLESS", "description": "V2Ray VLESS 协议", "supports_tls": true, "supports_reality": true},
		{"id": "trojan", "name": "Trojan", "description": "Trojan 协议", "supports_tls": true, "supports_reality": false},
		{"id": "shadowsocks", "name": "Shadowsocks", "description": "Shadowsocks 协议", "supports_tls": false, "supports_reality": false},
		{"id": "hysteria2", "name": "Hysteria2", "description": "Hysteria2 协议", "supports_tls": true, "supports_reality": false},
		{"id": "tuic", "name": "TUIC", "description": "TUIC 协议", "supports_tls": true, "supports_reality": false},
	}

	c.JSON(http.StatusOK, gin.H{"data": protocols})
}

// PreviewSubscription 预览订阅内容
// POST /api/v2/admin/subscription/preview
func (h *SubscriptionAdminHandler) PreviewSubscription(c *gin.Context) {
	var req struct {
		UserID uint                     `json:"user_id"`
		Format model.SubscriptionFormat `json:"format"`
		Groups []uint                   `json:"groups"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "参数错误", "error": err.Error()})
		return
	}

	// 获取用户
	userService := service.NewUserService()
	user, err := userService.GetByID(req.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
		return
	}

	// 构建请求
	subReq := &model.SubscriptionRequest{
		Token:  user.Token,
		Format: req.Format,
		Groups: req.Groups,
	}

	// 获取订阅
	resp, err := h.subscriptionService.GetUserSubscription(subReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "获取订阅失败", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"content":       resp.Content,
			"content_type":  resp.ContentType,
			"filename":      resp.Filename,
			"used_traffic":  resp.UsedTraffic,
			"total_traffic": resp.TotalTraffic,
		},
	})
}
