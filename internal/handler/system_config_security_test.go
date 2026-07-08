package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemConfigSecurityTest(t *testing.T) (*gorm.DB, *SystemHandler, *gin.Engine) {
	t.Helper()

	db := initTestDB()
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.BackupConfig{}, &model.BackupRecord{}, &model.OperationLog{}))
	require.NoError(t, db.Exec("DELETE FROM v2_system_config").Error)
	require.NoError(t, db.Exec("DELETE FROM v2_backup_config").Error)
	require.NoError(t, db.Exec("DELETE FROM v2_backup_record").Error)
	require.NoError(t, db.Exec("DELETE FROM v2_operation_log").Error)

	router := gin.New()
	handler := NewSystemHandler()
	adminUserID := uint(99)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", adminUserID)
		c.Set("email", "admin@example.com")
		c.Set("is_admin", true)
		c.Next()
	})

	return db, handler, router
}

func performSystemConfigJSONRequest(t *testing.T, router *gin.Engine, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, body)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func decodeJSONMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func findConfigByKey(t *testing.T, items any, key string) map[string]any {
	t.Helper()

	list, ok := items.([]any)
	require.True(t, ok)

	for _, item := range list {
		decoded, ok := item.(map[string]any)
		require.True(t, ok)
		if decoded["key"] == key {
			return decoded
		}
	}

	t.Fatalf("config %q not found", key)
	return nil
}

