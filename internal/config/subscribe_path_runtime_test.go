package config_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouterSetupRejectsUnsafeSubscribePath(t *testing.T) {
	for _, subscribePath := range []string{" /api/v2/hidden/ ", "api/:segment", "api/*rest"} {
		t.Run(subscribePath, func(t *testing.T) {
			cfg := &config.Config{App: config.AppConfig{SubscribePath: subscribePath}}
			routerEngine := gin.New()

			require.Panics(t, func() {
				router.Setup(routerEngine, cfg)
			})
			require.Empty(t, routerEngine.Routes())
		})
	}
}

func TestGinParameterSubscriptionPathWouldOverlapReservedV2Namespace(t *testing.T) {
	routerEngine := gin.New()
	routerEngine.GET("/api/:segment/:token", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v2/token", nil)
	response := httptest.NewRecorder()
	routerEngine.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
}
