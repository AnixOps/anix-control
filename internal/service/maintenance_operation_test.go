package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func maintenanceOperationFixture(t *testing.T) (*gorm.DB, uint) {
	t.Helper()
	db := newKernelTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.MaintenanceSettings{}))
	require.NoError(t, db.AutoMigrate(model.MaintenanceOperationModels()...))
	require.NoError(t, db.Create(&model.MaintenanceSettings{ID: 1, OwnerID: 1, TechnicianIDs: []uint{2}}).Error)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	for _, version := range []string{"1.0.0", "2.0.0"} {
		artifact := []byte("signed telemetry " + version)
		digest := sha256.Sum256(artifact)
		manifest := PluginManifest{ID: "machine-telemetry", Name: "Machine Telemetry", Version: version, APIVersion: "v1", Publisher: "AnixOps", Targets: []string{"agent"}, Architectures: []string{"linux-amd64"}, ArtifactSHA256: hex.EncodeToString(digest[:]), ConfigSchema: json.RawMessage(`{"type":"object","properties":{"interval":{"type":"integer"}},"additionalProperties":false}`)}
		canonical, err := CanonicalPluginManifest(manifest)
		require.NoError(t, err)
		release, err := RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(private, canonical)), public)
		require.NoError(t, err)
		_, err = StorePluginArtifact(db, release.ID, artifact)
		require.NoError(t, err)
	}
	node := model.Node{Name: "maintenance node", Host: "127.0.0.1", APIKey: "test-only", Status: model.NodeStatusOnline}
	require.NoError(t, db.Create(&node).Error)
	require.NoError(t, db.Create(&model.PluginInstallation{PluginID: "machine-telemetry", Target: "agent", DesiredVersion: "1.0.0", State: "enabled", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.NodeServiceAssignment{NodeID: node.ID, PluginID: "machine-telemetry", ServiceScope: "monitoring", Role: "telemetry", DesiredVersion: "1.0.0", Enabled: true, LifecycleGeneration: 1}).Error)
	require.NoError(t, db.Create(&model.NodePluginLifecycle{NodeID: node.ID, PluginID: "machine-telemetry", DesiredVersion: "1.0.0", ActiveVersion: "1.0.0", ActiveEnabled: true, DesiredEnabled: true, DesiredGeneration: 1}).Error)
	return db, node.ID
}

func maintenanceOperationInput(node uint, kind, version string) MaintenanceChangeInput {
	return MaintenanceChangeInput{RequestKey: uuid.NewString(), NodeIDs: []uint{node}, PluginID: "machine-telemetry", TargetVersion: version, Kind: kind, Config: json.RawMessage(`{}`)}
}

func TestMaintenanceChangeApprovalIsExactAndExecutionIsIdempotent(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	input := maintenanceOperationInput(nodeID, "update", "2.0.0")
	input.Config = json.RawMessage(`{"interval":30}`)
	change, err := CreateMaintenanceChange(db, 2, input)
	require.NoError(t, err)
	require.Equal(t, "pending", change.Status)
	_, err = ExecuteMaintenanceChange(db, 2, change.ID)
	require.ErrorIs(t, err, ErrMaintenanceOperationDenied)
	_, err = ApproveMaintenanceChange(db, 2, change.ID, change.BindingHash)
	require.ErrorIs(t, err, ErrMaintenanceOperationDenied)
	_, err = ApproveMaintenanceChange(db, 1, change.ID, "wrong")
	require.Error(t, err)
	_, err = ApproveMaintenanceChange(db, 1, change.ID, change.BindingHash)
	require.NoError(t, err)
	executed, err := ExecuteMaintenanceChange(db, 2, change.ID)
	require.NoError(t, err)
	var operations []model.KernelOperation
	require.NoError(t, db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 3)
	require.Equal(t, "plugin.install", operations[0].Kind)
	require.Equal(t, "plugin.update", operations[1].Kind)
	require.Equal(t, "plugin.enable", operations[2].Kind)
	require.Equal(t, operations[0].ID, operations[1].DependsOnOperationID)
	require.Equal(t, operations[1].ID, operations[2].DependsOnOperationID)
	require.Equal(t, `{"interval":30}`, operations[1].ConfigJSON)
	replay, err := ExecuteMaintenanceChange(db, 2, change.ID)
	require.NoError(t, err)
	require.Equal(t, executed.OperationIDsJSON, replay.OperationIDsJSON)
	var count int64
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.EqualValues(t, 3, count)
	_, err = LoadAuthorizedAgentPluginRelease(db, nodeID, "machine-telemetry", "2.0.0")
	require.NoError(t, err, "approved node-specific version may download while fleet default stays at 1.0.0")
	_, err = ReconcileAgentAssignments(db, time.Now())
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.EqualValues(t, 3, count, "reconciliation cannot enqueue an unapproved replacement")
}

