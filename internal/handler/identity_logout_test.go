package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/middleware"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogoutEndsOnlyTheCallingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.Set(&config.Config{JWT: config.JWTConfig{Secret: "logout-secret", Expire: 3600}})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.IdentityRevocation{}, &model.IdentitySessionRevocation{}))
	authn.SetDefaultStore(authn.NewStore(db, 24*time.Hour))
	t.Cleanup(func() { authn.SetDefaultStore(nil) })
	first, err := utils.GenerateToken(7, "a@example.test", false, "logout-secret", 3600)
	require.NoError(t, err)
	second, err := utils.GenerateToken(7, "a@example.test", false, "logout-secret", 3600)
	require.NoError(t, err)

	router := gin.New()
	router.POST("/logout", middleware.JWTAuth(), IdentityLogout)
	router.GET("/me", middleware.JWTAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	call := func(method, path, token string) int {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	require.Equal(t, http.StatusNoContent, call("GET", "/me", first))
	require.Equal(t, http.StatusOK, call("POST", "/logout", first))
	require.Equal(t, http.StatusUnauthorized, call("GET", "/me", first), "the logged out session is refused")
	require.Equal(t, http.StatusNoContent, call("GET", "/me", second), "other sessions of the user continue")
}
