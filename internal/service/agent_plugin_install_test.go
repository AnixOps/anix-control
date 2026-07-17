package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type agentPluginInstallFixture struct {
	db           *gorm.DB
	publicKey    ed25519.PublicKey
	privateKey   ed25519.PrivateKey
	node         model.Node
	release      model.PluginRelease
	artifact     []byte
	manifestJSON []byte
	installation model.PluginInstallation
	assignment   model.NodeServiceAssignment
}

func newAgentPluginInstallFixture(t *testing.T) agentPluginInstallFixture {
	t.Helper()
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	artifact := []byte("anixops-signed-agent-package-v1")
	artifactDigest := sha256.Sum256(artifact)
	manifest := PluginManifest{
		ID: "agent-package-test", Name: "Agent Package Test", Version: "1.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, Architectures: []string{"linux-amd64"}, ArtifactSHA256: hex.EncodeToString(artifactDigest[:]),
		ConfigSchema: json.RawMessage(`{"type":"object","properties":{"interval":{"type":"integer"}}}`),
	}
	canonical, err := CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	node := model.Node{Name: "agent-package-node", Host: "127.0.0.1", APIKey: "agent-package-key", Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	installation := model.PluginInstallation{
		PluginID: manifest.ID, Target: "agent", DesiredVersion: manifest.Version, State: "pending", Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&installation).Error)
	configJSON := `{"interval":30}`
	configHash, err := HashKernelOperationConfig(configJSON)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginConfiguration{
		InstallationID: installation.ID, Revision: 2, ConfigJSON: configJSON, ConfigHash: configHash, UpdatedBy: 1,
	}).Error)
	assignment := model.NodeServiceAssignment{
		NodeID: node.ID, ServiceScope: "monitor", PluginID: manifest.ID, Role: "telemetry", DesiredVersion: manifest.Version,
		DesiredConfigRevision: 2, Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&assignment).Error)
	return agentPluginInstallFixture{
		db: db, publicKey: publicKey, privateKey: privateKey, node: node, release: *release, artifact: artifact,
		manifestJSON: canonical, installation: installation, assignment: assignment,
	}
}

func TestBuildAgentPluginInstallConfigUsesOnlySameOriginVerifiedMetadata(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	raw, err := BuildAgentPluginInstallConfig(fixture.db, fixture.node.ID, fixture.release.PluginID, fixture.release.Version)
	require.NoError(t, err)
	var payload AgentPluginInstallConfig
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	require.Equal(t, AgentPluginInstallAPIVersion, payload.APIVersion)
	require.Equal(t, fixture.release.PluginID, payload.PluginID)
	require.Equal(t, fixture.release.Version, payload.Version)
	require.Equal(t, int64(len(fixture.artifact)), payload.Artifact.Size)
	require.Equal(t, fixture.release.ArtifactSHA256, payload.Artifact.SHA256)
	require.NotContains(t, raw, base64.StdEncoding.EncodeToString(fixture.artifact), "artifact bytes must never be embedded in ConfigJSON")

	for _, relative := range []string{payload.Artifact.URL, payload.Manifest.URL} {
		parsed, parseErr := url.Parse(relative)
		require.NoError(t, parseErr)
		require.False(t, parsed.IsAbs())
		require.Empty(t, parsed.Host)
		require.Empty(t, parsed.Scheme)
		require.True(t, strings.HasPrefix(parsed.Path, "/api/v3/agent/plugin-releases/"))
		require.NotEmpty(t, parsed.Query().Get("sha256"))
		require.NotEmpty(t, parsed.Query().Get("size"))
	}
	require.Equal(t, fixture.release.Signature, payload.Manifest.Signature)
	require.Equal(t, fixture.release.TrustRootKeyID, payload.Manifest.KeyID)
	require.NotContains(t, raw, "signature_algorithm")
	var contract map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &contract))
	require.ElementsMatch(t, []string{"api_version", "plugin_id", "version", "artifact", "manifest"}, mapKeys(contract))
	var artifactContract map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(contract["artifact"], &artifactContract))
	require.ElementsMatch(t, []string{"url", "sha256", "size"}, mapKeys(artifactContract))
	var manifestContract map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(contract["manifest"], &manifestContract))
	require.ElementsMatch(t, []string{"url", "sha256", "size", "signature", "publisher", "key_id", "api_version"}, mapKeys(manifestContract))
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func TestLoadAuthorizedAgentPluginReleaseRejectsOtherNodeAndTampering(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	other := model.Node{Name: "other-node", Host: "127.0.0.2", Status: model.NodeStatusOnline}
	require.NoError(t, fixture.db.Create(&other).Error)
	_, err := LoadAuthorizedAgentPluginRelease(fixture.db, other.ID, fixture.release.PluginID, fixture.release.Version)
	require.ErrorIs(t, err, ErrAgentPluginAssignmentDenied)

	tampered := append([]byte(nil), fixture.artifact...)
	tampered[0] ^= 0x01
	require.NoError(t, fixture.db.Model(&model.PluginArtifact{}).Where("release_id = ?", fixture.release.ID).Update("data", tampered).Error)
	_, err = LoadAuthorizedAgentPluginMetadata(fixture.db, fixture.node.ID, fixture.release.PluginID, fixture.release.Version)
	require.NoError(t, err, "bounded metadata path must not read or hash artifact bytes")
	_, err = LoadAuthorizedAgentPluginRelease(fixture.db, fixture.node.ID, fixture.release.PluginID, fixture.release.Version)
	require.ErrorIs(t, err, ErrAgentPluginReleaseIntegrity)
}

