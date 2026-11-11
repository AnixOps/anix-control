package handler

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/branding"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
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
//   - type: 指定输出格式 (v2ray, clash, stash, egern, surge, loon, shadowrocket, quantumultx, json, base64json, wireguard, iwan, anixops)
//     也支持 type=auto 或 type=ua，强制按 User-Agent 自动识别格式
//   - include: 包含节点名关键词 (正则)
//   - exclude: 排除节点名关键词 (正则)
//   - groups: 指定分组 ID (逗号分隔)
func (h *SubscribeHandler) GetSubscription(c *gin.Context) {
	if !h.servesSubscriptionsHere(c) {
		c.String(http.StatusNotFound, "404 page not found")
		return
	}
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
			case "wg":
				formatStr = "wireguard"
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
	c.Header("Profile-Title", branding.ControlName+" Subscription")

	// 返回订阅内容
	c.String(http.StatusOK, resp.Content)
}

// buildSubscribeURL is the subscription link handed back in client configs.
// The host is a configured subscription domain (app.subscribe_domains) when
// there is one; otherwise, as the scheme, it comes from the request, where
// X-Forwarded-Proto/Host count only from a trusted reverse proxy
// (server.trusted_proxies). Without a valid host it is "" and the formatter
// uses its placeholder.
func (h *SubscribeHandler) buildSubscribeURL(c *gin.Context) string {
	origin := requestorigin.Resolve(c.Request)
	host := h.resolveSubscribeHost(origin.Host)
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s%s", origin.Scheme, host, c.Request.URL.RequestURI())
}

func (h *SubscribeHandler) buildSubscribeDomain(c *gin.Context) string {
	host := h.resolveSubscribeHost(requestorigin.Resolve(c.Request).Host)

	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}

	// IPv6 host without port can appear as [::1]
	return strings.Trim(host, "[]")
}

// servesSubscriptionsHere reports whether this request's host may answer a
// subscription. With app.subscribe_domains_only on, only the configured
// subscription domains do; every other host (the panel's domain, a bare IP)
// answers 404 as if the path did not exist, so it never shows up in what
// clients fetch.
func (h *SubscribeHandler) servesSubscriptionsHere(c *gin.Context) bool {
	settings := service.GetSubscriptionSettings(h.configService, h.cfg)
	if !settings.DomainsOnly || len(settings.SubscribeDomains) == 0 {
		return true
	}
	host := normalizeSubscriptionRequestHost(requestorigin.Resolve(c.Request).Host)
	for _, configured := range settings.SubscribeDomains {
		if normalizeSubscriptionRequestHost(configured) == host {
			return true
		}
	}
	return false
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
		`wireguard`:    "wireguard",
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
		// AnixOps Net: links plus Panabit iWAN lines (iwan://, via).
		`anixopsnet`: "anixops",
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
