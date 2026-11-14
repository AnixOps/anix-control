package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/clientrelease"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// writeClientRelease lays a client release out in dir as Store leaves it
// (Store itself only trusts the built-in key; clientrelease tests it).
func writeClientRelease(t *testing.T, dir, version string, files map[string]string) {
	t.Helper()
	root := filepath.Join(dir, clientrelease.Product, version)
	require.NoError(t, os.MkdirAll(root, 0o750))
	sums := ""
	for name, content := range files {
		sum := sha256.Sum256([]byte(content))
		sums += hex.EncodeToString(sum[:]) + "  " + name + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, clientrelease.SumsName), []byte(sums), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, clientrelease.SignatureName), []byte("sig"), 0o600))
}

func TestClientReleasesUploadAndList(t *testing.T) {
	env := newUpgradeTestEnv(t)
	h := &ClientReleasesHandler{db: func() *gorm.DB { return env.db }, config: func() *config.Config { return env.cfg }}
	env.router.POST("/client-releases", h.Upload)
	env.router.GET("/client-releases", h.List)
	env.router.DELETE("/client-releases/:version", h.Delete)
	upload := func(user string, files map[string]string) (int, map[string]any) {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for name, content := range files {
			part, err := form.CreateFormFile("files", name)
			require.NoError(t, err)
			_, err = part.Write([]byte(content))
			require.NoError(t, err)
		}
		require.NoError(t, form.Close())
		request := httptest.NewRequest(http.MethodPost, "/client-releases", &body)
		request.Header.Set("Content-Type", form.FormDataContentType())
		if user != "" {
			request.Header.Set("X-Test-User", user)
		}
		recorder := httptest.NewRecorder()
		env.router.ServeHTTP(recorder, request)
		var answer map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer), recorder.Body.String())
		return recorder.Code, answer
	}
	errorCode := func(answer map[string]any) any { return answer["error"].(map[string]any)["code"] }
	name := "anixops-net-anx-0.3.0-beta.6-android-arm64.apk"
	sum := sha256.Sum256([]byte("apk"))
	release := map[string]string{
		clientrelease.SumsName:      hex.EncodeToString(sum[:]) + "  " + name + "\n",
		clientrelease.SignatureName: "-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n",
		name:                        "apk",
	}

	code, answer := upload("8", release)
	assert.Equal(t, http.StatusForbidden, code, answer)
	code, answer = upload("", map[string]string{name: "apk"})
	assert.Equal(t, http.StatusBadRequest, code, answer)
	assert.Equal(t, "invalid_request", errorCode(answer))
	// Only the built-in release key is trusted.
	code, answer = upload("", release)
	assert.Equal(t, http.StatusBadRequest, code, answer)
	assert.Equal(t, "client_release_invalid", errorCode(answer))
	assert.Empty(t, clientrelease.Versions(env.cfg.AgentInstall.ArtifactDir))

	writeClientRelease(t, env.cfg.AgentInstall.ArtifactDir, "0.3.0-beta.6", map[string]string{name: "apk"})
	recorder := httptest.NewRecorder()
	env.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/client-releases", nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var list struct {
		Data struct {
			Configured bool                    `json:"configured"`
			Latest     string                  `json:"latest"`
			Releases   []clientrelease.Release `json:"releases"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &list))
	assert.True(t, list.Data.Configured)
	assert.Equal(t, "0.3.0-beta.6", list.Data.Latest)
	require.Len(t, list.Data.Releases, 1)
	assert.Equal(t, "android", list.Data.Releases[0].Files[0].Platform)

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/client-releases/0.3.0-beta.6", nil)
	request.Header.Set("X-Test-User", "8")
	env.router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	recorder = httptest.NewRecorder()
	env.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/client-releases/0.3.0-beta.6", nil))
	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Empty(t, clientrelease.Versions(env.cfg.AgentInstall.ArtifactDir))

	env.cfg.AgentInstall.ArtifactDir = ""
	code, answer = upload("", release)
	assert.Equal(t, http.StatusConflict, code, answer)
	assert.Equal(t, "agent_install_unconfigured", errorCode(answer))
}

func TestClientReleasesSubscriberDownload(t *testing.T) {
	env := newUpgradeTestEnv(t)
	h := &ClientReleasesHandler{db: func() *gorm.DB { return env.db }, config: func() *config.Config { return env.cfg }}
	router := gin.New()
	router.GET("/s/:token/client", h.Latest)
	router.GET("/s/:token/client/:version/:file", h.Download)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://ctl.example.com"+path, nil))
		return recorder
	}

	// No release yet.
	assert.Equal(t, http.StatusNotFound, get("/s/token-7/client").Code)

	zip := "anixops-net-anx-0.3.0-beta.6-windows-x64-portable.zip"
	link := "anixops-net-anx-0.3.0-beta.6-ios-testflight.url"
	writeClientRelease(t, env.cfg.AgentInstall.ArtifactDir, "0.3.0-beta.6", map[string]string{
		zip:  "zip",
		link: "https://testflight.apple.com/join/abcdef\n",
	})

	recorder := get("/s/token-7/client")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var latest struct {
		Data struct {
			Version       string `json:"version"`
			TestFlightURL string `json:"testflight_url"`
			Files         []struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &latest))
	assert.Equal(t, "0.3.0-beta.6", latest.Data.Version)
	assert.Equal(t, "https://testflight.apple.com/join/abcdef", latest.Data.TestFlightURL)
	require.Len(t, latest.Data.Files, 2)
	assert.Equal(t, "windows-portable", latest.Data.Files[0].Platform)
	assert.Equal(t, "http://ctl.example.com/s/token-7/client/0.3.0-beta.6/"+zip, latest.Data.Files[0].URL)

	recorder = get("/s/token-7/client/0.3.0-beta.6/" + zip)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "zip", recorder.Body.String())
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), zip)
	assert.Equal(t, http.StatusOK, get("/s/token-7/client/0.3.0-beta.6/"+clientrelease.SumsName).Code)

	// Unknown tokens and files, and names outside the release, are 404.
	assert.Equal(t, http.StatusNotFound, get("/s/nobody/client").Code)
	assert.Equal(t, http.StatusNotFound, get("/s/nobody/client/0.3.0-beta.6/"+zip).Code)
	assert.Equal(t, http.StatusNotFound, get("/s/token-7/client/0.3.0-beta.6/anixops-net-anx-0.3.0-beta.6-linux-x64.tar.gz").Code)
	assert.Equal(t, http.StatusNotFound, get("/s/token-7/client/0.3.0-beta.6/..%2F..%2Fkernel.db").Code)

	// A banned or expired subscriber may not download.
	expired := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, env.db.Model(&model.User{}).Where("id = ?", 7).Update("expired_at", expired).Error)
	assert.Equal(t, http.StatusForbidden, get("/s/token-7/client").Code)
	assert.Equal(t, http.StatusForbidden, get("/s/token-7/client/0.3.0-beta.6/"+zip).Code)
}
