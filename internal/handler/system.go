package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
)

// SystemHandler 系统处理器
// SystemHandler handles system configuration, backup, and load balancer endpoints.
type SystemHandler struct {
	configService       *service.SystemConfigService
	backupService       *service.BackupService
	operationLogService *service.OperationLogService
}

func systemConfigResponse(cfg *model.SystemConfig, maskSensitive bool) gin.H {
	if cfg == nil {
		return gin.H{}
	}

	displayValue, sensitive, hasValue := service.MaskSystemConfigValue(cfg.Key, cfg.Value)
	value := cfg.Value
	if maskSensitive && sensitive {
		value = displayValue
	}

	return gin.H{
		"id":            cfg.ID,
		"key":           cfg.Key,
		"value":         value,
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

func systemConfigTargetID(cfg *model.SystemConfig) *uint {
	if cfg == nil || cfg.ID == 0 {
		return nil
	}

	targetID := cfg.ID
	return &targetID
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

func systemConfigAuditContent(cfg *model.SystemConfig, preserveExisting bool) string {
	if cfg == nil {
		return ""
	}

	content, err := json.Marshal(gin.H{
		"key":               cfg.Key,
		"group":             cfg.Group,
		"type":              cfg.Type,
		"sensitive":         service.IsSensitiveSystemConfigKey(cfg.Key),
		"has_value":         strings.TrimSpace(cfg.Value) != "",
		"preserve_existing": preserveExisting,
	})
	if err != nil {
		return cfg.Key
	}

	return string(content)
}

func (h *SystemHandler) recordSystemConfigAudit(c *gin.Context, action string, cfg *model.SystemConfig, preserveExisting bool) {
	if h == nil || h.operationLogService == nil || cfg == nil {
		return
	}

	if err := h.operationLogService.Record(&service.OperationLogInput{
		UserID:     contextUint(c, "user_id"),
		Username:   contextString(c, "email"),
		Action:     action,
		Module:     "system",
		TargetType: "system_config",
		TargetID:   systemConfigTargetID(cfg),
		Content:    systemConfigAuditContent(cfg, preserveExisting),
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		Status:     1,
	}); err != nil {
		log.Printf("record system config audit failed for key=%s: %v", cfg.Key, err)
	}
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

func maskBackupSensitiveValue(value string) (displayValue string, sensitive bool, hasValue bool) {
	hasValue = strings.TrimSpace(value) != ""
	if !hasValue {
		return "", true, false
	}

	return service.SensitiveSystemConfigPlaceholder, true, true
}

func backupConfigResponse(cfg *model.BackupConfig) gin.H {
	s3AccessKeyDisplayValue, s3AccessKeySensitive, s3AccessKeyHasValue := maskBackupSensitiveValue(cfg.S3AccessKey)
	s3SecretKeyDisplayValue, s3SecretKeySensitive, s3SecretKeyHasValue := maskBackupSensitiveValue(cfg.S3SecretKey)

	return gin.H{
		"id":                          cfg.ID,
		"enabled":                     cfg.Enabled,
		"auto_backup":                 cfg.AutoBackup,
		"schedule":                    cfg.Schedule,
		"retention_days":              cfg.RetentionDays,
		"backup_database":             cfg.BackupDatabase,
		"backup_files":                cfg.BackupFiles,
		"storage_type":                cfg.StorageType,
		"storage_path":                cfg.StoragePath,
		"s3_bucket":                   cfg.S3Bucket,
		"s3_region":                   cfg.S3Region,
		"s3_endpoint":                 cfg.S3Endpoint,
		"s3_access_key":               s3AccessKeyDisplayValue,
		"s3_access_key_display_value": s3AccessKeyDisplayValue,
		"s3_access_key_sensitive":     s3AccessKeySensitive,
		"s3_access_key_has_value":     s3AccessKeyHasValue,
		"s3_secret_key":               s3SecretKeyDisplayValue,
		"s3_secret_key_display_value": s3SecretKeyDisplayValue,
		"s3_secret_key_sensitive":     s3SecretKeySensitive,
		"s3_secret_key_has_value":     s3SecretKeyHasValue,
		"created_at":                  cfg.CreatedAt,
		"updated_at":                  cfg.UpdatedAt,
		// Frontend aliases used by System.vue.
		"interval":   backupIntervalFromSchedule(cfg.Schedule),
		"keep_count": cfg.RetentionDays,
	}
}

func backupConfigTargetID(cfg *model.BackupConfig) *uint {
	if cfg == nil || cfg.ID == 0 {
		return nil
	}

	targetID := cfg.ID
	return &targetID
}

func backupConfigAuditContent(cfg *model.BackupConfig, preservedSensitiveFields []string) string {
	if cfg == nil {
		return ""
	}

	content, err := json.Marshal(gin.H{
		"enabled":                    cfg.Enabled,
		"auto_backup":                cfg.AutoBackup,
		"schedule":                   cfg.Schedule,
		"retention_days":             cfg.RetentionDays,
		"backup_database":            cfg.BackupDatabase,
		"backup_files":               cfg.BackupFiles,
		"storage_type":               cfg.StorageType,
		"storage_path":               cfg.StoragePath,
		"s3_bucket":                  cfg.S3Bucket,
		"s3_region":                  cfg.S3Region,
		"s3_endpoint":                cfg.S3Endpoint,
		"s3_access_key_has_value":    strings.TrimSpace(cfg.S3AccessKey) != "",
		"s3_secret_key_has_value":    strings.TrimSpace(cfg.S3SecretKey) != "",
		"preserved_sensitive_fields": preservedSensitiveFields,
	})
	if err != nil {
		return cfg.StorageType
	}

	return string(content)
}

func (h *SystemHandler) recordBackupConfigAudit(c *gin.Context, action string, cfg *model.BackupConfig, preservedSensitiveFields []string) {
	if h == nil || h.operationLogService == nil || cfg == nil {
		return
	}

	if err := h.operationLogService.Record(&service.OperationLogInput{
		UserID:     contextUint(c, "user_id"),
		Username:   contextString(c, "email"),
		Action:     action,
		Module:     "system",
		TargetType: "backup_config",
		TargetID:   backupConfigTargetID(cfg),
		Content:    backupConfigAuditContent(cfg, preservedSensitiveFields),
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		Status:     1,
	}); err != nil {
		log.Printf("record backup config audit failed: %v", err)
	}
}

func applyBackupSensitiveFieldUpdate(req map[string]any, fieldName, currentValue string, preserveRequested bool) (updatedValue string, preserved bool) {
	rawValue, exists := req[fieldName]
	if !exists {
		return currentValue, false
	}

	if rawValue == nil {
		if preserveRequested {
			return currentValue, true
		}
		return "", false
	}

	stringValue, ok := rawValue.(string)
	if !ok {
		return currentValue, false
	}

	if stringValue == service.SensitiveSystemConfigPlaceholder {
		return currentValue, true
	}
	if preserveRequested && strings.TrimSpace(stringValue) == "" {
		return currentValue, true
	}

	return stringValue, false
}

func backupRecordTargetID(record *model.BackupRecord) *uint {
	if record == nil || record.ID == 0 {
		return nil
	}

	targetID := record.ID
	return &targetID
}

func backupRecordAuditContent(record *model.BackupRecord) string {
	if record == nil {
		return ""
	}

	filename := record.Name
	if strings.TrimSpace(record.Path) != "" {
		filename = filepath.Base(record.Path)
	}

	content, err := json.Marshal(gin.H{
		"name":         record.Name,
		"filename":     filename,
		"type":         record.Type,
		"size":         record.Size,
		"status":       backupStatusToText(record.Status),
		"status_code":  record.Status,
		"auto":         record.Auto,
		"created_by":   record.CreatedBy,
		"has_error":    strings.TrimSpace(record.Error) != "",
		"completed_at": record.CompletedAt,
		"created_at":   record.CreatedAt,
		"updated_at":   record.UpdatedAt,
	})
	if err != nil {
		return record.Name
	}

	return string(content)
}

func (h *SystemHandler) recordBackupRecordAudit(c *gin.Context, action string, record *model.BackupRecord) {
	if h == nil || h.operationLogService == nil || record == nil {
		return
	}

	if err := h.operationLogService.Record(&service.OperationLogInput{
		UserID:     contextUint(c, "user_id"),
		Username:   contextString(c, "email"),
		Action:     action,
		Module:     "system",
		TargetType: "backup_record",
		TargetID:   backupRecordTargetID(record),
		Content:    backupRecordAuditContent(record),
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		Status:     1,
	}); err != nil {
		log.Printf("record backup record audit failed: %v", err)
	}
}

// NewSystemHandler 创建处理器
// NewSystemHandler creates a system handler.
func NewSystemHandler() *SystemHandler {
	db := database.Get()
	return &SystemHandler{
		configService:       service.NewSystemConfigService(db),
		backupService:       service.NewBackupService(db),
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
		list = append(list, systemConfigResponse(&configs[i], true))
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

	panelSuccess(c, systemConfigResponse(entry, false))
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
	if existingEntry != nil && sensitiveConfig && (preserveExisting || value == service.SensitiveSystemConfigPlaceholder) {
		value = existingEntry.Value
		preserveExisting = true
	}
	if len(rawValue) == 0 && existingEntry != nil {
		if !preserveExisting {
			panelError(c, "value is required")
			return
		}
		value = existingEntry.Value
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

	resp := systemConfigResponse(savedEntry, true)
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

	panelSuccess(c, gin.H{"message": "config deleted"})
}

// ========== 备份管理 ==========

// GetBackupConfig godoc
// @Summary 获取备份配置
// @Description 管理员获取系统备份配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/backup/config [get]
func (h *SystemHandler) GetBackupConfig(c *gin.Context) {
	cfg, err := h.backupService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	panelSuccess(c, backupConfigResponse(cfg))
}

// UpdateBackupConfig godoc
// @Summary 更新备份配置
// @Description 管理员更新系统备份配置
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body model.BackupConfig true "备份配置"
// @Success 200 {object} map[string]any
// @Failure 400 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/backup/config [put]
func (h *SystemHandler) UpdateBackupConfig(c *gin.Context) {
	currentCfg, err := h.backupService.GetConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	cfg := *currentCfg
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	preserveExistingSensitive, _ := req["preserve_existing_sensitive"].(bool)
	preservedSensitiveFields := make([]string, 0, 2)

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
	updatedS3AccessKey, preservedS3AccessKey := applyBackupSensitiveFieldUpdate(req, "s3_access_key", cfg.S3AccessKey, preserveExistingSensitive)
	cfg.S3AccessKey = updatedS3AccessKey
	if preservedS3AccessKey {
		preservedSensitiveFields = append(preservedSensitiveFields, "s3_access_key")
	}
	updatedS3SecretKey, preservedS3SecretKey := applyBackupSensitiveFieldUpdate(req, "s3_secret_key", cfg.S3SecretKey, preserveExistingSensitive)
	cfg.S3SecretKey = updatedS3SecretKey
	if preservedS3SecretKey {
		preservedSensitiveFields = append(preservedSensitiveFields, "s3_secret_key")
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

	h.recordBackupConfigAudit(c, "update", &cfg, preservedSensitiveFields)

	panelSuccess(c, backupConfigResponse(&cfg))
}

// CreateBackup godoc
// @Summary 创建备份
// @Description 管理员手动创建系统备份
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param type query string false "备份类型 (database/full)" default(database)
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/backup [post]
func (h *SystemHandler) CreateBackup(c *gin.Context) {
	backupType := c.DefaultQuery("type", "database")

	record, err := h.backupService.CreateBackup(backupType, contextUint(c, "user_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.recordBackupRecordAudit(c, "create", record)

	panelSuccess(c, record)
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
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/backups [get]
func (h *SystemHandler) ListBackups(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = ClampPagination(page, pageSize)

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

	panelSuccess(c, gin.H{
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
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
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

	panelSuccess(c, stats)
}

// DeleteBackup godoc
// @Summary 删除备份
// @Description 管理员删除指定备份记录
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "备份ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/backups/{id} [delete]
func (h *SystemHandler) DeleteBackup(c *gin.Context) {
	id := c.Param("id")

	backupID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var record model.BackupRecord
	recordLoaded := database.Get().First(&record, uint(backupID)).Error == nil
	if err := h.backupService.DeleteBackup(uint(backupID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if recordLoaded {
		h.recordBackupRecordAudit(c, "delete", &record)
	}

	panelSuccess(c, gin.H{"message": "backup deleted"})
}

// RestoreBackup godoc
// @Summary 恢复备份
// @Description 管理员从备份恢复系统
// @Tags 管理端-系统
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "备份ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]any
// @Router /admin/system/backups/{id}/restore [post]
func (h *SystemHandler) RestoreBackup(c *gin.Context) {
	id := c.Param("id")

	backupID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var record model.BackupRecord
	recordLoaded := database.Get().First(&record, uint(backupID)).Error == nil
	if err := h.backupService.RestoreBackup(uint(backupID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if recordLoaded {
		h.recordBackupRecordAudit(c, "restore", &record)
	}

	panelSuccess(c, gin.H{"message": "backup restored, please restart server"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	stats, err := h.lbService.GetStats(uint(lbID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