func TestQueueAgentAssignmentLifecycleIsTransactionalOrderedAndIdempotent(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	now := time.Now().UTC()
	chain, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, now)
	require.NoError(t, err)
	require.NotNil(t, chain.Install)
	require.NotNil(t, chain.Update)
	require.NotNil(t, chain.Enable)
	require.Equal(t, int64(1), chain.Install.Revision)
	require.Equal(t, int64(2), chain.Update.Revision)
	require.Equal(t, int64(3), chain.Enable.Revision)
	require.Empty(t, chain.Install.DependsOnOperationID)
	require.Equal(t, chain.Install.ID, chain.Update.DependsOnOperationID)
	require.Equal(t, chain.Update.ID, chain.Enable.DependsOnOperationID)

	again, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, chain.Install.ID, again.Install.ID)
	require.Equal(t, chain.Update.ID, again.Update.ID)
	require.Equal(t, chain.Enable.ID, again.Enable.ID)
	var operationCount int64
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&operationCount).Error)
	require.Equal(t, int64(3), operationCount)
	for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("state", "succeeded").Error)
	}
	succeededReplay, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, chain.Install.ID, succeededReplay.Install.ID)
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&operationCount).Error)
	require.Equal(t, int64(3), operationCount, "successful chains must never repeat on reconcile")

	secondRole := fixture.assignment
	secondRole.ID = 0
	secondRole.Role = "telemetry-egress"
	require.NoError(t, fixture.db.Create(&secondRole).Error)
	deduplicated, err := QueueAgentAssignmentLifecycle(fixture.db, secondRole, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, chain.Install.ID, deduplicated.Install.ID)
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&operationCount).Error)
	require.Equal(t, int64(3), operationCount, "multiple roles for one node/plugin must share one lifecycle chain")
}

func TestQueueAgentAssignmentLifecycleFailsClosedOnRoleConflict(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	conflicting := fixture.assignment
	conflicting.ID = 0
	conflicting.Role = "conflicting-role"
	conflicting.DesiredConfigRevision = 3
	require.NoError(t, fixture.db.Create(&conflicting).Error)
	_, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now())
	require.ErrorIs(t, err, ErrAgentPluginAssignmentConflict)
	var count int64
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestNodePluginDisableUsesObservedActiveVersionAcrossRoles(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	chain, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now())
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("state", "succeeded").Error)
	}
	require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
		return RecordNodePluginOperationSuccessTx(tx, *chain.Enable)
	}))
	staleDisabledRole := fixture.assignment
	staleDisabledRole.ID = 0
	staleDisabledRole.Role = "stale-disabled"
	staleDisabledRole.DesiredVersion = "9.9.9"
	staleDisabledRole.Enabled = false
	require.NoError(t, fixture.db.Create(&staleDisabledRole).Error)

	require.NoError(t, fixture.db.Model(&model.NodeServiceAssignment{}).
		Where("id = ?", fixture.assignment.ID).Update("enabled", false).Error)
	require.NoError(t, fixture.db.First(&fixture.assignment, fixture.assignment.ID).Error)
	disableChain, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.NotNil(t, disableChain.Disable)
	require.Equal(t, fixture.release.Version, disableChain.Disable.TargetVersion, "disable must use observed active version, never a stale disabled role")

	neverInstalled := newAgentPluginInstallFixture(t)
	require.NoError(t, neverInstalled.db.Model(&model.NodeServiceAssignment{}).Where("id = ?", neverInstalled.assignment.ID).Update("enabled", false).Error)
	require.NoError(t, neverInstalled.db.First(&neverInstalled.assignment, neverInstalled.assignment.ID).Error)
	noOp, err := QueueAgentAssignmentLifecycle(neverInstalled.db, neverInstalled.assignment, time.Now())
	require.NoError(t, err)
	require.Nil(t, noOp.Disable, "a known never-enabled plugin must converge without a failing disable")
}

