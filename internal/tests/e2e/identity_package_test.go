package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInstallIdentityPlatformE2EPackageRoutesRegistrationThroughBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache.InitMemory()
	cfg := &config.Config{
		Env:      "test",
		Database: config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"},
		JWT:      config.JWTConfig{Secret: "identity-e2e-test-secret", Expire: 3600},
		App:      config.AppConfig{APIToken: "identity-e2e-api-token", SubscribePath: "s"},
	}
	config.Set(cfg)
	require.NoError(t, database.Init(&cfg.Database))
	t.Cleanup(func() { requireDatabaseClosed(t) })
	require.NoError(t, database.Get().AutoMigrate(&model.User{}))

	restoreHost := installIdentityPlatformE2EPackage(t, cfg)
	t.Cleanup(restoreHost)

	engine := gin.New()
	router.Setup(engine, cfg)
	body, err := json.Marshal(map[string]string{"email": "bridge@example.test", "password": "password123"})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v2/register", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	data := requirePanelDataMap(t, response.Body.Bytes())
	require.Equal(t, "bridge@example.test", data["email"])
	require.NotEmpty(t, data["token"])
}