func TestMaintenanceSingleNodeRepairsAndLegacyAdminBypass(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	denied, err := BlockLegacyMaintenanceMutation(db, 2)
	require.NoError(t, err)
	require.True(t, denied)
	denied, err = BlockLegacyMaintenanceMutation(db, 1)
	require.NoError(t, err)
	require.False(t, denied)
	_, err = CreateMaintenanceChange(db, 3, maintenanceOperationInput(nodeID, "restart", "1.0.0"))
	require.ErrorIs(t, err, ErrMaintenanceOperationDenied)
	_, err = CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "restart", "2.0.0"))
	require.Error(t, err)
	_, err = CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "rollback", "2.0.0"))
	require.ErrorContains(t, err, "successful execution evidence")
	change, err := CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "restart", "1.0.0"))
	require.NoError(t, err)
	require.Equal(t, "approved", change.Status)
	_, err = ExecuteMaintenanceChange(db, 2, change.ID)
	require.NoError(t, err)
	var operations []model.KernelOperation
	require.NoError(t, db.Order("revision").Find(&operations).Error)
	require.Len(t, operations, 2)
	require.Equal(t, "plugin.disable", operations[0].Kind)
	require.Equal(t, "plugin.enable", operations[1].Kind)
	require.Equal(t, operations[0].ID, operations[1].DependsOnOperationID)
}

func TestMaintenanceManualRollbackPinsVerifiedVersion(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	require.NoError(t, db.Model(&model.NodeServiceAssignment{}).Where("node_id = ?", nodeID).Update("desired_version", "2.0.0").Error)
	require.NoError(t, db.Model(&model.NodePluginLifecycle{}).Where("node_id = ?", nodeID).Updates(map[string]any{"desired_version": "2.0.0", "active_version": "2.0.0"}).Error)
	require.NoError(t, db.Create(&model.KernelOperation{ID: uuid.NewString(), IdempotencyKey: "verified-v1-config", NodeID: &nodeID, PluginID: "machine-telemetry", TargetVersion: "1.0.0", Kind: "plugin.update", State: "succeeded", ConfigJSON: `{"interval":60}`, Revision: 1}).Error)
	require.NoError(t, db.Create(&model.KernelOperation{ID: uuid.NewString(), IdempotencyKey: "verified-v1", NodeID: &nodeID, PluginID: "machine-telemetry", TargetVersion: "1.0.0", Kind: "plugin.enable", State: "succeeded", ConfigJSON: "{}", Revision: 2, ResultJSON: maintenanceHealthyProof(t, "1.0.0", `{"interval":60}`, 2)}).Error)
	change, err := CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "rollback", "1.0.0"))
	require.NoError(t, err)
	_, err = ExecuteMaintenanceChange(db, 2, change.ID)
	require.NoError(t, err)
	_, err = ReconcileAgentAssignments(db, time.Now())
	require.NoError(t, err)
	var assignment model.NodeServiceAssignment
	require.NoError(t, db.First(&assignment, "node_id = ?", nodeID).Error)
	require.Equal(t, "1.0.0", assignment.DesiredVersion)
	var count int64
	require.NoError(t, db.Model(&model.KernelOperation{}).Where("kind = ? AND state = ? AND target_version = ?", "plugin.update", "pending", "1.0.0").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMaintenanceChangeRejectsSignatureConfigTamperingAndDuplicateKeyMutation(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	input := maintenanceOperationInput(nodeID, "update", "2.0.0")
	change, err := CreateMaintenanceChange(db, 2, input)
	require.NoError(t, err)
	input.TargetVersion = "1.0.0"
	_, err = CreateMaintenanceChange(db, 2, input)
	require.ErrorContains(t, err, "bound to another action")
	_, err = ApproveMaintenanceChange(db, 1, change.ID, change.BindingHash)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.MaintenanceChange{}).Where("id = ?", change.ID).Update("config_json", `{"interval":99}`).Error)
	_, err = ExecuteMaintenanceChange(db, 2, change.ID)
	require.ErrorContains(t, err, "configuration was modified")
	require.NoError(t, db.Model(&model.PluginRelease{}).Where("plugin_id = ? AND version = ?", "machine-telemetry", "2.0.0").Update("signature", "tampered").Error)
	_, err = CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "update", "2.0.0"))
	require.Error(t, err)
}

