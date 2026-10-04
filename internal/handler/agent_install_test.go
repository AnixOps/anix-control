package handler

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
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
	"github.com/AnixOps/anix-control/v4/internal/agentpki"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type installTestEnv struct {
	router  *gin.Engine
	db      *gorm.DB
	pki     *agentpki.Service
	cfg     *config.Config
	node    model.Node
	forward model.ForwardNode
	private ed25519.PrivateKey
}

func newInstallTestEnv(t *testing.T) *installTestEnv {
	t.Helper()
	pki, db, node := agentPKITestService(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	for _, user := range []model.User{
		{ID: 7, Email: "root@example.com", IsAdmin: 1},
		{ID: 8, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1},
		{ID: 9, Email: "banned@example.com", IsAdmin: 1, Banned: 1},
		{ID: 10, Email: "user@example.com"},
	} {
		user.Token = "token-" + strconv.FormatUint(uint64(user.ID), 10)
		user.UUID = "uuid-" + strconv.FormatUint(uint64(user.ID), 10)
		require.NoError(t, db.Create(&user).Error)
	}
	forward := model.ForwardNode{Name: "relay", Host: "198.51.100.4", Port: 443, Enabled: true}
	require.NoError(t, db.Create(&forward).Error)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg := &config.Config{}
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(public)
	cfg.AgentInstall.PublicURL = "https://ctl.example.com"
	cfg.AgentInstall.AgentVersion = "v4.2.0"
	cfg.GRPC.Port = 50051
	env := &installTestEnv{db: db, pki: pki, cfg: cfg, node: node, forward: forward, private: private}

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
	handler := &AgentInstallHandler{
		pki:    func() (*agentpki.Service, error) { return pki, nil },
		config: func() *config.Config { return env.cfg },
		db:     func() *gorm.DB { return db },
	}
	router.POST("/install-tokens", handler.CreateInstallToken)
	router.GET("/install.sh", handler.Script)
	router.GET("/install.sh.sig", handler.Signature)
	router.GET("/install/agent.env", handler.Metadata)
	router.GET("/install/agent/:tag/:asset", handler.Artifact)
	env.router = router
	return env
}

func (e *installTestEnv) post(t *testing.T, user uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/install-tokens", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-User", strconv.FormatUint(uint64(user), 10))
	e.router.ServeHTTP(recorder, request)
	return recorder
}

func (e *installTestEnv) get(path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	e.router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

type installTokenBody struct {
	Data struct {
		Enrollment   map[string]any         `json:"enrollment"`
		Credential   string                 `json:"credential"`
		Node         string                 `json:"node"`
		ExpiresAt    time.Time              `json:"expires_at"`
		AgentVersion string                 `json:"agent_version"`
		Commands     []agentinstall.Command `json:"commands"`
		Script       struct {
			URL          string `json:"url"`
			SignatureURL string `json:"signature_url"`
			Signed       bool   `json:"signed"`
		} `json:"script"`
	} `json:"data"`
}

func decodeInstallToken(t *testing.T, response *httptest.ResponseRecorder) installTokenBody {
	t.Helper()
	var body installTokenBody
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body), response.Body.String())
	return body
}

