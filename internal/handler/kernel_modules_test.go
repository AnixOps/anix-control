package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
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
	router.POST("/enrollments", modules.CreateEnrollment)
	router.GET("/enrollments", modules.ListEnrollments)
	router.DELETE("/enrollments/:id", modules.RevokeEnrollment)
	router.POST("/ca/rotate", modules.RotateCA)
	return router
}

func moduleTestAuthority(t *testing.T) *modulepki.Authority {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ServiceCA{}, &model.ModuleEnrollment{}, &model.ModuleCertificate{}))
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

func TestModuleEnrollmentAdministration(t *testing.T) {
	authority := moduleTestAuthority(t)
	router := newModuleTestRouter(t, func() (*modulepki.Authority, error) { return authority, nil })

	created := serveModuleRequest(router, http.MethodPost, "/enrollments", `{"package_id":"identity-platform","ttl_seconds":3600}`)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.Equal(t, "no-store", created.Header().Get("Cache-Control"))
	var response struct {
		Data struct {
			Enrollment map[string]any `json:"enrollment"`
			Credential string         `json:"credential"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
	require.True(t, strings.HasPrefix(response.Data.Credential, "anixenr_"))
	require.NotContains(t, response.Data.Enrollment, "credential_hash")
	require.EqualValues(t, 7, response.Data.Enrollment["created_by"])
	id := response.Data.Enrollment["id"].(string)

	listed := serveModuleRequest(router, http.MethodGet, "/enrollments", "")
	require.Equal(t, http.StatusOK, listed.Code)
	require.NotContains(t, listed.Body.String(), response.Data.Credential)
	require.Contains(t, listed.Body.String(), id)

	require.Equal(t, http.StatusNoContent, serveModuleRequest(router, http.MethodDelete, "/enrollments/"+id, "").Code)
	require.Equal(t, http.StatusNotFound, serveModuleRequest(router, http.MethodDelete, "/enrollments/missing", "").Code)
	require.Equal(t, http.StatusBadRequest, serveModuleRequest(router, http.MethodPost, "/enrollments", `{"package_id":"Bad_ID","ttl_seconds":60}`).Code)
	require.Equal(t, http.StatusBadRequest, serveModuleRequest(router, http.MethodPost, "/enrollments", `{"package_id":"identity-platform"}`).Code)

	rotated := serveModuleRequest(router, http.MethodPost, "/ca/rotate", "")
	require.Equal(t, http.StatusOK, rotated.Code)
	require.Contains(t, rotated.Body.String(), `"state":"next"`)
	require.NotContains(t, rotated.Body.String(), "sealed")
}

func TestModuleAdministrationWithoutBuiltinPKI(t *testing.T) {
	router := newModuleTestRouter(t, func() (*modulepki.Authority, error) { return nil, modulepki.ErrBuiltinPKIDisabled })
	response := serveModuleRequest(router, http.MethodPost, "/enrollments", `{"package_id":"identity-platform","ttl_seconds":60}`)
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "module_pki_disabled")
}

func TestModuleRuntimeAdministration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Plugin{}, &model.PluginRuntime{}))
	require.NoError(t, db.Create(&model.Plugin{ID: "knowledge", Name: "Knowledge", Publisher: "AnixOps", Official: true}).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)) })
	modules := &ModuleHandler{db: func() *gorm.DB { return db }, remote: func() bool { return false }}
	router.GET("/runtimes", modules.ListRuntimes)
	router.PUT("/runtimes/:plugin_id", modules.SetRuntime)

	require.Equal(t, http.StatusOK, serveModuleRequest(router, http.MethodPut, "/runtimes/knowledge", `{"runtime":"local"}`).Code)
	disabled := serveModuleRequest(router, http.MethodPut, "/runtimes/knowledge", `{"runtime":"remote"}`)
	require.Equal(t, http.StatusConflict, disabled.Code)
	require.Contains(t, disabled.Body.String(), "module_runtime_disabled")
	require.Equal(t, http.StatusNotFound, serveModuleRequest(router, http.MethodPut, "/runtimes/ticket", `{"runtime":"local"}`).Code)
	require.Equal(t, http.StatusBadRequest, serveModuleRequest(router, http.MethodPut, "/runtimes/knowledge", `{"runtime":"cloud"}`).Code)
	listed := serveModuleRequest(router, http.MethodGet, "/runtimes", "")
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listed.Body.String(), `"plugin_id":"knowledge","runtime":"local","updated_by":7`)
}
