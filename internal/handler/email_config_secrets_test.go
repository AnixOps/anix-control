package handler

import (
	"net/http"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const storedEmailConfigValue = `{"host":"smtp.test","port":465,"username":"mailer","password":"smtp-secret","from_name":"Anix","from_address":"noreply@example.test","encryption":"ssl"}`

func seedEmailConfig(t *testing.T, db *gorm.DB, value string) {
	t.Helper()
	require.NoError(t, db.Create(&model.SystemConfig{Key: notificationEmailConfigKey, Value: value, Type: "json", Group: "notification", Remark: "Email notification config"}).Error)
}

func storedEmailConfig(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var row model.SystemConfig
	require.NoError(t, db.Where("key = ?", notificationEmailConfigKey).Take(&row).Error)
	return row.Value
}

// The generic system configuration answers (list, single key, update)
// show the e-mail configuration with its password masked, and saving the
// value they showed keeps the stored password.
func TestSystemConfigAnswersMaskTheEmailPassword(t *testing.T) {
	db, handler, router := setupSystemConfigSecurityTest(t)
	seedEmailConfig(t, db, storedEmailConfigValue)
	router.GET("/admin/system/configs", handler.GetConfigs)
	router.GET("/admin/system/configs/:key", handler.GetConfig)
	router.PUT("/admin/system/configs/:key", handler.SetConfig)
	masked := `{"host":"smtp.test","port":465,"username":"mailer","password":"********","from_name":"Anix","from_address":"noreply@example.test","encryption":"ssl"}`

	list := performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs", nil)
	require.NotContains(t, list.Body.String(), "smtp-secret")
	item := findConfigByKey(t, decodeJSONMap(t, list)["data"].(map[string]any)["list"], notificationEmailConfigKey)
	require.Equal(t, masked, item["value"])
	require.Equal(t, masked, item["display_value"])
	require.Equal(t, false, item["sensitive"], "the value stays editable")
	require.Equal(t, true, item["has_value"])

	single := performSystemConfigJSONRequest(t, router, http.MethodGet, "/admin/system/configs/"+notificationEmailConfigKey, nil)
	require.NotContains(t, single.Body.String(), "smtp-secret")
	data := decodeJSONMap(t, single)["data"].(map[string]any)
	require.Equal(t, masked, data["value"])
	require.Equal(t, masked, data["display_value"])

	// Saving what the answer showed, with another host, keeps the password.
	edited := `{"host":"smtp.other","port":465,"username":"mailer","password":"********","from_name":"Anix","from_address":"noreply@example.test","encryption":"ssl"}`
	updated := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/"+notificationEmailConfigKey, map[string]any{"value": edited, "type": "json"})
	require.NotContains(t, updated.Body.String(), "smtp-secret")
	require.Equal(t, edited, decodeJSONMap(t, updated)["data"].(map[string]any)["value"])
	require.Equal(t, `{"host":"smtp.other","port":465,"username":"mailer","password":"smtp-secret","from_name":"Anix","from_address":"noreply@example.test","encryption":"ssl"}`, storedEmailConfig(t, db))

	// A new password replaces it.
	performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/"+notificationEmailConfigKey, map[string]any{"value": `{"host":"smtp.other","password":"rotated"}`})
	require.Equal(t, `{"host":"smtp.other","password":"rotated"}`, storedEmailConfig(t, db))
}