func TestInstallTokenIssuesSingleUseNodeBoundToken(t *testing.T) {
	env := newInstallTestEnv(t)
	nodeName := "forward-" + strconv.FormatUint(uint64(env.forward.ID), 10)
	before := time.Now()

	response := env.post(t, 7, `{"node":"`+nodeName+`"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	body := decodeInstallToken(t, response)
	data := body.Data
	assert.True(t, strings.HasPrefix(data.Credential, "anixagt_"))
	assert.Equal(t, nodeName, data.Node)
	assert.Equal(t, "v4.2.0", data.AgentVersion)
	assert.WithinDuration(t, before.Add(time.Hour), data.ExpiresAt, time.Minute, "the default lifetime is one hour (H18)")
	assert.NotContains(t, data.Enrollment, "credential_hash")
	assert.False(t, data.Script.Signed)
	assert.Equal(t, "https://ctl.example.com/install.sh", data.Script.URL)

	require.Len(t, data.Commands, 3)
	want := "curl -fsSL https://ctl.example.com/install.sh | sudo bash -s -- --control https://ctl.example.com --node " +
		nodeName + " --token " + data.Credential
	assert.Equal(t, agentinstall.MirrorControl, data.Commands[0].Mirror)
	assert.Equal(t, want, data.Commands[0].Command)
	assert.Equal(t, want+" --mirror cn", data.Commands[1].Command)
	assert.Equal(t, "curl -fsSL https://github.com/AnixOps/anix-control/releases/download/"+
		"v"+strings.TrimPrefix(ReleaseVersion, "v")+"/agent-install.sh | sudo bash -s -- --control https://ctl.example.com --node "+
		nodeName+" --token "+data.Credential+" --mirror github", data.Commands[2].Command)

	// Only the hash is stored.
	var stored model.AgentEnrollment
	require.NoError(t, env.db.Where("node_kind = ? AND node_id = ?", "forward", env.forward.ID).First(&stored).Error)
	require.NotNil(t, stored.CredentialHash)
	assert.NotEqual(t, data.Credential, *stored.CredentialHash)
	assert.EqualValues(t, 7, stored.CreatedBy)

	// The issue is audited without the token.
	var audit model.OperationLog
	require.NoError(t, env.db.Where("action = ?", agentpki.AuditActionTokenIssue).First(&audit).Error)
	assert.Equal(t, "root@example.com", audit.Username)
	assert.NotContains(t, audit.Content, data.Credential)

	// Single use: the first enrollment consumes it, a second is refused.
	enroll := func() error {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: nodeName}}, key)
		require.NoError(t, err)
		_, err = env.pki.Enroll(t.Context(), agentpki.EnrollRequest{
			Bootstrap: agentpki.Bootstrap{Method: model.AgentEnrollmentMethodCredential, Secret: data.Credential},
			CSRDER:    csr, AgentVersion: "v4.2.0", InstanceID: "test",
		})
		return err
	}
	require.NoError(t, enroll())
	assert.ErrorIs(t, enroll(), agentpki.ErrEnrollmentRejected)
}

func TestInstallTokenAuthorizationAndBounds(t *testing.T) {
	env := newInstallTestEnv(t)
	proxy := "proxy-" + strconv.FormatUint(uint64(env.node.ID), 10)
	for _, user := range []uint{8, 9, 10, 4242} {
		response := env.post(t, user, `{"node":"`+proxy+`"}`)
		assert.Equal(t, http.StatusForbidden, response.Code, "user %d", user)
		assert.Contains(t, response.Body.String(), "super_admin_required")
	}
	var count int64
	require.NoError(t, env.db.Model(&model.AgentEnrollment{}).Count(&count).Error)
	assert.Zero(t, count, "a refused caller issues nothing")

	for body, code := range map[string]int{
		`{"node":"` + proxy + `","ttl_seconds":60}`:     http.StatusCreated,
		`{"node":"` + proxy + `","ttl_seconds":604800}`: http.StatusCreated,
		`{"node":"` + proxy + `","ttl_seconds":604801}`: http.StatusBadRequest,
		`{"node":"` + proxy + `","ttl_seconds":59}`:     http.StatusBadRequest,
		`{"node":"` + proxy + `","ttl_seconds":-1}`:     http.StatusBadRequest,
		`{"node":"` + proxy + `","ttl_seconds":"1h"}`:   http.StatusBadRequest,
		`{"node":"proxy-4242"}`:                         http.StatusNotFound,
		`{"node":"module-1"}`:                           http.StatusBadRequest,
		`{"node":"proxy-0"}`:                            http.StatusBadRequest,
		`{"node":"proxy-1 --token x"}`:                  http.StatusBadRequest,
		`{}`:                                            http.StatusBadRequest,
	} {
		response := env.post(t, 7, body)
		assert.Equal(t, code, response.Code, body+": "+response.Body.String())
	}

	week := decodeInstallToken(t, env.post(t, 7, `{"node":"`+proxy+`","ttl_seconds":604800}`))
	assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), week.Data.ExpiresAt, time.Minute)

	env.node.Status = model.NodeStatusDisabled
	require.NoError(t, env.db.Save(&env.node).Error)
	response := env.post(t, 7, `{"node":"`+proxy+`"}`)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestInstallTokenRendersIPv6AndPorts(t *testing.T) {
	env := newInstallTestEnv(t)
	env.cfg.AgentInstall.PublicURL = "https://[2001:db8::1]:8443/"
	proxy := "proxy-" + strconv.FormatUint(uint64(env.node.ID), 10)
	response := env.post(t, 7, `{"node":"`+proxy+`"}`)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	data := decodeInstallToken(t, response).Data
	assert.Equal(t, "curl -fsSLg 'https://[2001:db8::1]:8443/install.sh' | sudo bash -s -- --control 'https://[2001:db8::1]:8443' --node "+
		proxy+" --token "+data.Credential, data.Commands[0].Command)
	for _, command := range data.Commands {
		assert.Contains(t, command.Command, " | sudo bash -s -- --control ", "the -- ends bash's options")
	}

	metadata := env.get("/install/agent.env")
	require.Equal(t, http.StatusOK, metadata.Code, metadata.Body.String())
	assert.Contains(t, metadata.Body.String(), "grpc_target [2001:db8::1]:50051\n")
}

func TestInstallTokenNeedsAnHTTPSAddress(t *testing.T) {
	env := newInstallTestEnv(t)
	proxy := "proxy-" + strconv.FormatUint(uint64(env.node.ID), 10)

	// Without public_url the request's origin is used; a plain http
	// origin is refused before a token is issued.
	env.cfg.AgentInstall.PublicURL = ""
	response := env.post(t, 7, `{"node":"`+proxy+`"}`)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "agent_install_unconfigured")
	var count int64
	require.NoError(t, env.db.Model(&model.AgentEnrollment{}).Count(&count).Error)
	assert.Zero(t, count)

	env.cfg.AgentInstall.PublicURL = "https://ctl.example.com"
	env.cfg.AgentInstall.AgentVersion = ""
	previous := ReleaseVersion
	ReleaseVersion = "dev"
	t.Cleanup(func() { ReleaseVersion = previous })
	response = env.post(t, 7, `{"node":"`+proxy+`"}`)
	assert.Equal(t, http.StatusConflict, response.Code, "a development build names no Agent release")
}

func TestInstallTokenWithoutBuiltinPKI(t *testing.T) {
	env := newInstallTestEnv(t)
	handler := &AgentInstallHandler{
		pki:    func() (*agentpki.Service, error) { return nil, agentpki.ErrDisabled },
		config: func() *config.Config { return env.cfg },
		db:     func() *gorm.DB { return env.db },
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", uint(7)) })
	router.POST("/install-tokens", handler.CreateInstallToken)
	response := serveModuleRequest(router, http.MethodPost, "/install-tokens", `{"node":"proxy-1"}`)
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "agent_pki_disabled")
}

func TestInstallScriptAndSignature(t *testing.T) {
	env := newInstallTestEnv(t)
	script := env.get("/install.sh")
	require.Equal(t, http.StatusOK, script.Code)
	assert.Equal(t, agentinstall.Script, script.Body.Bytes())
	assert.Equal(t, "text/x-shellscript; charset=utf-8", script.Header().Get("Content-Type"))

	// No signature file: 404, pointing at the release asset.
	missing := env.get("/install.sh.sig")
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assert.Contains(t, missing.Body.String(), "signature_unavailable")

	dir := t.TempDir()
	stale := filepath.Join(dir, "stale.sig")
	require.NoError(t, os.WriteFile(stale, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(env.private, []byte("old script")))+"\n"), 0o644))
	env.cfg.AgentInstall.SignatureFile = stale
	assert.Equal(t, http.StatusNotFound, env.get("/install.sh.sig").Code, "a stale signature is never served")

	good := filepath.Join(dir, "agent-install.sh.sig")
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(env.private, agentinstall.Script)) + "\n"
	require.NoError(t, os.WriteFile(good, []byte(signature), 0o644))
	env.cfg.AgentInstall.SignatureFile = good
	served := env.get("/install.sh.sig")
	require.Equal(t, http.StatusOK, served.Code)
	assert.Equal(t, signature, served.Body.String())

	proxy := "proxy-" + strconv.FormatUint(uint64(env.node.ID), 10)
	assert.True(t, decodeInstallToken(t, env.post(t, 7, `{"node":"`+proxy+`"}`)).Data.Script.Signed)
}

func TestInstallMetadataAndArtifacts(t *testing.T) {
	env := newInstallTestEnv(t)
	env.cfg.AgentInstall.CNMirrorURL = "https://mirror.example.cn/anix-agent"
	metadata := env.get("/install/agent.env")
	require.Equal(t, http.StatusOK, metadata.Code)
	assert.Equal(t, "format 1\nagent_version v4.2.0\ngrpc_target ctl.example.com:50051\n"+
		"asset amd64 anix-agent-linux-64.zip\nasset arm64 anix-agent-linux-arm64-v8a.zip\n"+
		"source cn https://mirror.example.cn/anix-agent\n"+
		"source github https://github.com/AnixOps/anix-agent/releases/download\n", metadata.Body.String())

	dir := t.TempDir()
	env.cfg.AgentInstall.ArtifactDir = dir
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "v4.2.0"), 0o755))
	for asset, digest := range map[string]string{"anix-agent-linux-64.zip": strings.Repeat("a", 64), "anix-agent-linux-arm64-v8a.zip": strings.Repeat("b", 64)} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "v4.2.0", asset), []byte("zip "+asset), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "v4.2.0", asset+".dgst"), []byte("SHA2-256= "+digest+"\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644))
	metadata = env.get("/install/agent.env")
	assert.Contains(t, metadata.Body.String(), "source control https://ctl.example.com/install/agent\n")
	assert.Contains(t, metadata.Body.String(), "sha256 anix-agent-linux-64.zip "+strings.Repeat("a", 64)+"\n")

	asset := env.get("/install/agent/v4.2.0/anix-agent-linux-64.zip")
	require.Equal(t, http.StatusOK, asset.Code)
	assert.Equal(t, "zip anix-agent-linux-64.zip", asset.Body.String())
	for _, path := range []string{
		"/install/agent/v4.2.0/secret.txt",
		"/install/agent/v4.2.0/..%2Fsecret.txt",
		"/install/agent/latest/anix-agent-linux-64.zip",
		"/install/agent/v4.2.1/anix-agent-linux-64.zip",
	} {
		assert.Equal(t, http.StatusNotFound, env.get(path).Code, path)
	}

	proxy := "proxy-" + strconv.FormatUint(uint64(env.node.ID), 10)
	commands := decodeInstallToken(t, env.post(t, 7, `{"node":"`+proxy+`"}`)).Data.Commands
	assert.True(t, commands[0].Available)
	assert.True(t, commands[1].Available)
}
