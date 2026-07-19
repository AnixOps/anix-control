package config_test

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouterSetupRejectsUnsafeSubscribePath(t *testing.T) {
	cfg := &config.Config{App: config.AppConfig{SubscribePath: " /api/v2/hidden/ "}}
	routerEngine := gin.New()

	require.PanicsWithValue(t, "invalid app.subscribe_path: subscription path must not overlap /api/v2", func() {
		router.Setup(routerEngine, cfg)
	})
}
