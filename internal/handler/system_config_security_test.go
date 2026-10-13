package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemConfigSecurityTest(t *testing.T) (*gorm.DB, *SystemHandler, *gin.Engine) {
	t.Helper()
	db := initTestDB()
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.OperationLog{}))
	require.NoError(t, db.Exec("DELETE FROM v2_system_config").Error)
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

// systemConfigData decodes a successful panel answer's data.
func systemConfigData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := decodeJSONMap(t, recorder)
	require.Equal(t, float64(0), body["code"], recorder.Body.String())
	require.NotEmpty(t, body["msg"])
	require.NotZero(t, body["ts"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	return data
}

// The single-key answer masks a sensitive value as the list does: the
// value and display_value read the placeholder, and the stored secret
// appears nowhere in the answer.
func TestSystemHandlerGetConfigMasksSensitiveValues(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.GET("/admin/system/configs/:key", handler.GetConfig)

	secrets := map[string]string{
		"forward.runtime.nodex.token": "runtime-secret-token",
		"smtp.password":               "smtp-secret-password",
		"payment.stripe.api_key":      "sk_live_secret",
		"oauth.client_secret":         "oauth-client-secret",
	}
	for key, value := range secrets {
		require.True(t, service.IsSensitiveSystemConfigKey(key), key)
		require.NoError(t, db.Create(&model.SystemConfig{Key: key, Value: value, Type: "string", Group: "test", Remark: key}).Error)
	}

	for key, value := range secrets {
		recorder := performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/"+key, nil)
		data := systemConfigData(t, recorder)
		assert.Equal(t, key, data["key"])
		assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["value"], key)
		assert.Equal(t, service.SensitiveSystemConfigPlaceholder, data["display_value"], key)
		assert.Equal(t, true, data["sensitive"], key)
		assert.Equal(t, true, data["has_value"], key)
		assert.NotContains(t, recorder.Body.String(), value, key)
	}
}

// A sensitive key without a value, or that does not exist, answers an
// empty value and has_value false; a plain key answers its value.
func TestSystemHandlerGetConfigAnswersEmptySecretsAndPlainValues(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.GET("/admin/system/configs/:key", handler.GetConfig)

	require.NoError(t, db.Create(&[]model.SystemConfig{
		{Key: "forward.runtime.nodex.token", Value: "", Type: "string", Group: "forward"},
		{Key: "forward.runtime.nodex.base_url", Value: "https://nodex.internal", Type: "string", Group: "forward"},
	}).Error)

	for _, key := range []string{"forward.runtime.nodex.token", "smtp.password"} {
		data := systemConfigData(t, performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/"+key, nil))
		assert.Equal(t, "", data["value"], key)
		assert.Equal(t, "", data["display_value"], key)
		assert.Equal(t, true, data["sensitive"], key)
		assert.Equal(t, false, data["has_value"], key)
	}

	data := systemConfigData(t, performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/forward.runtime.nodex.base_url", nil))
	assert.Equal(t, "https://nodex.internal", data["value"])
	assert.Equal(t, "https://nodex.internal", data["display_value"])
	assert.Equal(t, false, data["sensitive"])
	assert.Equal(t, true, data["has_value"])
}

// An editor that reads a secret and saves the answer back keeps the stored
// secret; a new value replaces it, and no answer echoes it.
func TestSystemHandlerSensitiveConfigRoundTripKeepsTheStoredValue(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.GET("/admin/system/configs/:key", handler.GetConfig)
	router.PUT("/admin/system/configs/:key", handler.SetConfig)
	const key = "forward.runtime.nodex.token"
	stored := func() string {
		var cfg model.SystemConfig
		require.NoError(t, db.Where("key = ?", key).First(&cfg).Error)
		return cfg.Value
	}

	require.NoError(t, db.Create(&model.SystemConfig{Key: key, Value: "first-secret", Type: "string", Group: "forward"}).Error)
	read := systemConfigData(t, performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/"+key, nil))

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/"+key, map[string]any{
		"value": read["value"], "type": "string", "group": "forward", "description": "Forward runtime NodeX token",
	})
	saved := systemConfigData(t, recorder)
	assert.Equal(t, "first-secret", stored(), "saving the answer back keeps the secret")
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, saved["value"])
	assert.NotContains(t, recorder.Body.String(), "first-secret")

	recorder = performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/"+key, map[string]any{
		"value": "second-secret", "type": "string", "group": "forward",
	})
	saved = systemConfigData(t, recorder)
	assert.Equal(t, "second-secret", stored(), "a new value replaces the secret")
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, saved["value"])
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, saved["display_value"])
	assert.Equal(t, true, saved["has_value"])
	assert.NotContains(t, recorder.Body.String(), "second-secret")

	recorder = performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/"+key, nil)
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, systemConfigData(t, recorder)["value"])
	assert.NotContains(t, recorder.Body.String(), "second-secret")
}

// The placeholder is not a value: saving it for a secret that is not
// stored is refused and stores nothing.
func TestSystemHandlerSetConfigRefusesThePlaceholderForANewSecret(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	router.PUT("/admin/system/configs/:key", handler.SetConfig)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/smtp.password", map[string]any{
		"value": service.SensitiveSystemConfigPlaceholder, "type": "string", "group": "mail",
	})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := decodeJSONMap(t, recorder)
	assert.NotEqual(t, float64(0), body["code"])
	assert.Equal(t, "value is required", body["msg"])
	var count int64
	require.NoError(t, db.Model(&model.SystemConfig{}).Where("key = ?", "smtp.password").Count(&count).Error)
	assert.Zero(t, count)

	recorder = performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/site.motto", map[string]any{
		"value": service.SensitiveSystemConfigPlaceholder, "type": "string", "group": "site",
	})
	assert.Equal(t, service.SensitiveSystemConfigPlaceholder, systemConfigData(t, recorder)["value"], "a plain key stores any value")
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
