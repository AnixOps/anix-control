package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func systemAudit(t *testing.T, db *gorm.DB) []model.OperationLog {
	t.Helper()
	var entries []model.OperationLog
	require.NoError(t, db.Where("module = ?", "system").Order("id").Find(&entries).Error)
	return entries
}

// E-mail and invite configuration writes record the system configuration
// handler's audit entry for the key they write: module system, the action,
// the key, whether the SMTP password is set and was kept, and never a
// secret value.
func TestEmailAndInviteConfigWritesRecordTheSystemConfigAudit(t *testing.T) {
	db, _, router := setupSystemConfigSecurityTest(t)
	require.NoError(t, db.AutoMigrate(&model.InviteConfig{}))
	require.NoError(t, db.Exec("DELETE FROM v2_invite_config").Error)
	seedEmailConfig(t, db, storedEmailConfigValue)
	notifications := NewNotificationHandler()
	router.PUT("/admin/notification/email/config", notifications.UpdateEmailConfig)
	router.PUT("/admin/invite/config", NewInviteHandler().UpdateConfig)

	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/notification/email/config",
		map[string]any{"host": "smtp.test", "port": 465, "password": "********", "from_address": "noreply@example.test"})
	require.Equal(t, float64(0), decodeJSONMap(t, recorder)["code"], recorder.Body.String())
	recorder = performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/notification/email/config",
		map[string]any{"host": "smtp.test", "port": 465, "password": "rotated", "from_address": "noreply@example.test"})
	require.Equal(t, float64(0), decodeJSONMap(t, recorder)["code"], recorder.Body.String())
	recorder = performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/invite/config", map[string]any{"code_prefix": "VIP"})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	entries := systemAudit(t, db)
	require.Len(t, entries, 3)
	var emailRow, inviteRow model.SystemConfig
	require.NoError(t, db.Where("key = ?", notificationEmailConfigKey).Take(&emailRow).Error)
	require.NoError(t, db.Where("key = ?", inviteFrontendConfigKey).Take(&inviteRow).Error)
	for _, entry := range entries {
		require.Equal(t, "system_config", entry.TargetType)
		require.Equal(t, uint(99), *entry.UserID)
		require.Equal(t, "admin@example.com", entry.Username)
		require.NotContains(t, entry.Content, "smtp-secret")
		require.NotContains(t, entry.Content, "rotated")
	}
	require.Equal(t, "update", entries[0].Action)
	require.Equal(t, emailRow.ID, *entries[0].TargetID)
	require.JSONEq(t, `{"key":"notification.email.config","group":"notification","type":"json","sensitive":false,"has_value":true,
		"preserve_existing":true,"masked_fields":["password"],"masked_fields_with_value":["password"]}`, entries[0].Content)
	require.JSONEq(t, `{"key":"notification.email.config","group":"notification","type":"json","sensitive":false,"has_value":true,
		"preserve_existing":false,"masked_fields":["password"],"masked_fields_with_value":["password"]}`, entries[1].Content)
	require.Equal(t, "create", entries[2].Action)
	require.Equal(t, inviteRow.ID, *entries[2].TargetID)
	require.JSONEq(t, `{"key":"invite.frontend.config","group":"invite","type":"json","sensitive":false,"has_value":true,"preserve_existing":false}`, entries[2].Content)
}

// A legacy handler the package bridge relays to gets the actor's id only;
// its audit entries name the user by e-mail all the same.
func TestBridgedAuditEntriesNameTheUser(t *testing.T) {
	db, handler, _ := setupSystemConfigSecurityTest(t)
	user := model.User{Email: "bridged-" + time.Now().Format("150405.000000000") + "@example.test", Token: "bridged-" + time.Now().Format("150405.000000000"), UUID: "b-" + time.Now().Format("150405.000000000")}
	require.NoError(t, db.Create(&user).Error)
	t.Cleanup(func() { db.Delete(&model.User{}, user.ID) })

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	router.PUT("/admin/system/configs/:key", handler.SetConfig)
	recorder := performSystemConfigJSONRequest(t, router, http.MethodPut, "/admin/system/configs/site.name", map[string]any{"value": "Anix"})
	require.Equal(t, float64(0), decodeJSONMap(t, recorder)["code"], recorder.Body.String())

	entries := systemAudit(t, db)
	require.Len(t, entries, 1)
	require.Equal(t, user.Email, entries[0].Username)

	// An unknown user is recorded without a username.
	missing := gin.New()
	missing.Use(func(c *gin.Context) {
		c.Set("user_id", uint(987654))
		c.Next()
	})
	missing.PUT("/admin/system/configs/:key", handler.SetConfig)
	performSystemConfigJSONRequest(t, missing, http.MethodPut, "/admin/system/configs/site.name", map[string]any{"value": "Anix 2"})
	entries = systemAudit(t, db)
	require.Len(t, entries, 2)
	require.Empty(t, entries[1].Username)
}
