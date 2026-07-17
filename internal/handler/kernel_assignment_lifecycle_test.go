package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
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
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), DispatchEnabled: true,
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	registerKernelAgentRelease(t, db, privateKey, publicKey, pluginID, "1.0.0")
	node := model.Node{Name: pluginID + "-node", Host: "127.0.0.1", APIKey: pluginID + "-key", Status: model.NodeStatusOnline}
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
	chain, err := service.QueueAgentAssignmentLifecycle(db, assignment, time.Now())
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable} {
		require.NotNil(t, operation)
		require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("state", "succeeded").Error)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return service.RecordNodePluginOperationSuccessTx(tx, *operation)
		}))
	}
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

func TestKernelAssignmentQueuesAgentInstallUpdateEnableAndDisable(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{
		OfficialPublicKey: base64.StdEncoding.EncodeToString(publicKey), DispatchEnabled: true,
	}})
	t.Cleanup(func() { config.Set(previousConfig) })

	artifact := []byte("handler-agent-lifecycle-package")
	digest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: "handler-agent-lifecycle", Name: "Handler Agent Lifecycle", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: hex.EncodeToString(digest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	node := model.Node{Name: "handler-agent-node", Host: "127.0.0.1", Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	installation := model.PluginInstallation{
		PluginID: manifest.ID, Target: "agent", DesiredVersion: manifest.Version, State: "pending", Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&installation).Error)
	configHash, err := service.HashKernelOperationConfig(`{"interval":15}`)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginConfiguration{
		InstallationID: installation.ID, Revision: 4, ConfigJSON: `{"interval":15}`, ConfigHash: configHash, UpdatedBy: 1,
	}).Error)

	handler := (&KernelHandler{db: db}).UpsertAssignment
	path := "/nodes/" + strconv.FormatUint(uint64(node.ID), 10) + "/assignments"
	route := "/nodes/:id/assignments"
	body := `{"service_scope":"forward","plugin_id":"` + manifest.ID + `","role":"telemetry","desired_version":"1.0.0","desired_config_revision":4,"enabled":true}`
	first := performKernelHandlerRequest(t, http.MethodPut, path, body, route, handler)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	firstChain := first.Header().Get("X-AnixOps-Operation-Chain")
	require.Len(t, strings.Split(firstChain, ","), 3)
	require.Equal(t, strings.Split(firstChain, ",")[0], first.Header().Get("X-AnixOps-Operation-ID"))

	var operations []model.KernelOperation
	require.NoError(t, db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 3)
	require.Equal(t, []string{"plugin.install", "plugin.update", "plugin.enable"}, []string{operations[0].Kind, operations[1].Kind, operations[2].Kind})
	require.Empty(t, operations[0].DependsOnOperationID)
	require.Equal(t, operations[0].ID, operations[1].DependsOnOperationID)
	require.Equal(t, operations[1].ID, operations[2].DependsOnOperationID)
	require.Contains(t, operations[0].ConfigJSON, service.AgentPluginInstallAPIVersion)

	replayed := performKernelHandlerRequest(t, http.MethodPut, path, body, route, handler)
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.Equal(t, firstChain, replayed.Header().Get("X-AnixOps-Operation-Chain"))
	require.NoError(t, db.Find(&operations).Error)
	require.Len(t, operations, 3)
	for index := range operations {
		require.NoError(t, db.Model(&model.KernelOperation{}).Where("id = ?", operations[index].ID).Update("state", "succeeded").Error)
	}
	var observedEnable model.KernelOperation
	require.NoError(t, db.First(&observedEnable, "id = ?", operations[2].ID).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return service.RecordNodePluginOperationSuccessTx(tx, observedEnable)
	}))

	disableBody := `{"service_scope":"forward","plugin_id":"` + manifest.ID + `","role":"telemetry","desired_version":"1.0.0","desired_config_revision":4,"enabled":false}`
	disabled := performKernelHandlerRequest(t, http.MethodPut, path, disableBody, route, handler)
	require.Equal(t, http.StatusOK, disabled.Code, disabled.Body.String())
	require.Len(t, strings.Split(disabled.Header().Get("X-AnixOps-Operation-Chain"), ","), 1)
	require.NoError(t, db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 4)
	require.Equal(t, "plugin.disable", operations[3].Kind)
	require.Equal(t, []string{"succeeded", "succeeded", "succeeded", "pending"}, []string{operations[0].State, operations[1].State, operations[2].State, operations[3].State})

	var assignment model.NodeServiceAssignment
	require.NoError(t, db.First(&assignment, "node_id = ? AND plugin_id = ?", node.ID, manifest.ID).Error)
	require.False(t, assignment.Enabled)
	require.Equal(t, int64(2), assignment.LifecycleGeneration)
}

