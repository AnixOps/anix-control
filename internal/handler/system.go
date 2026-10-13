package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
)

// SystemHandler 系统处理器
// SystemHandler handles system configuration and load balancer endpoints.
// The backups are /api/v5/backups (BackupsHandler).
type SystemHandler struct {
	configService       *service.SystemConfigService
	operationLogService *service.OperationLogService
}

// systemConfigResponse is a configuration entry as the administrator API
// answers it. A sensitive value (service.IsSensitiveSystemConfigKey) is
// never answered: value and display_value read
// service.SensitiveSystemConfigPlaceholder when one is stored, and
// has_value says whether one is. Writing the placeholder back keeps the
// stored value (SetConfig).
func systemConfigResponse(cfg *model.SystemConfig) gin.H {
	if cfg == nil {
		return gin.H{}
	}

	displayValue, sensitive, hasValue := service.MaskSystemConfigValue(cfg.Key, cfg.Value)

	return gin.H{
		"id":            cfg.ID,
		"key":           cfg.Key,
		"value":         displayValue,
		"display_value": displayValue,
		"sensitive":     sensitive,
		"has_value":     hasValue,
		"type":          cfg.Type,
		"group":         cfg.Group,
		"remark":        cfg.Remark,
		"description":   cfg.Remark,
		"created_at":    cfg.CreatedAt,
		"updated_at":    cfg.UpdatedAt,
	}
}

func contextUint(c *gin.Context, key string) *uint {
	if c == nil {
		return nil
	}

	raw, exists := c.Get(key)
	if !exists {
		return nil
	}

	switch value := raw.(type) {
	case uint:
		id := value
		return &id
	case *uint:
		return value
	case uint64:
		id := uint(value)
		return &id
	case uint32:
		id := uint(value)
		return &id
	case int:
		if value < 0 {
			return nil
		}
		id := uint(value)
		return &id
	case int64:
		if value < 0 {
			return nil
		}
		id := uint(value)
		return &id
	case float64:
		if value < 0 {
			return nil
		}
		id := uint(value)
		return &id
	default:
		return nil
	}
}

func contextString(c *gin.Context, key string) string {
	if c == nil {
		return ""
	}

	raw, exists := c.Get(key)
	if !exists {
		return ""
	}

	value, ok := raw.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(value)
}

// settingsAuditActor is the request's actor as the audit trail records it.
// A request the package bridge relays carries the actor's id only; its
// username is then the user's e-mail, as KernelSettings records it.
func settingsAuditActor(c *gin.Context) service.SettingsAuditActor {
	actor := service.SettingsAuditActor{
		UserID: contextUint(c, "user_id"), Username: contextString(c, "email"),
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	}
	if actor.Username == "" {
		actor.Username = service.AuditUsername(database.GetDB(), actor.UserID)
	}
	return actor
}

// recordSettingsAudit records the system configuration audit entry of a
// key a dedicated handler (the e-mail and invite configuration) wrote: the
// entry the system configuration handler and KernelSettings record.
func recordSettingsAudit(c *gin.Context, configs *service.SystemConfigService, operations *service.OperationLogService, key string, created, preserveExisting bool) {
	if configs == nil || operations == nil {
		return
	}
	saved, err := configs.GetEntry(key)
	if err != nil || saved == nil {
		log.Printf("record system config audit failed for key=%s: saved entry not found: %v", key, err)
		return
	}
	action := "update"
	if created {
		action = "create"
	}
	if err := operations.Record(service.SystemConfigAuditInput(settingsAuditActor(c), action, saved, preserveExisting)); err != nil {
		log.Printf("record system config audit failed for key=%s: %v", key, err)
	}
}

func (h *SystemHandler) recordSystemConfigAudit(c *gin.Context, action string, cfg *model.SystemConfig, preserveExisting bool) {
	if h == nil || h.operationLogService == nil || cfg == nil {
		return
	}

	if err := h.operationLogService.Record(service.SystemConfigAuditInput(settingsAuditActor(c), action, cfg, preserveExisting)); err != nil {
		log.Printf("record system config audit failed for key=%s: %v", cfg.Key, err)
	}
}

// NewSystemHandler 创建处理器
// NewSystemHandler creates a system handler.
func NewSystemHandler() *SystemHandler {
	db := database.Get()
	return &SystemHandler{
		configService:       service.NewSystemConfigService(db),
		operationLogService: service.NewOperationLogService(db),
	}
}

