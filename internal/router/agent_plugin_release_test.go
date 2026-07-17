package router

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAgentPluginReleaseRoutesRequireExactNodeAssignmentAndStreamRawBytes(t *testing.T) {
	router, _ := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	require.NoError(t, service.EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.Node{}))

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	artifact := []byte("raw-agent-plugin-artifact")
	artifactDigest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "router-agent-package", Name: "Router Agent Package", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, Architectures: []string{"linux-amd64"}, ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	node := model.Node{Name: "assigned-node", Host: "127.0.0.1", APIKey: "assigned-node-key", Status: model.NodeStatusOnline}
	other := model.Node{Name: "other-node", Host: "127.0.0.2", APIKey: "other-node-key", Status: model.NodeStatusOnline}
	disabled := model.Node{Name: "disabled-node", Host: "127.0.0.3", APIKey: "disabled-node-key", Status: model.NodeStatusDisabled}
	addressNode := model.Node{Name: "address-node", Host: "127.0.0.4", APIKey: "address-node-key", Status: model.NodeStatusOnline}
	for _, candidate := range []*model.Node{&node, &other, &disabled, &addressNode} {
		require.NoError(t, db.Create(candidate).Error)
	}
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: manifest.ID, Role: "telemetry", DesiredVersion: manifest.Version, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{
		NodeID: addressNode.ID, ServiceScope: "forward", PluginID: manifest.ID, Role: "telemetry", DesiredVersion: manifest.Version, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: manifest.ID, Target: "agent", DesiredVersion: manifest.Version, Enabled: true, State: "pending",
	}).Error)

	installConfigJSON, err := service.BuildAgentPluginInstallConfig(db, node.ID, manifest.ID, manifest.Version)
	require.NoError(t, err)
	var installConfig service.AgentPluginInstallConfig
	require.NoError(t, json.Unmarshal([]byte(installConfigJSON), &installConfig))

	request := httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL, nil)
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, request)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	request = httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL, nil)
	request.Header.Set("X-API-Key", other.APIKey)
	forbidden := httptest.NewRecorder()
	router.ServeHTTP(forbidden, request)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())

	request = httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL, nil)
	request.Header.Set("X-API-Key", disabled.APIKey)
	disabledResponse := httptest.NewRecorder()
	router.ServeHTTP(disabledResponse, request)
	require.Equal(t, http.StatusForbidden, disabledResponse.Code, disabledResponse.Body.String())

	request = httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL, nil)
	request.Header.Set("X-API-Key", node.APIKey)
	artifactResponse := httptest.NewRecorder()
	router.ServeHTTP(artifactResponse, request)
	require.Equal(t, http.StatusOK, artifactResponse.Code, artifactResponse.Body.String())
	require.Equal(t, artifact, artifactResponse.Body.Bytes())
	require.Equal(t, "application/octet-stream", artifactResponse.Header().Get("Content-Type"))
	require.Equal(t, manifest.ArtifactSHA256, artifactResponse.Header().Get("X-AnixOps-Artifact-SHA256"))
	require.Equal(t, release.Signature, artifactResponse.Header().Get("X-AnixOps-Signature"))
	require.Equal(t, "private, max-age=31536000, immutable", artifactResponse.Header().Get("Cache-Control"))

	request = httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL, nil)
	request.URL.RawQuery += "&api_key=" + node.APIKey
	queryCredential := httptest.NewRecorder()
	router.ServeHTTP(queryCredential, request)
	require.Equal(t, http.StatusUnauthorized, queryCredential.Code, "package URLs must never accept query credentials")

	request = httptest.NewRequest(http.MethodGet, installConfig.Manifest.URL, nil)
	request.Header.Set("X-API-Key", node.APIKey)
	manifestResponse := httptest.NewRecorder()
	router.ServeHTTP(manifestResponse, request)
	require.Equal(t, http.StatusOK, manifestResponse.Code, manifestResponse.Body.String())
	require.Equal(t, canonical, manifestResponse.Body.Bytes())
	require.Equal(t, "application/vnd.anixops.plugin-manifest+json", manifestResponse.Header().Get("Content-Type"))

	request = httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL+"&unexpected=1", nil)
	request.Header.Set("X-API-Key", node.APIKey)
	invalidAddress := httptest.NewRecorder()
	router.ServeHTTP(invalidAddress, request)
	require.Equal(t, http.StatusBadRequest, invalidAddress.Code)

	tamperedBytes := append([]byte(nil), artifact...)
	tamperedBytes[0] ^= 0x01
	require.NoError(t, db.Model(&model.PluginArtifact{}).Where("release_id = ?", release.ID).Update("data", tamperedBytes).Error)
	request = httptest.NewRequest(http.MethodGet, installConfig.Manifest.URL, nil)
	request.Header.Set("X-API-Key", node.APIKey)
	manifestAfterTamper := httptest.NewRecorder()
	router.ServeHTTP(manifestAfterTamper, request)
	require.Equal(t, http.StatusOK, manifestAfterTamper.Code, "manifest metadata path must not load or hash the artifact blob")
	wrongAddressURL := strings.Replace(installConfig.Artifact.URL, installConfig.Artifact.SHA256, strings.Repeat("0", 64), 1)
	request = httptest.NewRequest(http.MethodGet, wrongAddressURL, nil)
	request.Header.Set("X-API-Key", addressNode.APIKey)
	wrongAddress := httptest.NewRecorder()
	router.ServeHTTP(wrongAddress, request)
	require.Equal(t, http.StatusBadRequest, wrongAddress.Code, "wrong content address must be rejected before reading the blob")
	request = httptest.NewRequest(http.MethodGet, installConfig.Artifact.URL, nil)
	request.Header.Set("X-API-Key", node.APIKey)
	tampered := httptest.NewRecorder()
	router.ServeHTTP(tampered, request)
	require.Equal(t, http.StatusConflict, tampered.Code, tampered.Body.String())

	for attempt, expected := range []int{http.StatusOK, http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		request = httptest.NewRequest(http.MethodGet, installConfig.Manifest.URL, nil)
		request.Header.Set("X-API-Key", node.APIKey)
		limited := httptest.NewRecorder()
		router.ServeHTTP(limited, request)
		require.Equalf(t, expected, limited.Code, "per-node package limiter attempt %d: %s", attempt, limited.Body.String())
	}
}