func TestKernelAgentInstallationUpgradeSynchronizesAssignmentVersion(t *testing.T) {
	fixture := newKernelAgentAssignmentFixture(t, "handler-agent-upgrade")
	registerKernelAgentRelease(t, fixture.db, fixture.privateKey, fixture.publicKey, fixture.pluginID, "2.0.0")

	body := `{"plugin_id":"` + fixture.pluginID + `","target":"agent","desired_version":"2.0.0","enabled":true}`
	recorder := performKernelHandlerRequest(
		t, http.MethodPut, "/plugin-installations", body, "/plugin-installations",
		(&KernelHandler{db: fixture.db}).UpsertPluginInstallation,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Len(t, strings.Split(recorder.Header().Get("X-AnixOps-Operation-Chain"), ","), 3)

	var assignment model.NodeServiceAssignment
	require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
	require.Equal(t, "2.0.0", assignment.DesiredVersion)
	require.Equal(t, int64(2), assignment.DesiredConfigRevision)

	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).Error)
	require.Equal(t, "2.0.0", lifecycle.DesiredVersion)
	require.Equal(t, "1.0.0", lifecycle.ActiveVersion)
	require.True(t, lifecycle.ActiveEnabled)

	var operations []model.KernelOperation
	require.NoError(t, fixture.db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 6)
	require.Equal(t, []string{"succeeded", "succeeded", "succeeded"}, []string{operations[0].State, operations[1].State, operations[2].State})
	for _, operation := range operations[3:] {
		require.Equal(t, "2.0.0", operation.TargetVersion)
		require.Equal(t, "pending", operation.State)
	}
}

func TestKernelAgentConfigurationUpdateAdvancesAssignmentRevision(t *testing.T) {
	fixture := newKernelAgentAssignmentFixture(t, "handler-agent-config-sync")
	path := "/plugin-installations/" + strconv.FormatUint(uint64(fixture.installation.ID), 10) + "/config"
	recorder := performKernelHandlerRequest(
		t, http.MethodPut, path, `{"config":{"interval":45},"expected_revision":2}`,
		"/plugin-installations/:id/config", (&KernelHandler{db: fixture.db}).UpdatePluginInstallationConfiguration,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Len(t, strings.Split(recorder.Header().Get("X-AnixOps-Operation-Chain"), ","), 3)

	var assignment model.NodeServiceAssignment
	require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
	require.Equal(t, int64(3), assignment.DesiredConfigRevision)
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).Error)
	require.Equal(t, int64(3), lifecycle.DesiredConfigRevision)
	require.Equal(t, int64(2), lifecycle.DesiredGeneration)

	var operations []model.KernelOperation
	require.NoError(t, fixture.db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 6)
	require.Equal(t, "plugin.update", operations[4].Kind)
	require.JSONEq(t, `{"interval":45}`, operations[4].ConfigJSON)
}

