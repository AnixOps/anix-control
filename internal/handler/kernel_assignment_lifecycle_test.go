package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type kernelAgentAssignmentFixture struct {
	db           *gorm.DB
	publicKey    ed25519.PublicKey
	privateKey   ed25519.PrivateKey
	node         model.Node
	installation model.PluginInstallation
	assignment   model.NodeServiceAssignment
	pluginID     string
}

func newKernelAgentAssignmentFixture(t *testing.T, pluginID string) *kernelAgentAssignmentFixture {
	t.Helper()
	db := newKernelHandlerTestDB(t, &model.Node{})
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	registerKernelAgentRelease(t, db, privateKey, publicKey, pluginID, "1.0.0")
	node := model.Node{Name: pluginID + "-node", Host: "127.0.0.1", Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	installation := model.PluginInstallation{
		PluginID: pluginID, Target: "agent", DesiredVersion: "1.0.0", State: "healthy", Enabled: true,
		LifecycleGeneration: 1, ConfigRevision: 2, ObservedVersion: "1.0.0",
	}
	require.NoError(t, db.Create(&installation).Error)
	configJSON := `{"interval":30}`
	configHash, err := service.HashKernelOperationConfig(configJSON)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginConfiguration{
		InstallationID: installation.ID, Revision: 2, ConfigJSON: configJSON, ConfigHash: configHash, UpdatedBy: 1,
	}).Error)
	assignment := model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "forward", PluginID: pluginID, Role: "telemetry", DesiredVersion: "1.0.0",
		DesiredConfigRevision: 2, Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&assignment).Error)
	_, _, err = service.SyncNodePluginLifecycle(db, node.ID, pluginID, false, time.Now())
	require.NoError(t, err)
	return &kernelAgentAssignmentFixture{
		db: db, publicKey: publicKey, privateKey: privateKey, node: node,
		installation: installation, assignment: assignment, pluginID: pluginID,
	}
}

func registerKernelAgentRelease(t *testing.T, db *gorm.DB, privateKey ed25519.PrivateKey, publicKey ed25519.PublicKey, pluginID, version string) {
	t.Helper()
	artifact := []byte("signed-agent-package-" + pluginID + "-" + version)
	digest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: version, APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
		ConfigSchema: []byte(`{"type":"object","properties":{"interval":{"type":"integer"}}}`),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
}

// An assignment change and an Agent configuration change record the desired
// state and queue no Agent operation: Control does not dispatch Agent
// plugins.
func TestKernelAgentChangesRecordDesiredStateAndQueueNothing(t *testing.T) {
	fixture := newKernelAgentAssignmentFixture(t, "handler-agent-desired-state")
	configPath := "/plugin-installations/" + strconv.FormatUint(uint64(fixture.installation.ID), 10) + "/config"
	recorder := performKernelHandlerRequest(t, http.MethodPut, configPath, `{"config":{"interval":45},"expected_revision":2}`,
		"/plugin-installations/:id/config", (&KernelHandler{db: fixture.db}).UpdatePluginInstallationConfiguration)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var assignment model.NodeServiceAssignment
	require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
	require.Equal(t, int64(3), assignment.DesiredConfigRevision)
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).Error)
	require.Equal(t, int64(3), lifecycle.DesiredConfigRevision)

	path := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments"
	body := `{"service_scope":"forward","plugin_id":"` + fixture.pluginID + `","role":"telemetry","desired_version":"1.0.0","desired_config_revision":3,"enabled":false}`
	recorder = performKernelHandlerRequest(t, http.MethodPut, path, body, "/nodes/:id/assignments", (&KernelHandler{db: fixture.db}).UpsertAssignment)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
	require.False(t, assignment.Enabled)
	require.Equal(t, int64(3), assignment.LifecycleGeneration)
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).Error)
	require.False(t, lifecycle.DesiredEnabled)

	var operations int64
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&operations).Error)
	require.Zero(t, operations, "no Agent operation is queued")
}

// Deleting an assignment of a plugin that is not active on the node removes
// it at once.
func TestKernelAssignmentDeletePurgesInactivePlugin(t *testing.T) {
	fixture := newKernelAgentAssignmentFixture(t, "handler-agent-delete")
	path := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments/" + strconv.FormatUint(uint64(fixture.assignment.ID), 10)
	recorder := performKernelHandlerRequest(
		t, http.MethodDelete, path, "", "/nodes/:id/assignments/:assignment_id",
		(&KernelHandler{db: fixture.db}).DeleteAssignment,
	)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	err := fixture.db.First(&model.NodeServiceAssignment{}, fixture.assignment.ID).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func requireKernelErrorCode(t *testing.T, body []byte, code string) {
	t.Helper()
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &response), string(body))
	require.Equal(t, code, response.Error.Code, string(body))
	require.NotEmpty(t, response.Error.Message)
}
