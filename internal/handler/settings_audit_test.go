package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
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