func TestSystemHandlerGetConfigsMasksSensitiveValues(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.GET("/admin/system/configs", handler.GetConfigs)

	require.NoError(t, db.Create(&model.SystemConfig{
		Key:    "forward.runtime.nodex.token",
		Value:  "super-secret-token",
		Type:   "string",
		Group:  "forward",
		Remark: "NodeX runtime token",
	}).Error)
	require.NoError(t, db.Create(&model.SystemConfig{
		Key:    "site.name",
		Value:  "NodeX Panel",
		Type:   "string",
		Group:  "site",
		Remark: "Site name",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs", nil)
	require.Equal(t, http.StatusOK, recorder.Code)

	body := decodeJSONMap(t, recorder)
	assert.Equal(t, float64(0), body["code"])
	require.NotEmpty(t, body["msg"])
	require.NotZero(t, body["ts"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	sensitiveCfg := findConfigByKey(t, data["list"], "forward.runtime.nodex.token")
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, sensitiveCfg["value"])
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, sensitiveCfg["display_value"])
	assert.Equal(t, true, sensitiveCfg["sensitive"])
	assert.Equal(t, true, sensitiveCfg["has_value"])

	plainCfg := findConfigByKey(t, data["list"], "site.name")
	assert.Equal(t, "NodeX Panel", plainCfg["value"])
	assert.Equal(t, "NodeX Panel", plainCfg["display_value"])
	assert.Equal(t, false, plainCfg["sensitive"])
}

func TestSystemHandlerGetConfigReturnsRawSensitiveValueWithMaskedDisplay(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.GET("/admin/system/configs/:key", handler.GetConfig)

	require.NoError(t, db.Create(&model.SystemConfig{
		Key:    "forward.runtime.nodex.token",
		Value:  "runtime-secret-token",
		Type:   "string",
		Group:  "forward",
		Remark: "NodeX runtime token",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/forward.runtime.nodex.token", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := decodeJSONMap(t, recorder)
	assert.Equal(t, float64(0), body["code"])
	require.NotEmpty(t, body["msg"])
	require.NotZero(t, body["ts"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "runtime-secret-token", data["value"])
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["display_value"])
	assert.Equal(t, true, data["sensitive"])
	assert.Equal(t, true, data["has_value"])
}

func TestSystemHandlerSetConfigPreservesSensitiveValueAndWritesAuditLog(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.PUT("/admin/system/configs/:key", handler.SetConfig)

	require.NoError(t, db.Create(&model.SystemConfig{
		Key:    "forward.runtime.nodex.token",
		Value:  "super-secret-token",
		Type:   "string",
		Group:  "forward",
		Remark: "NodeX runtime token",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/forward.runtime.nodex.token", map[string]any{
		"value":             "",
		"type":              "string",
		"group":             "forward",
		"description":       "NodeX runtime token",
		"preserve_existing": true,
	})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var stored model.SystemConfig
	require.NoError(t, db.Where("key = ?", "forward.runtime.nodex.token").First(&stored).Error)
	assert.Equal(t, "super-secret-token", stored.Value)

	body := decodeJSONMap(t, recorder)
	assert.Equal(t, float64(0), body["code"])
	require.NotEmpty(t, body["msg"])
	require.NotZero(t, body["ts"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["value"])
	assert.Equal(t, true, data["sensitive"])
	assert.Equal(t, true, data["has_value"])

	var logs []model.OperationLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "update", logs[0].Action)
	assert.Equal(t, "admin@example.com", logs[0].Username)
	assert.NotContains(t, logs[0].Content, "super-secret-token")
	assert.Contains(t, logs[0].Content, "\"preserve_existing\":true")
}

func TestSystemHandlerSetConfigTreatsSensitivePlaceholderAsPreserveExisting(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.PUT("/admin/system/configs/:key", handler.SetConfig)

	require.NoError(t, db.Create(&model.SystemConfig{
		Key:    "forward.runtime.nodex.token",
		Value:  "placeholder-should-not-overwrite",
		Type:   "string",
		Group:  "forward",
		Remark: "NodeX runtime token",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/forward.runtime.nodex.token", map[string]any{
		"value":       service.SensitiveSystemConfigPlaceholder,
		"type":        "string",
		"group":       "forward",
		"description": "NodeX runtime token",
	})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var stored model.SystemConfig
	require.NoError(t, db.Where("key = ?", "forward.runtime.nodex.token").First(&stored).Error)
	assert.Equal(t, "placeholder-should-not-overwrite", stored.Value)

	var logs []model.OperationLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.NotContains(t, logs[0].Content, service.SensitiveSystemConfigPlaceholder)
	assert.Contains(t, logs[0].Content, "\"preserve_existing\":true")
}

func TestSystemHandlerDeleteConfigWritesAuditLogWithoutSecretLeak(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.DELETE("/admin/system/configs/:key", handler.DeleteConfig)

	require.NoError(t, db.Create(&model.SystemConfig{
		Key:    "forward.runtime.nodex.token",
		Value:  "delete-me-secret",
		Type:   "string",
		Group:  "forward",
		Remark: "NodeX runtime token",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodDelete, "/admin/system/configs/forward.runtime.nodex.token", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := decodeJSONMap(t, recorder)
	assert.Equal(t, float64(0), body["code"])
	require.NotEmpty(t, body["msg"])
	require.NotZero(t, body["ts"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "config deleted", data["message"])

	var count int64
	require.NoError(t, db.Model(&model.SystemConfig{}).Where("key = ?", "forward.runtime.nodex.token").Count(&count).Error)
	assert.Equal(t, int64(0), count)

	var logs []model.OperationLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "delete", logs[0].Action)
	assert.NotContains(t, logs[0].Content, "delete-me-secret")
	assert.Contains(t, logs[0].Content, "\"sensitive\":true")
}

func TestSystemHandlerGetBackupConfigMasksSensitiveFields(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.GET("/admin/system/backup/config", handler.GetBackupConfig)

	require.NoError(t, db.Create(&model.BackupConfig{
		Enabled:        true,
		AutoBackup:     true,
		Schedule:       "interval:12",
		RetentionDays:  14,
		BackupDatabase: true,
		BackupFiles:    true,
		StorageType:    "s3",
		StoragePath:    "backups",
		S3Bucket:       "panel-backups",
		S3Region:       "eu-west-1",
		S3Endpoint:     "https://s3.example.com",
		S3AccessKey:    "ACCESS-KEY-123",
		S3SecretKey:    "SECRET-KEY-456",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/backup/config", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := decodeJSONMap(t, recorder)
	assert.Equal(t, float64(0), body["code"])
	assert.NotContains(t, body, "error")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["s3_access_key"])
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["s3_secret_key"])
	assert.Equal(t, true, data["s3_access_key_sensitive"])
	assert.Equal(t, true, data["s3_secret_key_sensitive"])
	assert.Equal(t, true, data["s3_access_key_has_value"])
	assert.Equal(t, true, data["s3_secret_key_has_value"])
}

func TestSystemHandlerUpdateBackupConfigPreservesSensitiveFieldsAndWritesAuditLog(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.PUT("/admin/system/backup/config", handler.UpdateBackupConfig)

	require.NoError(t, db.Create(&model.BackupConfig{
		Enabled:        true,
		AutoBackup:     true,
		Schedule:       "interval:24",
		RetentionDays:  7,
		BackupDatabase: true,
		BackupFiles:    false,
		StorageType:    "s3",
		StoragePath:    "backups",
		S3Bucket:       "panel-backups",
		S3Region:       "eu-west-1",
		S3Endpoint:     "https://s3.example.com",
		S3AccessKey:    "ACCESS-KEY-123",
		S3SecretKey:    "SECRET-KEY-456",
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/backup/config", map[string]any{
		"enabled":                     true,
		"backup_database":             true,
		"backup_files":                true,
		"storage_type":                "s3",
		"storage_path":                "backups",
		"s3_bucket":                   "panel-backups",
		"s3_region":                   "eu-west-1",
		"s3_endpoint":                 "https://s3.example.com",
		"s3_access_key":               "",
		"s3_secret_key":               "",
		"preserve_existing_sensitive": true,
		"keep_count":                  30,
		"interval":                    6,
	})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var stored model.BackupConfig
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, "ACCESS-KEY-123", stored.S3AccessKey)
	assert.Equal(t, "SECRET-KEY-456", stored.S3SecretKey)
	assert.Equal(t, 30, stored.RetentionDays)
	assert.Equal(t, "interval:6", stored.Schedule)

	body := decodeJSONMap(t, recorder)
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["s3_access_key"])
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["s3_secret_key"])

	var logs []model.OperationLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "backup_config", logs[0].TargetType)
	assert.Equal(t, "update", logs[0].Action)
	assert.NotContains(t, logs[0].Content, "ACCESS-KEY-123")
	assert.NotContains(t, logs[0].Content, "SECRET-KEY-456")
	assert.Contains(t, logs[0].Content, "\"preserved_sensitive_fields\":[\"s3_access_key\",\"s3_secret_key\"]")
}

func TestSystemHandlerUpdateBackupConfigTreatsSensitivePlaceholderAsPreserveExisting(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.PUT("/admin/system/backup/config", handler.UpdateBackupConfig)

	require.NoError(t, db.Create(&model.BackupConfig{
		Enabled:        true,
		StorageType:    "s3",
		StoragePath:    "backups",
		S3AccessKey:    "ACCESS-KEY-123",
		S3SecretKey:    "SECRET-KEY-456",
		BackupDatabase: true,
	}).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/backup/config", map[string]any{
		"storage_type":  "s3",
		"s3_access_key": service.SensitiveSystemConfigPlaceholder,
		"s3_secret_key": service.SensitiveSystemConfigPlaceholder,
	})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var stored model.BackupConfig
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, "ACCESS-KEY-123", stored.S3AccessKey)
	assert.Equal(t, "SECRET-KEY-456", stored.S3SecretKey)
}

func TestSystemHandlerCreateBackupUsesActorAndWritesAuditLog(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.POST("/admin/system/backup", handler.CreateBackup)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPost, "/admin/system/backup?type=files", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var record model.BackupRecord
	require.NoError(t, db.Order("id DESC").First(&record).Error)
	require.NotNil(t, record.CreatedBy)
	assert.Equal(t, uint(99), *record.CreatedBy)

	var logs []model.OperationLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "backup_record", logs[0].TargetType)
	assert.Equal(t, "create", logs[0].Action)
	assert.Contains(t, logs[0].Content, "\"type\":\"files\"")
}

func TestSystemHandlerDeleteBackupWritesAuditLog(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.DELETE("/admin/system/backups/:id", handler.DeleteBackup)

	require.NoError(t, db.Create(&model.BackupRecord{
		Name:   "backup_20260411_000000",
		Type:   "files",
		Status: 1,
		Auto:   false,
	}).Error)

	var record model.BackupRecord
	require.NoError(t, db.Order("id DESC").First(&record).Error)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodDelete, "/admin/system/backups/"+strconv.FormatUint(uint64(record.ID), 10), nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var logs []model.OperationLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "backup_record", logs[0].TargetType)
	assert.Equal(t, "delete", logs[0].Action)
	assert.Contains(t, logs[0].Content, "\"filename\":\"backup_20260411_000000\"")
}
