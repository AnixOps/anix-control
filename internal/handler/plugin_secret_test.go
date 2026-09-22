package handler

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPluginSecretAPIExposesMetadataWithoutMaterial(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{SecretEncryption: config.PluginSecretEncryptionConfig{
		ActiveKeyID: "primary",
		Keys:        map[string]string{"primary": base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))},
	}}})
	t.Cleanup(func() { config.Set(previousConfig) })
	handler := &KernelHandler{db: db}
	plaintext := "api-private-key-material"
	encoded := base64.StdEncoding.EncodeToString([]byte(plaintext))
	body := `{"id":"api-mesh","name":"API mesh","files":[{"name":"client.key","content_base64":"` + encoded + `"}]}`
	response := performKernelHandlerRequestWithSetup(t, http.MethodPost, "/api/v3/secrets", body, "/api/v3/secrets", handler.CreatePluginSecret, func(c *gin.Context) {
		c.Set("user_id", uint(41))
	})
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), plaintext)
	require.NotContains(t, response.Body.String(), encoded)
	require.Contains(t, response.Body.String(), `"sha256"`)

	var material model.PluginSecretMaterial
	require.NoError(t, db.First(&material).Error)
	require.NotContains(t, string(material.Ciphertext), plaintext)
	var audits []model.PluginSecretAudit
	require.NoError(t, db.Find(&audits).Error)
	require.Len(t, audits, 1)
	require.NotContains(t, audits[0].Detail, plaintext)
	require.NotContains(t, strings.ToLower(audits[0].Detail), "content")
}

func TestPluginSecretAPIFailsClosedWithoutKeyring(t *testing.T) {
	db := newKernelHandlerTestDB(t)
	previousConfig := config.Get()
	config.Set(&config.Config{})
	t.Cleanup(func() { config.Set(previousConfig) })
	handler := &KernelHandler{db: db}
	response := performKernelHandlerRequest(t, http.MethodGet, "/api/v3/secrets", "", "/api/v3/secrets", handler.ListPluginSecrets)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "plugin_secret_keyring_unavailable")
}