func TestMaintenanceChangeQueueFailureRollsBackApprovalClaimAndAudit(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	change, err := CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "restart", "1.0.0"))
	require.NoError(t, err)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("reject-maintenance-kernel", func(tx *gorm.DB) {
		if tx.Statement.Table == (model.KernelOperation{}).TableName() {
			_ = tx.AddError(gorm.ErrInvalidTransaction)
		}
	}))
	_, err = ExecuteMaintenanceChange(db, 2, change.ID)
	require.Error(t, err)
	require.NoError(t, db.First(change, change.ID).Error)
	require.Equal(t, "approved", change.Status)
	var count int64
	require.NoError(t, db.Model(&model.MaintenanceChangeAudit{}).Where("change_id = ? AND action = ?", change.ID, "queued").Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Callback().Create().Remove("reject-maintenance-kernel"))
	_, err = ExecuteMaintenanceChange(db, 2, change.ID)
	require.NoError(t, err)
}

func TestMaintenanceChangeConcurrentExecutionQueuesOnce(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	// One SQLite connection gives deterministic transaction ordering while callers
	// still race the same durable approved request from independent goroutines.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	change, err := CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "restart", "1.0.0"))
	require.NoError(t, err)
	var group sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		group.Add(1)
		go func() { defer group.Done(); _, err := ExecuteMaintenanceChange(db, 2, change.ID); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&model.KernelOperation{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	require.NoError(t, db.Model(&model.MaintenanceChangeAudit{}).Where("action = ?", "queued").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func maintenanceHealthyProof(t *testing.T, version, config string, revision int64) string {
	t.Helper()
	hash, err := HashKernelOperationConfig(config)
	require.NoError(t, err)
	data, err := json.Marshal(map[string]any{"id": "machine-telemetry", "desired_version": version, "observed_version": version, "enabled": true, "health": "healthy", "config_hash": hash, "observed_revision": revision})
	require.NoError(t, err)
	return string(data)
}

func TestMaintenanceRestoreRequiresHealthBoundToVersionAndConfiguration(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	config := model.KernelOperation{ID: uuid.NewString(), IdempotencyKey: "proof-config", NodeID: &nodeID, PluginID: "machine-telemetry", TargetVersion: "1.0.0", Kind: "plugin.configure", State: "succeeded", Revision: 1, ConfigJSON: `{"interval":42}`}
	enabled := model.KernelOperation{ID: uuid.NewString(), IdempotencyKey: "proof-enable", NodeID: &nodeID, PluginID: "machine-telemetry", TargetVersion: "1.0.0", Kind: "plugin.enable", State: "succeeded", Revision: 2, ConfigJSON: "{}"}
	require.NoError(t, db.Create(&config).Error)
	require.NoError(t, db.Create(&enabled).Error)
	_, err := CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "rollback", "1.0.0"))
	require.ErrorContains(t, err, "healthy state")
	enabled.ResultJSON = maintenanceHealthyProof(t, "1.0.0", `{"interval":99}`, 2)
	require.NoError(t, db.Save(&enabled).Error)
	_, err = CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "rollback", "1.0.0"))
	require.ErrorContains(t, err, "matching configuration")
	enabled.ResultJSON = maintenanceHealthyProof(t, "2.0.0", config.ConfigJSON, 2)
	require.NoError(t, db.Save(&enabled).Error)
	_, err = CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "rollback", "1.0.0"))
	require.Error(t, err)
	enabled.ResultJSON = maintenanceHealthyProof(t, "1.0.0", config.ConfigJSON, 2)
	require.NoError(t, db.Save(&enabled).Error)
	restored, err := CreateMaintenanceChange(db, 2, maintenanceOperationInput(nodeID, "rollback", "1.0.0"))
	require.NoError(t, err)
	require.Equal(t, config.ConfigJSON, restored.ConfigJSON)
	catalog, err := GetMaintenanceOperationCatalog(db, 0)
	require.NoError(t, err)
	require.Len(t, catalog.Nodes, 1)
	require.Equal(t, []string{"1.0.0"}, catalog.Nodes[0].RestoreVersions)
	encoded, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "test-only")
	require.NotContains(t, string(encoded), "config_json")
	require.NotContains(t, string(encoded), "127.0.0.1")
}

