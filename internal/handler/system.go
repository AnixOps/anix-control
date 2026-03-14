package handler

import (
	"net/http"
	"strconv"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// SystemHandler 系统处理器
type SystemHandler struct {
	configService  *service.SystemConfigService
	backupService  *service.BackupService
}

// NewSystemHandler 创建处理器
func NewSystemHandler() *SystemHandler {
	db := database.Get()
	return &SystemHandler{
		configService:  service.NewSystemConfigService(db),
		backupService:  service.NewBackupService(db),
	}
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
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": configs})
}

// GetConfig godoc
// @Summary 获取单个系统配置
// @Description 管理员获取指定key的系统配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "配置键名"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/configs/{key} [get]
func (h *SystemHandler) GetConfig(c *gin.Context) {
	key := c.Param("key")

	value, err := h.configService.Get(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"key": key, "value": value}})
}

// SetConfig godoc
// @Summary 设置系统配置
// @Description 管理员设置系统配置项
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "配置键名"
// @Param request body map[string]interface{} true "配置值"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/configs/{key} [put]
func (h *SystemHandler) SetConfig(c *gin.Context) {
	key := c.Param("key")

	var req struct {
		Value  string `json:"value" binding:"required"`
		Type   string `json:"type"`
		Group  string `json:"group"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configService.Set(key, req.Value, req.Type, req.Group, req.Remark); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "config updated"})
}

// DeleteConfig godoc
// @Summary 删除系统配置
// @Description 管理员删除指定key的系统配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "配置键名"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/configs/{key} [delete]
func (h *SystemHandler) DeleteConfig(c *gin.Context) {
	key := c.Param("key")

	if err := h.configService.Delete(key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "config deleted"})
}

// ========== 备份管理 ==========

// GetBackupConfig godoc
// @Summary 获取备份配置
// @Description 管理员获取系统备份配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backup/config [get]
func (h *SystemHandler) GetBackupConfig(c *gin.Context) {
	cfg, err := h.backupService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// UpdateBackupConfig godoc
// @Summary 更新备份配置
// @Description 管理员更新系统备份配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.BackupConfig true "备份配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backup/config [put]
func (h *SystemHandler) UpdateBackupConfig(c *gin.Context) {
	var cfg model.BackupConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.backupService.UpdateConfig(&cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": cfg})
}

// CreateBackup godoc
// @Summary 创建备份
// @Description 管理员手动创建系统备份
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param type query string false "备份类型 (database/full)" default(database)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backup [post]
func (h *SystemHandler) CreateBackup(c *gin.Context) {
	backupType := c.DefaultQuery("type", "database")

	record, err := h.backupService.CreateBackup(backupType, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": record})
}

// ListBackups godoc
// @Summary 获取备份列表
// @Description 管理员获取备份记录列表
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backups [get]
func (h *SystemHandler) ListBackups(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	records, total, err := h.backupService.ListBackups(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      records,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetBackupStats godoc
// @Summary 获取备份统计
// @Description 管理员获取备份统计数据
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backup/stats [get]
func (h *SystemHandler) GetBackupStats(c *gin.Context) {
	stats, err := h.backupService.GetBackupStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// DeleteBackup godoc
// @Summary 删除备份
// @Description 管理员删除指定备份记录
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "备份ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backups/{id} [delete]
func (h *SystemHandler) DeleteBackup(c *gin.Context) {
	id := c.Param("id")

	backupID, _ := strconv.ParseUint(id, 10, 32)
	if err := h.backupService.DeleteBackup(uint(backupID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backup deleted"})
}

// RestoreBackup godoc
// @Summary 恢复备份
// @Description 管理员从备份恢复系统
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "备份ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/system/backups/{id}/restore [post]
func (h *SystemHandler) RestoreBackup(c *gin.Context) {
	id := c.Param("id")

	backupID, _ := strconv.ParseUint(id, 10, 32)
	if err := h.backupService.RestoreBackup(uint(backupID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backup restored, please restart server"})
}

// ========== 负载均衡 ==========

// LoadBalancerHandler 负载均衡处理器
type LoadBalancerHandler struct {
	lbService *service.LoadBalancerService
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
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers [get]
func (h *LoadBalancerHandler) ListLoadBalancers(c *gin.Context) {
	groupID, _ := strconv.Atoi(c.Query("group_id"))

	lbs, err := h.lbService.List(uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": lbs})
}

// CreateLoadBalancer godoc
// @Summary 创建负载均衡器
// @Description 管理员创建新的负载均衡器
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.LoadBalancer true "负载均衡器配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers [post]
func (h *LoadBalancerHandler) CreateLoadBalancer(c *gin.Context) {
	var lb model.LoadBalancer
	if err := c.ShouldBindJSON(&lb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.lbService.Create(&lb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": lb})
}

// GetLoadBalancer godoc
// @Summary 获取负载均衡器详情
// @Description 管理员获取指定负载均衡器的详细信息
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /admin/loadbalancers/{id} [get]
func (h *LoadBalancerHandler) GetLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lbID, _ := strconv.ParseUint(id, 10, 32)
	lb, err := h.lbService.GetByID(uint(lbID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "load balancer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": lb})
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
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers/{id} [put]
func (h *LoadBalancerHandler) UpdateLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lbID, _ := strconv.ParseUint(id, 10, 32)
	lb, err := h.lbService.GetByID(uint(lbID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "load balancer not found"})
		return
	}

	var req model.LoadBalancer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.ID = lb.ID
	if err := h.lbService.Update(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": req})
}

// DeleteLoadBalancer godoc
// @Summary 删除负载均衡器
// @Description 管理员删除指定负载均衡器
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers/{id} [delete]
func (h *LoadBalancerHandler) DeleteLoadBalancer(c *gin.Context) {
	id := c.Param("id")

	lbID, _ := strconv.ParseUint(id, 10, 32)
	if err := h.lbService.Delete(uint(lbID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// GetLoadBalancerStats godoc
// @Summary 获取负载均衡统计
// @Description 管理员获取指定负载均衡器的统计数据
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers/{id}/stats [get]
func (h *LoadBalancerHandler) GetLoadBalancerStats(c *gin.Context) {
	id := c.Param("id")

	lbID, _ := strconv.ParseUint(id, 10, 32)
	stats, err := h.lbService.GetStats(uint(lbID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// RunHealthCheck godoc
// @Summary 执行健康检查
// @Description 管理员对指定负载均衡器执行健康检查
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "负载均衡器ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers/{id}/check [post]
func (h *LoadBalancerHandler) RunHealthCheck(c *gin.Context) {
	id := c.Param("id")

	lbID, _ := strconv.ParseUint(id, 10, 32)
	if err := h.lbService.RunHealthCheck(uint(lbID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "health check completed"})
}