func TestKernelGlobalAgentDisableQueuesObservedVersionAndDeniesPackage(t *testing.T) {
	fixture := newKernelAgentAssignmentFixture(t, "handler-agent-global-disable")
	body := `{"plugin_id":"` + fixture.pluginID + `","target":"agent","desired_version":"1.0.0","enabled":false}`
	recorder := performKernelHandlerRequest(
		t, http.MethodPut, "/plugin-installations", body, "/plugin-installations",
		(&KernelHandler{db: fixture.db}).UpsertPluginInstallation,
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Len(t, strings.Split(recorder.Header().Get("X-AnixOps-Operation-Chain"), ","), 1)

	var operations []model.KernelOperation
	require.NoError(t, fixture.db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 4)
	require.Equal(t, "plugin.disable", operations[3].Kind)
	require.Equal(t, "1.0.0", operations[3].TargetVersion)
	_, err := service.LoadAuthorizedAgentPluginMetadata(fixture.db, fixture.node.ID, fixture.pluginID, "1.0.0")
	require.ErrorIs(t, err, service.ErrAgentPluginAssignmentDenied)

	var assignment model.NodeServiceAssignment
	require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
	require.True(t, assignment.Enabled, "global disable must preserve the node role intent")
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).Error)
	require.False(t, lifecycle.DesiredEnabled)
	require.True(t, lifecycle.ActiveEnabled)
}

func TestKernelAssignmentClearAndDeleteUseObservedVersion(t *testing.T) {
	t.Run("dispatch disabled purges already inactive assignment", func(t *testing.T) {
		fixture := newKernelAgentAssignmentFixture(t, "handler-agent-delete-disabled")
		fixture.assignment.DesiredVersion = ""
		fixture.assignment.Enabled = false
		require.NoError(t, fixture.db.Save(&fixture.assignment).Error)
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).
			Where("node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).
			Update("state", "superseded").Error)
		var lifecycle model.NodePluginLifecycle
		require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.pluginID).Error)
		lifecycle.DesiredVersion = ""
		lifecycle.DesiredEnabled = false
		lifecycle.ActiveVersion = ""
		lifecycle.ActiveEnabled = false
		lifecycle.ActiveRevision = 0
		require.NoError(t, fixture.db.Save(&lifecycle).Error)
		previousConfig := config.Get()
		config.Set(&config.Config{Plugins: config.PluginConfig{OfficialPublicKey: base64.StdEncoding.EncodeToString(fixture.publicKey)}})
		t.Cleanup(func() { config.Set(previousConfig) })

		path := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments/" + strconv.FormatUint(uint64(fixture.assignment.ID), 10)
		recorder := performKernelHandlerRequest(
			t, http.MethodDelete, path, "", "/nodes/:id/assignments/:assignment_id",
			(&KernelHandler{db: fixture.db}).DeleteAssignment,
		)
		require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
		var assignment model.NodeServiceAssignment
		err := fixture.db.First(&assignment, fixture.assignment.ID).Error
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("clear desired version", func(t *testing.T) {
		fixture := newKernelAgentAssignmentFixture(t, "handler-agent-clear")
		path := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments"
		body := `{"service_scope":"forward","plugin_id":"` + fixture.pluginID + `","role":"telemetry","desired_version":"","enabled":false}`
		recorder := performKernelHandlerRequest(t, http.MethodPut, path, body, "/nodes/:id/assignments", (&KernelHandler{db: fixture.db}).UpsertAssignment)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Len(t, strings.Split(recorder.Header().Get("X-AnixOps-Operation-Chain"), ","), 1)

		var assignment model.NodeServiceAssignment
		require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
		require.Empty(t, assignment.DesiredVersion)
		require.False(t, assignment.Enabled)
		var disable model.KernelOperation
		require.NoError(t, fixture.db.Order("revision DESC").First(&disable).Error)
		require.Equal(t, "plugin.disable", disable.Kind)
		require.Equal(t, "1.0.0", disable.TargetVersion)
	})

	t.Run("recreate clears delete tombstone", func(t *testing.T) {
		fixture := newKernelAgentAssignmentFixture(t, "handler-agent-recreate")
		deletePath := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments/" + strconv.FormatUint(uint64(fixture.assignment.ID), 10)
		recorder := performKernelHandlerRequest(
			t, http.MethodDelete, deletePath, "", "/nodes/:id/assignments/:assignment_id",
			(&KernelHandler{db: fixture.db}).DeleteAssignment,
		)
		require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
		recreatePath := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments"
		recreateBody := `{"service_scope":"forward","plugin_id":"` + fixture.pluginID + `","role":"telemetry","desired_version":"1.0.0","desired_config_revision":2,"enabled":true}`
		recorder = performKernelHandlerRequest(t, http.MethodPut, recreatePath, recreateBody, "/nodes/:id/assignments", (&KernelHandler{db: fixture.db}).UpsertAssignment)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Len(t, strings.Split(recorder.Header().Get("X-AnixOps-Operation-Chain"), ","), 3)
		var assignment model.NodeServiceAssignment
		require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
		require.False(t, assignment.DeletePending)
		require.True(t, assignment.Enabled)
		var operations []model.KernelOperation
		require.NoError(t, fixture.db.Order("revision").Find(&operations).Error)
		require.Len(t, operations, 7)
		require.Equal(t, "superseded", operations[3].State)
		require.Equal(t, []string{"plugin.install", "plugin.update", "plugin.enable"}, []string{operations[4].Kind, operations[5].Kind, operations[6].Kind})
	})

	t.Run("soft delete then purge", func(t *testing.T) {
		fixture := newKernelAgentAssignmentFixture(t, "handler-agent-delete")
		path := "/nodes/" + strconv.FormatUint(uint64(fixture.node.ID), 10) + "/assignments/" + strconv.FormatUint(uint64(fixture.assignment.ID), 10)
		recorder := performKernelHandlerRequest(
			t, http.MethodDelete, path, "", "/nodes/:id/assignments/:assignment_id",
			(&KernelHandler{db: fixture.db}).DeleteAssignment,
		)
		require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
		var assignment model.NodeServiceAssignment
		require.NoError(t, fixture.db.First(&assignment, fixture.assignment.ID).Error)
		require.True(t, assignment.DeletePending)
		require.False(t, assignment.Enabled)

		var disable model.KernelOperation
		require.NoError(t, fixture.db.Order("revision DESC").First(&disable).Error)
		require.Equal(t, "plugin.disable", disable.Kind)
		require.Equal(t, "1.0.0", disable.TargetVersion)
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", disable.ID).Update("state", "succeeded").Error)
		require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
			return service.RecordNodePluginOperationSuccessTx(tx, disable)
		}))
		_, err := service.ReconcileAgentAssignments(fixture.db, time.Now().Add(time.Minute))
		require.NoError(t, err)
		err = fixture.db.First(&assignment, fixture.assignment.ID).Error
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})
}