// GetSubscriptionSettings returns current subscription URL settings.
func (h *SystemHandler) GetSubscriptionSettings(c *gin.Context) {
	settings := service.GetSubscriptionSettings(h.configService, config.Get())
	panelSuccess(c, settings)
}

// ========== 系统配置 ==========

// GetConfigs godoc
// @Summary 获取系统配置列表
// @Description 管理员获取系统配置列表，支持按分组筛选
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param group query string false "配置分组"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/configs [get]
func (h *SystemHandler) GetConfigs(c *gin.Context) {
	group := c.Query("group")

	var configs []model.SystemConfig
	var err error

	if group != "" {
		configs, err = h.configService.GetByGroup(group)
	} else {
		configs, err = h.configService.GetAll()
	}

	if err != nil {
		panelError(c, err.Error())
		return
	}

	list := make([]gin.H, 0, len(configs))
	for i := range configs {
		list = append(list, systemConfigResponse(&configs[i]))
	}

	panelSuccess(c, gin.H{
		"list":  list,
		"total": len(list),
	})
}

// GetConfig godoc
// @Summary 获取单个系统配置
// @Description 管理员获取指定key的系统配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "配置键名"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/configs/{key} [get]
func (h *SystemHandler) GetConfig(c *gin.Context) {
	// A sensitive value is masked as in the list (systemConfigResponse);
	// the kernel's own readers, and packages holding a namespace's
	// KernelSettings secrets grant, read it in clear.
	key := c.Param("key")

	entry, err := h.configService.GetEntry(key)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	if entry == nil {
		displayValue, sensitive, hasValue := service.MaskSystemConfigValue(key, "")
		panelSuccess(c, gin.H{
			"key":           key,
			"value":         "",
			"display_value": displayValue,
			"sensitive":     sensitive,
			"has_value":     hasValue,
		})
		return
	}

	panelSuccess(c, systemConfigResponse(entry))
}

