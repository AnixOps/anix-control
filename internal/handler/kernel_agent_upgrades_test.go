package handler

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentinstall"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// writeSignedAgentRelease puts a signed fake Agent release in dir/<tag>/
// as anix-agent's release job publishes it.
func writeSignedAgentRelease(t *testing.T, dir, tag string, private ed25519.PrivateKey) {
	t.Helper()
	release := filepath.Join(dir, tag)
	require.NoError(t, os.MkdirAll(release, 0o750))
	sign := func(data []byte) []byte {
		return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, data)) + "\n")
	}
	var sums bytes.Buffer
	for _, asset := range agentinstall.Assets {
		data := []byte("fake agent zip " + asset)
		require.NoError(t, os.WriteFile(filepath.Join(release, asset), data, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(release, asset+".sig"), sign(data), 0o600))
		sum := sha256.Sum256(data)
		sums.WriteString(hex.EncodeToString(sum[:]) + "  " + asset + "\n")
	}
	require.NoError(t, os.WriteFile(filepath.Join(release, agentinstall.SumsAssetName), sums.Bytes(), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(release, agentinstall.SumsSignatureAssetName), sign(sums.Bytes()), 0o600))
}

type upgradeTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	cfg    *config.Config
}

func newUpgradeTestEnv(t *testing.T) *upgradeTestEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(append(model.AgentUpgradeModels(), &model.User{}, &model.Node{}, &model.ForwardNode{},
		&model.AgentTransport{}, &model.OperationLog{})...))
	for _, user := range []model.User{{ID: 7, Email: "root@example.com", IsAdmin: 1}, {ID: 8, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1}} {
		user.Token = "token-" + strconv.FormatUint(uint64(user.ID), 10)
		user.UUID = "uuid-" + strconv.FormatUint(uint64(user.ID), 10)
		require.NoError(t, db.Create(&user).Error)
	}
	for id := uint(1); id <= 3; id++ {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: "proxy", APIKey: "key-" + strconv.Itoa(int(id)), Status: model.NodeStatusOnline}).Error)
		now := time.Now()
		require.NoError(t, db.Create(&model.AgentTransport{NodeKind: "proxy", NodeID: id, Transport: model.AgentTransportMTLSStream, FirstSeenAt: now, LastSeenAt: now}).Error)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	artifacts := t.TempDir()
	writeSignedAgentRelease(t, artifacts, "v4.2.0", private)
	cfg := &config.Config{}
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(public)
	cfg.AgentInstall.PublicURL = "https://ctl.example.com"
	cfg.AgentInstall.AgentVersion = "v4.2.0"
	cfg.AgentInstall.ArtifactDir = artifacts
	env := &upgradeTestEnv{db: db, cfg: cfg}
	previous := ReleaseVersion
	ReleaseVersion = "4.2.0"
	t.Cleanup(func() { ReleaseVersion = previous })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID := uint(7)
		if raw := c.GetHeader("X-Test-User"); raw != "" {
			parsed, _ := strconv.ParseUint(raw, 10, 32)
			userID = uint(parsed)
		}
		c.Set("user_id", userID)
		c.Set("email", "root@example.com")
	})
	handler := &AgentUpgradesHandler{db: func() *gorm.DB { return db }, config: func() *config.Config { return env.cfg }}
	router.POST("/upgrades", handler.Start)
	router.GET("/upgrades", handler.List)
	router.GET("/upgrades/:id", handler.Get)
	router.POST("/upgrades/:id/pause", handler.Pause)
	router.POST("/upgrades/:id/resume", handler.Resume)
	router.POST("/upgrades/:id/abort", handler.Abort)
	env.router = router
	return env
}

func (e *upgradeTestEnv) do(t *testing.T, method, path, body, user string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		request.Header.Set("X-Test-User", user)
	}
	recorder := httptest.NewRecorder()
	e.router.ServeHTTP(recorder, request)
	var answer map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer), recorder.Body.String())
	return recorder.Code, answer
}