func TestMaintenanceConcurrentRequestKeyCreatesOneRequestAndAudit(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	input := maintenanceOperationInput(nodeID, "restart", "1.0.0")
	var group sync.WaitGroup
	results := make(chan *model.MaintenanceChange, 6)
	errs := make(chan error, 6)
	for range 6 {
		group.Add(1)
		go func() {
			defer group.Done()
			copy := input
			copy.NodeIDs = append([]uint{}, input.NodeIDs...)
			row, err := CreateMaintenanceChange(db, 2, copy)
			results <- row
			errs <- err
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var expected uint
	for row := range results {
		if expected == 0 {
			expected = row.ID
		}
		require.Equal(t, expected, row.ID)
	}
	var count int64
	require.NoError(t, db.Model(&model.MaintenanceChange{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.MaintenanceChangeAudit{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMaintenanceLegacyGuardDeniesOtherAdminsAndFailsClosedOnDatabaseError(t *testing.T) {
	db, _ := maintenanceOperationFixture(t)
	denied, err := BlockLegacyMaintenanceMutation(db, 3)
	require.NoError(t, err)
	require.True(t, denied)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = BlockLegacyMaintenanceMutation(db, 2)
	require.Error(t, err)
}

func TestMaintenanceChangeAuditRetentionProtectsPendingAndCurrentOverrides(t *testing.T) {
	db, nodeID := maintenanceOperationFixture(t)
	now := time.Now().UTC()
	old := now.AddDate(-2, 0, 0)
	makeChange := func(state string) *model.MaintenanceChange {
		op := model.KernelOperation{ID: uuid.NewString(), IdempotencyKey: uuid.NewString(), NodeID: &nodeID, PluginID: "machine-telemetry", Kind: "plugin.enable", State: state, ConfigJSON: "{}", CreatedAt: old, UpdatedAt: old}
		require.NoError(t, db.Create(&op).Error)
		ids, _ := json.Marshal([]string{op.ID})
		row := model.MaintenanceChange{RequestKey: uuid.NewString(), RequestedBy: 2, NodeIDsJSON: "[1]", PluginID: "machine-telemetry", TargetVersion: "1.0.0", Kind: "restart", ConfigJSON: "{}", Status: "queued", QueuedAt: &old, CreatedAt: old, OperationIDsJSON: string(ids)}
		require.NoError(t, db.Create(&row).Error)
		require.NoError(t, db.Create(&model.MaintenanceChangeAudit{ChangeID: row.ID, ActorID: 2, Action: "queued", CreatedAt: old}).Error)
		return &row
	}
	complete := makeChange("succeeded")
	pending := makeChange("running")
	pinned := makeChange("failed")
	require.NoError(t, db.Create(&model.MaintenancePluginOverride{NodeID: nodeID, PluginID: "machine-telemetry", ChangeID: pinned.ID, Generation: 1, Version: "1.0.0", ConfigJSON: "{}"}).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return CleanupMaintenanceChangesTx(tx, now) }))
	var rows []model.MaintenanceChange
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, pending.ID, rows[0].ID)
	require.Equal(t, pinned.ID, rows[1].ID)
	var count int64
	require.NoError(t, db.Model(&model.MaintenanceChangeAudit{}).Where("change_id = ?", complete.ID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&model.NodePluginLifecycle{}).Where("node_id = ?", nodeID).Update("desired_generation", 2).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return CleanupMaintenanceChangesTx(tx, now) }))
	require.NoError(t, db.Model(&model.MaintenanceChange{}).Where("id = ?", pinned.ID).Count(&count).Error)
	require.Zero(t, count)
}
