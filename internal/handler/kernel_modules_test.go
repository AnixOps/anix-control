package handler

import (
	"bytes"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newModuleTestRouter(t *testing.T, authority func() (*modulepki.Authority, error)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)) })
	modules := &ModuleHandler{authority: authority}
	router.POST("/ca/rotate", modules.RotateCA)
	return router
}

func moduleTestAuthority(t *testing.T) *modulepki.Authority {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}))
	kek := make([]byte, 32)
	_, err = rand.Read(kek)
	require.NoError(t, err)
	authority, err := modulepki.New(modulepki.Options{DB: db, Cluster: "prod", KEK: kek})
	require.NoError(t, err)
	require.NoError(t, authority.Ensure(t.Context()))
	return authority
}

func serveModuleRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestModuleCARotation(t *testing.T) {
	authority := moduleTestAuthority(t)
	router := newModuleTestRouter(t, func() (*modulepki.Authority, error) { return authority, nil })
	rotated := serveModuleRequest(router, http.MethodPost, "/ca/rotate", "")
	require.Equal(t, http.StatusOK, rotated.Code)
	require.Contains(t, rotated.Body.String(), `"state":"next"`)
	require.NotContains(t, rotated.Body.String(), "sealed")
}

func TestModuleCARotationWithoutBuiltinPKI(t *testing.T) {
	router := newModuleTestRouter(t, func() (*modulepki.Authority, error) { return nil, modulepki.ErrBuiltinPKIDisabled })
	response := serveModuleRequest(router, http.MethodPost, "/ca/rotate", "")
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "module_pki_disabled")
}