func TestNodePluginObservedStateRejectsStaleTerminalRevision(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	initial, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now())
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{initial.Install, initial.Update, initial.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("state", "succeeded").Error)
		require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
			return RecordNodePluginOperationSuccessTx(tx, *operation)
		}))
	}

	require.NoError(t, fixture.db.Model(&model.NodeServiceAssignment{}).Where("id = ?", fixture.assignment.ID).Update("enabled", false).Error)
	require.NoError(t, fixture.db.First(&fixture.assignment, fixture.assignment.ID).Error)
	disableChain, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.NotNil(t, disableChain.Disable)

	require.NoError(t, fixture.db.Model(&model.NodeServiceAssignment{}).Where("id = ?", fixture.assignment.ID).Update("enabled", true).Error)
	require.NoError(t, fixture.db.First(&fixture.assignment, fixture.assignment.ID).Error)
	reenable, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now().Add(2*time.Minute))
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{reenable.Install, reenable.Update, reenable.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("state", "succeeded").Error)
		require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
			return RecordNodePluginOperationSuccessTx(tx, *operation)
		}))
	}
	require.Greater(t, reenable.Enable.Revision, disableChain.Disable.Revision)

	// A late success from the superseded disable is still useful evidence, but
	// its older revision must not roll active state back after the new enable.
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", disableChain.Disable.ID).Update("state", "succeeded").Error)
	require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
		return RecordNodePluginOperationSuccessTx(tx, *disableChain.Disable)
	}))
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	require.True(t, lifecycle.ActiveEnabled)
	require.Equal(t, reenable.Enable.Revision, lifecycle.ActiveRevision)
	require.Equal(t, fixture.release.Version, lifecycle.ActiveVersion)

	refreshed, _, err := SyncNodePluginLifecycle(fixture.db, fixture.node.ID, fixture.release.PluginID, false, time.Now().Add(3*time.Minute))
	require.NoError(t, err)
	require.True(t, refreshed.ActiveEnabled)
	require.Equal(t, reenable.Enable.Revision, refreshed.ActiveRevision)
}

func TestReconcileAgentAssignmentsEnrollsOnlyVersionedAlphaGenerations(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	require.NoError(t, fixture.db.Model(&model.NodeServiceAssignment{}).Where("id = ?", fixture.assignment.ID).Update("lifecycle_generation", 0).Error)
	reconciled, err := ReconcileAgentAssignments(fixture.db, time.Now())
	require.NoError(t, err)
	require.Zero(t, reconciled)
	var count int64
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.Zero(t, count, "legacy generation-zero assignments must remain observational")

	_, _, err = SyncNodePluginLifecycle(fixture.db, fixture.node.ID, fixture.release.PluginID, false, time.Now())
	require.NoError(t, err)
	reconciled, err = ReconcileAgentAssignments(fixture.db, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, reconciled)
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.Equal(t, int64(3), count)

	reconciled, err = ReconcileAgentAssignments(fixture.db, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, 1, reconciled)
	require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.Equal(t, int64(3), count, "restart reconciliation must reuse the original operation chain")
}

