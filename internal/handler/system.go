package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// SystemHandler 系统处理器
type SystemHandler struct {
	configService *service.SystemConfigService
	backupService *service.BackupService
}

func backupStatusToText(status int) string {
	switch status {
	case 1:
		return "completed"
	case 2:
		return "failed"
	default:
		return "pending"
	}
}

func backupIntervalFromSchedule(schedule string) int {
	schedule = strings.TrimSpace(schedule)
	if strings.HasPrefix(schedule, "interval:") {
		raw := strings.TrimPrefix(schedule, "interval:")
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 24
}

func backupConfigResponse(cfg *model.BackupConfig) gin.H {
	return gin.H{
		"id":              cfg.ID,
		"enabled":         cfg.Enabled,
		"auto_backup":     cfg.AutoBackup,
		"schedule":        cfg.Schedule,
		"retention_days":  cfg.RetentionDays,
		"backup_database": cfg.BackupDatabase,
		"backup_files":    cfg.BackupFiles,
		"storage_type":    cfg.StorageType,
		"storage_path":    cfg.StoragePath,
		"s3_bucket":       cfg.S3Bucket,
		"s3_region":       cfg.S3Region,
		"s3_endpoint":     cfg.S3Endpoint,
		"s3_access_key":   cfg.S3AccessKey,
		"s3_secret_key":   cfg.S3SecretKey,
		"created_at":      cfg.CreatedAt,
		"updated_at":      cfg.UpdatedAt,
		// Frontend aliases used by System.vue.
		"interval":   backupIntervalFromSchedule(cfg.Schedule),
		"keep_count": cfg.RetentionDays,
	}
}

// NewSystemHandler 创建处理器
func NewSystemHandler() *SystemHandler {
	db := database.Get()
	return &SystemHandler{
		configService: service.NewSystemConfigService(db),
		backupService: service.NewBackupService(db),
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

	list := make([]gin.H, 0, len(configs))
	for _, cfg := range configs {
		list = append(list, gin.H{
			"id":          cfg.ID,
			"key":         cfg.Key,
			"value":       cfg.Value,
			"type":        cfg.Type,
			"group":       cfg.Group,
			"remark":      cfg.Remark,
			"description": cfg.Remark,
			"created_at":  cfg.CreatedAt,
			"updated_at":  cfg.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":  list,
			"total": len(list),
		},
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
		Value       json.RawMessage `json:"value"`
		Type        string          `json:"type"`
		Group       string          `json:"group"`
		Remark      string          `json:"remark"`
		Description string          `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rawValue := bytes.TrimSpace(req.Value)
	if len(rawValue) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "value is required"})
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

	if err := h.configService.Set(key, value, req.Type, req.Group, remark); err != nil {
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

	c.JSON(http.StatusOK, gin.H{"data": backupConfigResponse(cfg)})
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
	currentCfg, err := h.backupService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cfg := *currentCfg
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if rawEnabled, ok := req["enabled"].(bool); ok {
		cfg.Enabled = rawEnabled
	}
	if rawAutoBackup, ok := req["auto_backup"].(bool); ok {
		cfg.AutoBackup = rawAutoBackup
	}
	if rawSchedule, ok := req["schedule"].(string); ok {
		cfg.Schedule = rawSchedule
	}
	if rawRetentionDays, ok := req["retention_days"]; ok {
		switch v := rawRetentionDays.(type) {
		case float64:
			cfg.RetentionDays = int(v)
		case int:
			cfg.RetentionDays = v
		}
	}
	if rawBackupDatabase, ok := req["backup_database"].(bool); ok {
		cfg.BackupDatabase = rawBackupDatabase
	}
	if rawBackupFiles, ok := req["backup_files"].(bool); ok {
		cfg.BackupFiles = rawBackupFiles
	}
	if rawStorageType, ok := req["storage_type"].(string); ok {
		cfg.StorageType = rawStorageType
	}
	if rawStoragePath, ok := req["storage_path"].(string); ok {
		cfg.StoragePath = rawStoragePath
	}
	if rawS3Bucket, ok := req["s3_bucket"].(string); ok {
		cfg.S3Bucket = rawS3Bucket
	}
	if rawS3Region, ok := req["s3_region"].(string); ok {
		cfg.S3Region = rawS3Region
	}
	if rawS3Endpoint, ok := req["s3_endpoint"].(string); ok {
		cfg.S3Endpoint = rawS3Endpoint
	}
	if rawS3AccessKey, ok := req["s3_access_key"].(string); ok {
		cfg.S3AccessKey = rawS3AccessKey
	}
	if rawS3SecretKey, ok := req["s3_secret_key"].(string); ok {
		cfg.S3SecretKey = rawS3SecretKey
	}
	if rawKeepCount, ok := req["keep_count"]; ok {
		switch v := rawKeepCount.(type) {
		case float64:
			cfg.RetentionDays = int(v)
		case int:
			cfg.RetentionDays = v
		}
	}
	if rawInterval, ok := req["interval"]; ok {
		switch v := rawInterval.(type) {
		case float64:
			if int(v) > 0 {
				cfg.Schedule = "interval:" + strconv.Itoa(int(v))
			}
		case int:
			if v > 0 {
				cfg.Schedule = "interval:" + strconv.Itoa(v)
			}
		}
	}

	if err := h.backupService.UpdateConfig(&cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": backupConfigResponse(&cfg)})
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

	list := make([]gin.H, 0, len(records))
	for _, record := range records {
		filename := record.Name
		if record.Path != "" {
			filename = filepath.Base(record.Path)
		}
		list = append(list, gin.H{
			"id":           record.ID,
			"name":         record.Name,
			"filename":     filename,
			"type":         record.Type,
			"size":         record.Size,
			"path":         record.Path,
			"status":       backupStatusToText(record.Status),
			"status_code":  record.Status,
			"error":        record.Error,
			"auto":         record.Auto,
			"created_by":   record.CreatedBy,
			"completed_at": record.CompletedAt,
			"created_at":   record.CreatedAt,
			"updated_at":   record.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":      list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"list":      list,
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

	if totalBackups, ok := stats["total_backups"]; ok {
		stats["total_count"] = totalBackups
	} else if _, ok := stats["total_count"]; !ok {
		stats["total_count"] = int64(0)
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

	backupID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
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

	backupID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
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

func loadBalancerResponse(lb *model.LoadBalancer) gin.H {
	weights := interface{}(map[string]interface{}{})
	if lb.NodeWeights != "" {
		var decoded interface{}
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
	Name          string          `json:"name"`
	GroupID       uint            `json:"group_id"`
	Strategy      string          `json:"strategy"`
	HealthCheck   *bool           `json:"health_check"`
	CheckInterval int             `json:"check_interval"`
	CheckTimeout  int             `json:"check_timeout"`
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

	list := make([]gin.H, 0, len(lbs))
	for i := range lbs {
		list = append(list, loadBalancerResponse(&lbs[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"list":  list,
			"total": len(list),
		},
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
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /admin/loadbalancers [post]
func (h *LoadBalancerHandler) CreateLoadBalancer(c *gin.Context) {
	var req loadBalancerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lb := model.LoadBalancer{
		HealthCheck: true,
		Enabled:     true,
		Strategy:    "round-robin",
	}
	applyLoadBalancerRequest(&lb, &req)

	if err := h.lbService.Create(&lb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": loadBalancerResponse(&lb)})
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

	c.JSON(http.StatusOK, gin.H{"data": loadBalancerResponse(lb)})
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

	var req loadBalancerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	applyLoadBalancerRequest(lb, &req)
	if err := h.lbService.Update(lb); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": loadBalancerResponse(lb)})
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

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
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

	lbID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.lbService.RunHealthCheck(uint(lbID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "health check completed"})
}