func TestAgentUpgradesAPI(t *testing.T) {
	env := newUpgradeTestEnv(t)

	code, answer := env.do(t, http.MethodPost, "/upgrades", `{}`, "8")
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, "super_admin_required", answer["error"].(map[string]any)["code"])

	code, answer = env.do(t, http.MethodPost, "/upgrades", `{"target_version":"v4.1.9"}`, "")
	assert.Equal(t, http.StatusConflict, code, answer)
	assert.Equal(t, "agent_release_unverified", answer["error"].(map[string]any)["code"])

	code, answer = env.do(t, http.MethodPost, "/upgrades", `{"batches":[{"percent":50,"min_duration_seconds":1800},{"percent":100,"min_duration_seconds":1800}]}`, "")
	assert.Equal(t, http.StatusBadRequest, code, answer)

	code, answer = env.do(t, http.MethodPost, "/upgrades", `{"exclude":{"nodes":["proxy-3"]},"reason":"v4.2.0 rollout"}`, "")
	require.Equal(t, http.StatusCreated, code, answer)
	campaign := answer["data"].(map[string]any)
	id := campaign["id"].(string)
	assert.Equal(t, "v4.2.0", campaign["target_version"])
	assert.Equal(t, "running", campaign["status"])
	assert.EqualValues(t, 2, campaign["total"])
	artifacts := campaign["artifacts"].([]any)
	require.Len(t, artifacts, 2)
	assert.Equal(t, "https://ctl.example.com/install/agent/v4.2.0/anix-agent-linux-64.zip", artifacts[0].(map[string]any)["url"])
	nodes := campaign["nodes"].([]any)
	require.Len(t, nodes, 2)
	for _, node := range nodes {
		assert.NotEqual(t, "proxy-3", node.(map[string]any)["node"])
	}

	code, answer = env.do(t, http.MethodPost, "/upgrades", `{}`, "")
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "agent_upgrade_active", answer["error"].(map[string]any)["code"])

	code, answer = env.do(t, http.MethodGet, "/upgrades", "", "8")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, answer["data"].(map[string]any)["campaigns"].([]any), 1)
	code, _ = env.do(t, http.MethodGet, "/upgrades?limit=0", "", "")
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = env.do(t, http.MethodGet, "/upgrades/missing", "", "")
	assert.Equal(t, http.StatusNotFound, code)

	code, _ = env.do(t, http.MethodPost, "/upgrades/"+id+"/pause", "", "8")
	assert.Equal(t, http.StatusForbidden, code)
	code, answer = env.do(t, http.MethodPost, "/upgrades/"+id+"/pause", "", "")
	require.Equal(t, http.StatusOK, code, answer)
	assert.Equal(t, "paused", answer["data"].(map[string]any)["status"])
	code, answer = env.do(t, http.MethodPost, "/upgrades/"+id+"/pause", "", "")
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "agent_upgrade_state", answer["error"].(map[string]any)["code"])
	code, answer = env.do(t, http.MethodPost, "/upgrades/"+id+"/resume", "", "")
	require.Equal(t, http.StatusOK, code, answer)
	assert.Equal(t, "running", answer["data"].(map[string]any)["status"])
	code, answer = env.do(t, http.MethodPost, "/upgrades/"+id+"/abort", `{"rollback":true}`, "")
	require.Equal(t, http.StatusOK, code, answer)
	assert.Equal(t, "rolling_back", answer["data"].(map[string]any)["status"])
	code, answer = env.do(t, http.MethodPost, "/upgrades/"+id+"/abort", "", "")
	require.Equal(t, http.StatusOK, code, answer)
	assert.Equal(t, "aborted", answer["data"].(map[string]any)["status"])

	var actions []string
	require.NoError(t, env.db.Model(&model.OperationLog{}).Where("module = ?", "agent_upgrade").Order("id").Pluck("action", &actions).Error)
	assert.Equal(t, []string{"agent_upgrade_start", "agent_upgrade_pause", "agent_upgrade_resume", "agent_upgrade_abort", "agent_upgrade_abort"}, actions)
}

// Without public_url the request's origin must be https; a plain http
// Control cannot push releases.
func TestAgentUpgradesNeedHTTPS(t *testing.T) {
	env := newUpgradeTestEnv(t)
	env.cfg.AgentInstall.PublicURL = ""
	code, answer := env.do(t, http.MethodPost, "/upgrades", `{}`, "")
	assert.Equal(t, http.StatusConflict, code, answer)
	assert.Equal(t, "agent_install_unconfigured", answer["error"].(map[string]any)["code"])
}