func TestQueueAgentAssignmentLifecycleBuildsV1ToV2InstallUpdateEnableChain(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	v1, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now())
	require.NoError(t, err)
	require.Equal(t, "1.0.0", v1.Install.TargetVersion)

	v2Artifact := []byte("anixops-signed-agent-package-v2")
	v2Digest := sha256.Sum256(v2Artifact)
	v2Manifest := PluginManifest{
		ID: fixture.release.PluginID, Name: "Agent Package Test", Version: "2.0.0", APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"agent"}, Architectures: []string{"linux-amd64"}, ArtifactSHA256: hex.EncodeToString(v2Digest[:]),
		ConfigSchema: json.RawMessage(`{"type":"object","properties":{"interval":{"type":"integer"}}}`),
	}
	v2Canonical, err := CanonicalPluginManifest(v2Manifest)
	require.NoError(t, err)
	v2Release, err := RegisterPluginRelease(fixture.db, string(v2Canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(fixture.privateKey, v2Canonical)), fixture.publicKey)
	require.NoError(t, err)
	_, err = StorePluginArtifact(fixture.db, v2Release.ID, v2Artifact)
	require.NoError(t, err)
	v2Config := `{"interval":45}`
	v2ConfigHash, err := HashKernelOperationConfig(v2Config)
	require.NoError(t, err)
	require.NoError(t, fixture.db.Model(&model.PluginInstallation{}).Where("id = ?", fixture.installation.ID).
		Updates(map[string]any{"desired_version": "2.0.0", "config_revision": 3}).Error)
	require.NoError(t, fixture.db.Model(&model.PluginConfiguration{}).Where("installation_id = ?", fixture.installation.ID).
		Updates(map[string]any{"revision": 3, "config_json": v2Config, "config_hash": v2ConfigHash}).Error)
	require.NoError(t, fixture.db.Model(&model.NodeServiceAssignment{}).Where("id = ?", fixture.assignment.ID).
		Updates(map[string]any{"desired_version": "2.0.0", "desired_config_revision": 3, "lifecycle_generation": 2}).Error)
	require.NoError(t, fixture.db.First(&fixture.assignment, fixture.assignment.ID).Error)

	v2, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "plugin.install", v2.Install.Kind)
	require.Equal(t, "plugin.update", v2.Update.Kind)
	require.Equal(t, "plugin.enable", v2.Enable.Kind)
	require.Equal(t, "2.0.0", v2.Install.TargetVersion)
	require.Equal(t, "2.0.0", v2.Update.TargetVersion)
	require.JSONEq(t, v2Config, v2.Update.ConfigJSON)
	require.Equal(t, v2.Install.ID, v2.Update.DependsOnOperationID)
	require.Equal(t, v2.Update.ID, v2.Enable.DependsOnOperationID)

	var operations []model.KernelOperation
	require.NoError(t, fixture.db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 6)
	require.Equal(t, []string{"superseded", "superseded", "superseded"}, []string{operations[0].State, operations[1].State, operations[2].State})
	require.Equal(t, []string{"plugin.install", "plugin.update", "plugin.enable"}, []string{operations[3].Kind, operations[4].Kind, operations[5].Kind})
}

func TestNodePluginLifecycleUsesBoundedRetryEpochAndExplicitRearm(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	chain, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now())
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{
			"state": "failed", "last_error": "synthetic failure",
		}).Error)
	}
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	_, err = QueueNodePluginLifecycle(fixture.db, &lifecycle, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, lifecycle.RetryEpoch, "first failure schedules the next epoch")
	require.NotNil(t, lifecycle.RetryAfter)
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	blocked, err := QueueNodePluginLifecycle(fixture.db, &lifecycle, lifecycle.RetryAfter.Add(-time.Millisecond))
	require.NoError(t, err)
	require.Nil(t, blocked.Install)

	for attempt := 0; attempt < 2; attempt++ {
		require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
		require.NotNil(t, lifecycle.RetryAfter)
		_, err = QueueNodePluginLifecycle(fixture.db, &lifecycle, lifecycle.RetryAfter.Add(time.Millisecond))
		require.NoError(t, err)
		var operations []model.KernelOperation
		require.NoError(t, fixture.db.Order("revision DESC").Limit(3).Find(&operations).Error)
		require.Len(t, operations, 3)
		for index := range operations {
			require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operations[index].ID).Updates(map[string]any{
				"state": "failed", "last_error": "synthetic retry failure",
			}).Error)
		}
		require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
		_, err = QueueNodePluginLifecycle(fixture.db, &lifecycle, time.Now())
		require.NoError(t, err)
	}
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	require.True(t, lifecycle.RetryExhausted, "automatic retries must stop at the bounded attempt limit")

	// Repeating the same desired-state PUT is an explicit retry epoch and is
	// allowed to rearm an exhausted lifecycle without changing the package.
	rearmed, changed, err := SyncNodePluginLifecycle(fixture.db, fixture.node.ID, fixture.release.PluginID, true, time.Now())
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, rearmed.RetryExhausted)
	newChain, err := QueueNodePluginLifecycle(fixture.db, rearmed, time.Now())
	require.NoError(t, err)
	require.NotNil(t, newChain.Install)
}