// SetConfig godoc
// @Summary 设置系统配置
// @Description 管理员设置系统配置项
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "配置键名"
// @Param request body map[string]any true "配置值"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/configs/{key} [put]
func (h *SystemHandler) SetConfig(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Value            json.RawMessage `json:"value"`
		Type             string          `json:"type" binding:"omitempty,oneof=string number boolean json bool int"`
		Group            string          `json:"group" binding:"omitempty,max=64"`
		Remark           string          `json:"remark" binding:"omitempty,max=255"`
		Description      string          `json:"description" binding:"omitempty,max=255"`
		PreserveExisting bool            `json:"preserve_existing"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	existingEntry, err := h.configService.GetEntry(key)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	rawValue := bytes.TrimSpace(req.Value)
	if len(rawValue) == 0 && existingEntry == nil {
		panelError(c, "value is required")
		return
	}

	value := ""
	if !bytes.Equal(rawValue, []byte("null")) {
		var stringValue string
		if err := json.Unmarshal(rawValue, &stringValue); err == nil {
			value = stringValue
		} else {
			value = string(rawValue)
		}
	}

	remark := req.Remark
	if remark == "" {
		remark = req.Description
	}

	preserveExisting := req.PreserveExisting
	sensitiveConfig := service.IsSensitiveSystemConfigKey(key)
	if existingEntry != nil {
		sensitiveConfig = sensitiveConfig || service.IsSensitiveSystemConfigKey(existingEntry.Key)
	}
	// The answers mask a sensitive value; writing the placeholder back
	// keeps the stored one, and never stores the placeholder itself.
	if existingEntry != nil && sensitiveConfig && (preserveExisting || value == service.SensitiveSystemConfigPlaceholder) {
		value = existingEntry.Value
		preserveExisting = true
	}
	if existingEntry == nil && sensitiveConfig && value == service.SensitiveSystemConfigPlaceholder {
		panelError(c, "value is required")
		return
	}
	if len(rawValue) == 0 && existingEntry != nil {
		if !preserveExisting {
			panelError(c, "value is required")
			return
		}
		value = existingEntry.Value
	}

	// A masked field sent back as the placeholder keeps its stored secret.
	storedValue := ""
	if existingEntry != nil {
		storedValue = existingEntry.Value
	}
	value, keptFields := service.KeepSystemConfigFields(key, value, storedValue)
	if keptFields {
		preserveExisting = true
	}

	if err := h.configService.Set(key, value, req.Type, req.Group, remark); err != nil {
		panelError(c, err.Error())
		return
	}

	savedEntry, err := h.configService.GetEntry(key)
	if err != nil {
		log.Printf("load saved system config failed: %v", err)
	}
	if savedEntry == nil {
		savedEntry = &model.SystemConfig{
			Key:    key,
			Value:  value,
			Type:   req.Type,
			Group:  req.Group,
			Remark: remark,
		}
		if existingEntry != nil {
			if savedEntry.Type == "" {
				savedEntry.Type = existingEntry.Type
			}
			if savedEntry.Group == "" {
				savedEntry.Group = existingEntry.Group
			}
			if savedEntry.Remark == "" {
				savedEntry.Remark = existingEntry.Remark
			}
		}
		if savedEntry.Type == "" {
			savedEntry.Type = "string"
		}
	}

	action := "update"
	if existingEntry == nil {
		action = "create"
	}
	h.recordSystemConfigAudit(c, action, savedEntry, preserveExisting)
	// The kernel's in-memory copies of the key's namespace reload.
	service.TouchSettingsKey(key)

	resp := systemConfigResponse(savedEntry)
	resp["message"] = "config updated"
	panelSuccess(c, resp)
}

// DeleteConfig godoc
// @Summary 删除系统配置
// @Description 管理员删除指定key的系统配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "配置键名"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/configs/{key} [delete]
func (h *SystemHandler) DeleteConfig(c *gin.Context) {
	key := c.Param("key")

	existingEntry, err := h.configService.GetEntry(key)
	if err != nil {
		panelError(c, err.Error())
		return
	}

	if err := h.configService.Delete(key); err != nil {
		panelError(c, err.Error())
		return
	}

	if existingEntry != nil {
		h.recordSystemConfigAudit(c, "delete", existingEntry, false)
	}
	service.TouchSettingsKey(key)

	panelSuccess(c, gin.H{"message": "config deleted"})
}

// ========== 负载均衡 ==========

// LoadBalancerHandler 负载均衡处理器
type LoadBalancerHandler struct {
	lbService *service.LoadBalancerService
}

func loadBalancerResponse(lb *model.LoadBalancer) gin.H {
	weights := any(map[string]any{})
	if lb.NodeWeights != "" {
		var decoded any
		if err := json.Unmarshal([]byte(lb.NodeWeights), &decoded); err == nil {
			weights = decoded
		}
	}

	return gin.H{
		"id":             lb.ID,
		"name":           lb.Name,
		"group_id":       lb.GroupID,
		"group_name":     "",
		"strategy":       lb.Strategy,
		"health_check":   lb.HealthCheck,
		"check_interval": lb.CheckInterval,
		"check_timeout":  lb.CheckTimeout,
		"enabled":        lb.Enabled,
		"node_weights":   lb.NodeWeights,
		"weights":        weights,
		"created_at":     lb.CreatedAt,
		"updated_at":     lb.UpdatedAt,
	}
}

type loadBalancerRequest struct {
	Name          string          `json:"name" binding:"omitempty,min=1,max=255"`
	GroupID       uint            `json:"group_id" binding:"omitempty,gt=0"`
	Strategy      string          `json:"strategy" binding:"omitempty,oneof=round-robin least-connections least-load weighted-random weight latency"`
	HealthCheck   *bool           `json:"health_check"`
	CheckInterval int             `json:"check_interval" binding:"omitempty,gt=0"`
	CheckTimeout  int             `json:"check_timeout" binding:"omitempty,gt=0"`
	Enabled       *bool           `json:"enabled"`
	NodeWeights   string          `json:"node_weights"`
	Weights       json.RawMessage `json:"weights"`
}

func applyLoadBalancerRequest(lb *model.LoadBalancer, req *loadBalancerRequest) {
	if req.Name != "" {
		lb.Name = req.Name
	}
	if req.GroupID > 0 {
		lb.GroupID = req.GroupID
	}
	if req.Strategy != "" {
		lb.Strategy = req.Strategy
	}
	if req.HealthCheck != nil {
		lb.HealthCheck = *req.HealthCheck
	}
	if req.CheckInterval > 0 {
		lb.CheckInterval = req.CheckInterval
	}
	if req.CheckTimeout > 0 {
		lb.CheckTimeout = req.CheckTimeout
	}
	if req.Enabled != nil {
		lb.Enabled = *req.Enabled
	}
	if len(req.Weights) > 0 && string(req.Weights) != "null" {
		lb.NodeWeights = string(req.Weights)
	}
	if req.NodeWeights != "" {
		lb.NodeWeights = req.NodeWeights
	}
}

// NewLoadBalancerHandler 创建处理器
func NewLoadBalancerHandler() *LoadBalancerHandler {
	return &LoadBalancerHandler{
		lbService: service.NewLoadBalancerService(database.Get()),
	}
}

// ListLoadBalancers godoc
// @Summary 获取负载均衡器列表
// @Description 管理员获取负载均衡器列表
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param group_id query int false "分组ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/loadbalancers [get]
func (h *LoadBalancerHandler) ListLoadBalancers(c *gin.Context) {
	groupID, _ := strconv.Atoi(c.Query("group_id"))
	if groupID < 0 {
		groupID = 0
	}

	lbs, err := h.lbService.List(uint(groupID))
	if err != nil {
		panelError(c, err.Error())
		return
	}

	list := make([]gin.H, 0, len(lbs))
	for i := range lbs {
		list = append(list, loadBalancerResponse(&lbs[i]))
	}

	panelSuccess(c, gin.H{
		"list":  list,
		"total": len(list),
	})
}

// CreateLoadBalancer godoc
// @Summary 创建负载均衡器
// @Description 管理员创建新的负载均衡器
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.LoadBalancer true "负载均衡器配置"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/loadbalancers [post]
func (h *LoadBalancerHandler) CreateLoadBalancer(c *gin.Context) {
	var req loadBalancerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	lb := model.LoadBalancer{
		HealthCheck: true,
		Enabled:     true,
		Strategy:    "round-robin",
	}
	applyLoadBalancerRequest(&lb, &req)

	if err := h.lbService.Create(&lb); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, loadBalancerResponse(&lb))
}

// GetLoadBalancer godoc
// @Summary 获取负载均衡器详情
// @Description 管理员获取指定负载均衡器的详细信息
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Router /admin/loadbalancers/{id} [get]
func (h *LoadBalancerHandler) GetLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		panelError(c, "invalid id")
		return
	}
	lb, err := h.lbService.GetByID(uint(lbID))
	if err != nil {
		panelError(c, "load balancer not found")
		return
	}

	panelSuccess(c, loadBalancerResponse(lb))
}

// UpdateLoadBalancer godoc
// @Summary 更新负载均衡器
// @Description 管理员更新指定负载均衡器的配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Param request body model.LoadBalancer true "负载均衡器配置"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/loadbalancers/{id} [put]
func (h *LoadBalancerHandler) UpdateLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		panelError(c, "invalid id")
		return
	}
	lb, err := h.lbService.GetByID(uint(lbID))
	if err != nil {
		panelError(c, "load balancer not found")
		return
	}

	var req loadBalancerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		panelError(c, err.Error())
		return
	}

	applyLoadBalancerRequest(lb, &req)
	if err := h.lbService.Update(lb); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, loadBalancerResponse(lb))
}

// DeleteLoadBalancer godoc
// @Summary 删除负载均衡器
// @Description 管理员删除指定负载均衡器
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/loadbalancers/{id} [delete]
func (h *LoadBalancerHandler) DeleteLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		panelError(c, "invalid id")
		return
	}
	if err := h.lbService.Delete(uint(lbID)); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, gin.H{"message": "deleted"})
}

// GetLoadBalancerStats godoc
// @Summary 获取负载均衡统计
// @Description 管理员获取指定负载均衡器的统计数据
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/loadbalancers/{id}/stats [get]
func (h *LoadBalancerHandler) GetLoadBalancerStats(c *gin.Context) {
	id := c.Param("id")

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		panelError(c, "invalid id")
		return
	}
	if _, err := h.lbService.GetByID(uint(lbID)); err != nil {
		panelError(c, "load balancer not found")
		return
	}
	stats, err := h.lbService.GetStats(uint(lbID))
	if err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, stats)
}

// RunHealthCheck godoc
// @Summary 执行健康检查
// @Description 管理员对指定负载均衡器执行健康检查
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/loadbalancers/{id}/check [post]
func (h *LoadBalancerHandler) RunHealthCheck(c *gin.Context) {
	id := c.Param("id")

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		panelError(c, "invalid id")
		return
	}
	if _, err := h.lbService.GetByID(uint(lbID)); err != nil {
		panelError(c, "load balancer not found")
		return
	}
	if err := h.lbService.RunHealthCheck(uint(lbID)); err != nil {
		panelError(c, err.Error())
		return
	}

	panelSuccess(c, gin.H{"message": "health check completed"})
}
