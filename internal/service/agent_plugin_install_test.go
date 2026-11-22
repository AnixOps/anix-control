package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
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
	node := model.Node{Name: "agent-package-node", Host: "127.0.0.1", Status: model.NodeStatusOnline}
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

func TestLoadAuthorizedAgentPluginArtifactRejectsOtherNodeAndTampering(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	loadAuthorizedRelease := func(nodeID uint) (*AgentPluginReleaseDownload, error) {
		metadata, err := LoadAuthorizedAgentPluginMetadata(fixture.db, nodeID, fixture.release.PluginID, fixture.release.Version)
		if err != nil {
			return nil, err
		}
		return LoadAgentPluginArtifactBlob(fixture.db, metadata)
	}
	other := model.Node{Name: "other-node", Host: "127.0.0.2", Status: model.NodeStatusOnline}
	require.NoError(t, fixture.db.Create(&other).Error)
	_, err := loadAuthorizedRelease(other.ID)
	require.ErrorIs(t, err, ErrAgentPluginAssignmentDenied)

	tampered := append([]byte(nil), fixture.artifact...)
	tampered[0] ^= 0x01
	require.NoError(t, fixture.db.Model(&model.PluginArtifact{}).Where("release_id = ?", fixture.release.ID).Update("data", tampered).Error)
	_, err = LoadAuthorizedAgentPluginMetadata(fixture.db, fixture.node.ID, fixture.release.PluginID, fixture.release.Version)
	require.NoError(t, err, "bounded metadata path must not read or hash artifact bytes")
	_, err = loadAuthorizedRelease(fixture.node.ID)
	require.ErrorIs(t, err, ErrAgentPluginReleaseIntegrity)
}

func TestSyncNodePluginLifecycleFailsClosedOnRoleConflict(t *testing.T) {
	fixture := newAgentPluginInstallFixture(t)
	conflicting := fixture.assignment
	conflicting.ID = 0
	conflicting.Role = "conflicting-role"
	conflicting.DesiredConfigRevision = 3
	require.NoError(t, fixture.db.Create(&conflicting).Error)
	_, _, err := SyncNodePluginLifecycle(fixture.db, fixture.node.ID, fixture.assignment.PluginID, false, time.Now())
	require.ErrorIs(t, err, ErrAgentPluginAssignmentConflict)
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