func TestNodePluginRetrySuccessRestoresHealthyInstallationState(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	first, err := QueueAgentAssignmentLifecycle(fixture.db, fixture.assignment, time.Now())
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{first.Install, first.Update, first.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{
			"state": "failed", "last_error": "synthetic failure",
		}).Error)
	}
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	_, err = QueueNodePluginLifecycle(fixture.db, &lifecycle, time.Now())
	require.NoError(t, err)
	require.NoError(t, RefreshAgentPluginInstallationObservedState(fixture.db, fixture.release.PluginID))
	require.NotEmpty(t, lifecycle.LastError)
	var installation model.PluginInstallation
	require.NoError(t, fixture.db.First(&installation, fixture.installation.ID).Error)
	require.Equal(t, "degraded", installation.State)

	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	require.NotNil(t, lifecycle.RetryAfter)
	retry, err := QueueNodePluginLifecycle(fixture.db, &lifecycle, lifecycle.RetryAfter.Add(time.Millisecond))
	require.NoError(t, err)
	for _, operation := range []*model.KernelOperation{retry.Install, retry.Update, retry.Enable} {
		require.NoError(t, fixture.db.Model(&model.KernelOperation{}).Where("id = ?", operation.ID).Update("state", "succeeded").Error)
		require.NoError(t, fixture.db.Transaction(func(tx *gorm.DB) error {
			return RecordNodePluginOperationSuccessTx(tx, *operation)
		}))
	}
	require.NoError(t, RefreshAgentPluginInstallationObservedState(fixture.db, fixture.release.PluginID))
	require.NoError(t, fixture.db.First(&lifecycle, "node_id = ? AND plugin_id = ?", fixture.node.ID, fixture.release.PluginID).Error)
	require.Empty(t, lifecycle.LastError)
	require.Nil(t, lifecycle.RetryAfter)
	require.False(t, lifecycle.RetryExhausted)
	require.NoError(t, fixture.db.First(&installation, fixture.installation.ID).Error)
	require.Equal(t, "healthy", installation.State)
	require.Equal(t, fixture.release.Version, installation.ObservedVersion)
}

func TestAgentLifecycleTransactionRetriesSQLiteWriterContention(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lifecycle-lock.db")+"?_pragma=busy_timeout(1)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PluginTargetLock{}, &model.NodePluginLifecycle{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	holderReady := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- db.Transaction(func(tx *gorm.DB) error {
			if err := LockPluginInstallationTarget(tx, "agent"); err != nil {
				return err
			}
			if err := tx.Model(&model.PluginTargetLock{}).Where("target = ?", "agent").Update("updated_at", time.Now()).Error; err != nil {
				return err
			}
			close(holderReady)
			<-releaseHolder
			return nil
		})
	}()
	<-holderReady

	var attempts atomic.Int32
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- WithAgentLifecycleTransaction(db, func(tx *gorm.DB) error {
			attempts.Add(1)
			if err := LockPluginInstallationTarget(tx, "agent"); err != nil {
				return err
			}
			_, err := LockNodePluginLifecycleTx(tx, 7, "contention-plugin")
			return err
		})
	}()

	time.Sleep(40 * time.Millisecond)
	close(releaseHolder)
	select {
	case err := <-holderDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("SQLite writer holder did not release")
	}
	select {
	case err := <-workerDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("SQLite lifecycle mutation did not converge after retry")
	}
	require.Greater(t, attempts.Load(), int32(1), "writer contention should exercise bounded retry")
	var lifecycle model.NodePluginLifecycle
	require.NoError(t, db.First(&lifecycle, "node_id = ? AND plugin_id = ?", 7, "contention-plugin").Error)
}
