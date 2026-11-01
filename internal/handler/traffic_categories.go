package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/destinationstats"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TrafficCategoriesHandler is the administrator API of the traffic
// category rules (internal/destinationstats): the built-in rules, the
// administrator's own rules and the remote lists, which the 访问统计
// categories are judged by. Every write is in the operation log (module
// traffic_category) and applies to the recorded history at once.
type TrafficCategoriesHandler struct {
	db    func() *gorm.DB
	fetch destinationstats.Fetcher
}

// NewTrafficCategoriesHandler reads the kernel database and fetches the
// sources over HTTP(S).
func NewTrafficCategoriesHandler() *TrafficCategoriesHandler {
	return &TrafficCategoriesHandler{db: database.Get, fetch: destinationstats.HTTPFetcher}
}

func (h *TrafficCategoriesHandler) service(c *gin.Context) (*destinationstats.Service, bool) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return nil, false
	}
	return &destinationstats.Service{DB: db}, true
}

func (h *TrafficCategoriesHandler) actor(c *gin.Context) destinationstats.Actor {
	return destinationstats.Actor{UserID: kernelActorID(c), Name: c.GetString("email"), IP: c.ClientIP()}
}

func (h *TrafficCategoriesHandler) id(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		kernelError(c, http.StatusBadRequest, "invalid_request", "invalid id")
		return 0, false
	}
	return uint(id), true
}

func (h *TrafficCategoriesHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, destinationstats.ErrNotFound):
		kernelError(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, destinationstats.ErrDuplicate):
		kernelError(c, http.StatusConflict, "duplicate", err.Error())
	case errors.Is(err, destinationstats.ErrRuleInvalid):
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		kernelDBError(c, err)
	}
}

// List godoc
// @Summary 流量分类规则
// @Description 访问统计判定流量类型的规则: categories 为类型 (video、music、messaging、social、gaming、ai、download、web、unknown); builtin 为内置规则 (按类型的 domains、networks、protocols); rules 为管理员规则 (kind 为 domain、network 或 protocol), 优先于远程规则源与内置规则, 指向 web 可把域名移出内置类型; sources 为远程规则源 (按 refresh_hours 定时拉取, entry_count 为最近一次成功拉取的条目数, fetch_error 为最近一次失败原因)。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories [get]
func (h *TrafficCategoriesHandler) List(c *gin.Context) {
	service, ok := h.service(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	rules, err := service.Rules(ctx)
	if err != nil {
		h.fail(c, err)
		return
	}
	sources, err := service.Sources(ctx)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	kernelData(c, http.StatusOK, gin.H{
		"categories": destinationstats.Categories,
		"builtin":    destinationstats.Builtin(),
		"rules":      rules,
		"sources":    sources,
	})
}

// Classify godoc
// @Summary 判定流量类型
// @Description 按当前规则 (管理员规则、远程规则源与内置规则) 判定一个目标的类型: domain 为域名、地址或网段, protocol 为嗅探到的协议 (可省略)。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Param domain query string false "域名、地址或网段"
// @Param protocol query string false "嗅探到的协议"
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/classify [get]
func (h *TrafficCategoriesHandler) Classify(c *gin.Context) {
	db := h.db()
	if db == nil {
		kernelError(c, http.StatusServiceUnavailable, "database_unavailable", "database is not initialized")
		return
	}
	classifier, err := destinationstats.CurrentClassifier(db.WithContext(c.Request.Context()))
	if err != nil {
		kernelDBError(c, err)
		return
	}
	domain, protocol := c.Query("domain"), c.Query("protocol")
	kernelData(c, http.StatusOK, gin.H{"domain": domain, "protocol": protocol, "category": classifier.Classify(domain, protocol)})
}

// CreateRule godoc
// @Summary 添加流量分类规则
// @Description 添加管理员规则: kind 为 domain (域名, 含子域名)、network (地址或 CIDR) 或 protocol (嗅探协议, 如 bittorrent); category 为 unknown 以外的类型; 同一 kind 与 value 只能有一条 (409 duplicate)。立即生效, 历史统计也按新规则判定。记入操作日志。
// @Tags 管理端-流量
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body destinationstats.RuleInput true "规则"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/rules [post]
func (h *TrafficCategoriesHandler) CreateRule(c *gin.Context) {
	var input destinationstats.RuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	rule, err := service.CreateRule(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusCreated, gin.H{"rule": rule})
}

// UpdateRule godoc
// @Summary 修改流量分类规则
// @Description 按添加时的检查替换规则的字段。记入操作日志。
// @Tags 管理端-流量
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则 ID"
// @Param body body destinationstats.RuleInput true "规则"
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/rules/{id} [put]
func (h *TrafficCategoriesHandler) UpdateRule(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var input destinationstats.RuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	rule, err := service.UpdateRule(c.Request.Context(), id, input, h.actor(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"rule": rule})
}

// DeleteRule godoc
// @Summary 删除流量分类规则
// @Description 删除管理员规则; 该目标回到远程规则源或内置规则的类型。记入操作日志。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则 ID"
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/rules/{id} [delete]
func (h *TrafficCategoriesHandler) DeleteRule(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	if err := service.DeleteRule(c.Request.Context(), id, h.actor(c)); err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"deleted": id})
}