func TestKernelAssignmentRollsBackDesiredStateWhenAgentChainCannotBeBuilt(t *testing.T) {
	db := newKernelHandlerTestDB(t, &model.Node{})
	previousConfig := config.Get()
	config.Set(&config.Config{Plugins: config.PluginConfig{DispatchEnabled: true}})
	t.Cleanup(func() { config.Set(previousConfig) })
	node := model.Node{Name: "rollback-node", Host: "127.0.0.1"}
	require.NoError(t, db.Create(&node).Error)
	pluginID := "rollback-agent-assignment"
	require.NoError(t, db.Create(&model.Plugin{ID: pluginID, Name: pluginID, Publisher: "AnixOps", Official: true}).Error)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, ArtifactSHA256: strings.Repeat("a", 64),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginRelease{
		PluginID: pluginID, Version: "1.0.0", APIVersion: "v1", ManifestJSON: string(canonical), ArtifactSHA256: manifest.ArtifactSHA256, Signature: "unverified",
	}).Error)

	path := "/nodes/" + strconv.FormatUint(uint64(node.ID), 10) + "/assignments"
	body := `{"service_scope":"forward","plugin_id":"` + pluginID + `","role":"telemetry","desired_version":"1.0.0","enabled":true}`
	recorder := performKernelHandlerRequest(t, http.MethodPut, path, body, "/nodes/:id/assignments", (&KernelHandler{db: db}).UpsertAssignment)
	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	var assignmentCount, operationCount int64
	require.NoError(t, db.Model(&model.NodeServiceAssignment{}).Where("node_id = ? AND plugin_id = ?", node.ID, pluginID).Count(&assignmentCount).Error)
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&operationCount).Error)
	require.Zero(t, assignmentCount)
	require.Zero(t, operationCount)
}