// CreateSource godoc
// @Summary 添加远程规则源
// @Description 添加远程规则源: name 唯一; url 为 http(s) 地址, 内容可为纯域名/地址/CIDR 列表、v2fly domain-list-community (domain:、full:)、Clash 规则 (DOMAIN-SUFFIX、DOMAIN、IP-CIDR) 或 Clash rule-provider (payload); 列表中的域名与网段都归入 category; refresh_hours 为 1 到 168 (默认 24); enabled 默认 true。新规则源在下一次检查 (每 10 分钟) 时拉取, 或立即调用 refresh。记入操作日志。
// @Tags 管理端-流量
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body destinationstats.SourceInput true "规则源"
// @Success 201 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/sources [post]
func (h *TrafficCategoriesHandler) CreateSource(c *gin.Context) {
	var input destinationstats.SourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	source, err := service.CreateSource(c.Request.Context(), input, h.actor(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusCreated, gin.H{"source": source})
}

// UpdateSource godoc
// @Summary 修改远程规则源
// @Description 按添加时的检查替换规则源的设置; 改了 url 的规则源在下一次检查时重新拉取。记入操作日志。
// @Tags 管理端-流量
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则源 ID"
// @Param body body destinationstats.SourceInput true "规则源"
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/sources/{id} [put]
func (h *TrafficCategoriesHandler) UpdateSource(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var input destinationstats.SourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		kernelError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	source, err := service.UpdateSource(c.Request.Context(), id, input, h.actor(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"source": source})
}

// DeleteSource godoc
// @Summary 删除远程规则源
// @Description 删除规则源与它拉取的列表。记入操作日志。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则源 ID"
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/sources/{id} [delete]
func (h *TrafficCategoriesHandler) DeleteSource(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	if err := service.DeleteSource(c.Request.Context(), id, h.actor(c)); err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"deleted": id})
}

// RefreshSource godoc
// @Summary 立即拉取远程规则源
// @Description 立即拉取规则源的列表 (最长 1 分钟, 最大 16 MiB, 最多 200000 条); 失败时保留上一次的列表并在 fetch_error 中给出原因。
// @Tags 管理端-流量
// @Produce json
// @Security BearerAuth
// @Param id path int true "规则源 ID"
// @Success 200 {object} map[string]any
// @Router /api/v4/admin/traffic/categories/sources/{id}/refresh [post]
func (h *TrafficCategoriesHandler) RefreshSource(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	service, ok := h.service(c)
	if !ok {
		return
	}
	source, err := service.RefreshSource(c.Request.Context(), id, h.fetch)
	if err != nil {
		h.fail(c, err)
		return
	}
	kernelData(c, http.StatusOK, gin.H{"source": source})
}
