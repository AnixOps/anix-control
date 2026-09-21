package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrMaintenanceOperationDenied = errors.New("maintenance operation is not authorized")

type MaintenanceChangeInput struct {
	RequestKey    string          `json:"request_key"`
	NodeIDs       []uint          `json:"node_ids"`
	PluginID      string          `json:"plugin_id"`
	TargetVersion string          `json:"target_version"`
	Kind          string          `json:"kind"`
	Config        json.RawMessage `json:"config"`
}

func maintenanceChangeBinding(row model.MaintenanceChange) string {
	raw, _ := json.Marshal([]any{row.RequestedBy, row.NodeIDsJSON, row.PluginID, row.TargetVersion, row.Kind, row.ConfigHash})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func maintenanceChangeAudit(tx *gorm.DB, row model.MaintenanceChange, actorID uint, action string) error {
	return tx.Create(&model.MaintenanceChangeAudit{ChangeID: row.ID, ActorID: actorID, Action: action, BindingHash: row.BindingHash}).Error
}

func CreateMaintenanceChange(db *gorm.DB, actorID uint, input MaintenanceChangeInput) (*model.MaintenanceChange, error) {
	role, err := MaintenanceActorRole(db, actorID)
	if err != nil {
		return nil, err
	}
	if role != "owner" && role != "technician" {
		return nil, ErrMaintenanceOperationDenied
	}
	if len(input.NodeIDs) == 0 || len(input.NodeIDs) > 50 {
		return nil, errors.New("between 1 and 50 node IDs are required")
	}
	if _, err := uuid.Parse(input.RequestKey); err != nil {
		return nil, errors.New("request_key must be a UUID")
	}
	if input.PluginID != "machine-telemetry" || !safePluginSegment(input.TargetVersion) || len(input.TargetVersion) > 64 {
		return nil, errors.New("a machine-telemetry release version is required")
	}
	switch input.Kind {
	case "restart", "rollback", "install", "enable", "disable", "update", "configure":
	default:
		return nil, errors.New("unsupported maintenance action; database and system network changes have no executor")
	}
	if len(input.Config) == 0 {
		input.Config = json.RawMessage(`{}`)
	}
	if len(input.Config) > 16<<10 {
		return nil, errors.New("configuration exceeds 16 KiB")
	}
	canonical, err := CanonicalKernelOperationConfig(string(input.Config))
	if err != nil {
		return nil, err
	}
	if input.Kind != "update" && input.Kind != "configure" && input.Kind != "install" && canonical != "{}" {
		return nil, errors.New("this action does not accept configuration changes")
	}
	// Inline secrets are never accepted into an approval request or its UI.
	if topologyConfigContainsInlineSecret(canonical) {
		return nil, errors.New("configuration must use secret references")
	}
	sort.Slice(input.NodeIDs, func(i, j int) bool { return input.NodeIDs[i] < input.NodeIDs[j] })
	for i, nodeID := range input.NodeIDs {
		if nodeID == 0 || (i > 0 && input.NodeIDs[i-1] == nodeID) {
			return nil, errors.New("node IDs must be positive and unique")
		}
	}
	if input.Kind == "rollback" {
		for index, nodeID := range input.NodeIDs {
			verifiedConfig, err := maintenanceVerifiedConfig(db, nodeID, input.PluginID, input.TargetVersion)
			if err != nil {
				return nil, err
			}
			if index > 0 && canonical != verifiedConfig {
				return nil, errors.New("batch restore targets have different verified configurations; create separate requests")
			}
			canonical = verifiedConfig
		}
	}
	nodesJSON, _ := json.Marshal(input.NodeIDs)
	configHash, _ := HashKernelOperationConfig(canonical)
	row := model.MaintenanceChange{RequestKey: input.RequestKey, RequestedBy: actorID, NodeIDsJSON: string(nodesJSON), PluginID: input.PluginID, TargetVersion: input.TargetVersion, Kind: input.Kind, ConfigJSON: canonical, ConfigHash: configHash, Status: "pending"}
	row.BindingHash = maintenanceChangeBinding(row)
	err = WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var existing model.MaintenanceChange
		if err := tx.First(&existing, "request_key = ?", row.RequestKey).Error; err == nil {
			if existing.BindingHash != row.BindingHash {
				return errors.New("request_key is bound to another action")
			}
			row = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := validateMaintenanceChangeTargets(tx, row, input.NodeIDs); err != nil {
			return err
		}
		// Only these two narrow single-node repairs may execute without an owner.
		if len(input.NodeIDs) == 1 && (row.Kind == "restart" || row.Kind == "rollback") {
			now := time.Now().UTC()
			row.Status, row.ApprovedBy, row.ApprovedAt = "approved", actorID, &now
		}
		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_key"}}, DoNothing: true}).Create(&row)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.First(&existing, "request_key = ?", row.RequestKey).Error; err != nil {
				return err
			}
			if existing.BindingHash != row.BindingHash {
				return errors.New("request_key is bound to another action")
			}
			row = existing
			return nil
		}
		return maintenanceChangeAudit(tx, row, actorID, "requested")
	})
	return &row, err
}

// The authenticated Agent acknowledgement includes the Supervisor's persisted
// PluginState. Merely marking an operation succeeded is insufficient: the proof
// must report healthy+enabled for this plugin/version and bind its running config.
func maintenanceVerifiedConfig(tx *gorm.DB, nodeID uint, pluginID, version string) (string, error) {
	var proofs []model.KernelOperation
	if err := tx.Where("node_id = ? AND plugin_id = ? AND target_version = ? AND kind IN ? AND state IN ?", nodeID, pluginID, version, []string{"plugin.enable", "plugin.health", "plugin.rollback", "plugin.update", "plugin.configure"}, []string{"succeeded", "completed"}).Order("revision DESC").Limit(100).Find(&proofs).Error; err != nil {
		return "", err
	}
	for _, operation := range proofs {
		var proof struct {
			ID               string `json:"id"`
			DesiredVersion   string `json:"desired_version"`
			ObservedVersion  string `json:"observed_version"`
			Health           string `json:"health"`
			Enabled          bool   `json:"enabled"`
			ConfigHash       string `json:"config_hash"`
			ObservedRevision int64  `json:"observed_revision"`
		}
		if json.Unmarshal([]byte(operation.ResultJSON), &proof) != nil || proof.ID != pluginID || proof.DesiredVersion != version || proof.ObservedVersion != version || proof.Health != "healthy" || !proof.Enabled || !validSHA256Hex(proof.ConfigHash) || proof.ObservedRevision < operation.Revision {
			continue
		}
		var configured []model.KernelOperation
		if err := tx.Where("node_id = ? AND plugin_id = ? AND target_version = ? AND kind IN ? AND state IN ? AND revision <= ?", nodeID, pluginID, version, []string{"plugin.update", "plugin.configure", "plugin.rollback"}, []string{"succeeded", "completed"}, operation.Revision).Order("revision DESC").Limit(100).Find(&configured).Error; err != nil {
			return "", err
		}
		for _, candidate := range configured {
			canonical, err := CanonicalKernelOperationConfig(candidate.ConfigJSON)
			if err != nil {
				continue
			}
			digest, err := HashKernelOperationConfig(canonical)
			if err == nil && digest == proof.ConfigHash {
				return canonical, nil
			}
		}
	}
	return "", errors.New("rollback target has no successful execution evidence with healthy state and matching configuration on this node")
}

func validateMaintenanceChangeTargets(tx *gorm.DB, row model.MaintenanceChange, nodeIDs []uint) error {
	var release model.PluginRelease
	if err := tx.First(&release, "plugin_id = ? AND version = ?", row.PluginID, row.TargetVersion).Error; err != nil {
		return err
	}
	manifest, err := VerifyStoredPluginRelease(tx, release, nil)
	if err != nil {
		return err
	}
	if err := ValidatePluginReleaseTarget(release, "agent"); err != nil {
		return err
	}
	if row.Kind == "install" || row.Kind == "update" || row.Kind == "configure" || row.Kind == "rollback" {
		if err := validatePluginConfigurationSchema(*manifest, row.ConfigJSON); err != nil {
			return err
		}
	}
	for _, nodeID := range nodeIDs {
		var node model.Node
		if err := tx.First(&node, nodeID).Error; err != nil {
			return err
		}
		var assignments int64
		if err := tx.Model(&model.NodeServiceAssignment{}).Where("node_id = ? AND plugin_id = ? AND delete_pending = ?", nodeID, row.PluginID, false).Count(&assignments).Error; err != nil {
			return err
		}
		if assignments == 0 {
			return errors.New("node must have an existing plugin assignment")
		}
		if row.Kind == "restart" {
			var lifecycle model.NodePluginLifecycle
			if err := tx.First(&lifecycle, "node_id = ? AND plugin_id = ?", nodeID, row.PluginID).Error; err != nil {
				return err
			}
			if lifecycle.ActiveVersion != row.TargetVersion || lifecycle.DesiredVersion != row.TargetVersion {
				return errors.New("restart must use the node's current desired and active version")
			}
		}
		if row.Kind == "rollback" {
			restored, err := maintenanceVerifiedConfig(tx, nodeID, row.PluginID, row.TargetVersion)
			if err != nil {
				return err
			}
			if restored != row.ConfigJSON {
				return errors.New("verified restore configuration changed; create a new request")
			}
		}
	}
	return nil
}

func ApproveMaintenanceChange(db *gorm.DB, actorID, changeID uint, binding string) (*model.MaintenanceChange, error) {
	var row model.MaintenanceChange
	err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
		role, err := MaintenanceActorRole(tx, actorID)
		if err != nil {
			return err
		}
		if role != "owner" {
			return ErrMaintenanceOperationDenied
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, changeID).Error; err != nil {
			return err
		}
		if row.BindingHash != binding || row.BindingHash != maintenanceChangeBinding(row) {
			return errors.New("approval does not match the immutable operation")
		}
		if row.Status == "approved" || row.Status == "queued" {
			return nil
		}
		if row.Status != "pending" {
			return errors.New("change is not awaiting approval")
		}
		now := time.Now().UTC()
		result := tx.Model(&row).Where("status = ?", "pending").Updates(map[string]any{"status": "approved", "approved_by": actorID, "approved_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("change was concurrently modified")
		}
		row.Status, row.ApprovedBy, row.ApprovedAt = "approved", actorID, &now
		return maintenanceChangeAudit(tx, row, actorID, "approved")
	})
	return &row, err
}

func ExecuteMaintenanceChange(db *gorm.DB, actorID, changeID uint) (*model.MaintenanceChange, error) {
	var row model.MaintenanceChange
	err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
		role, err := MaintenanceActorRole(tx, actorID)
		if err != nil {
			return err
		}
		if role != "owner" && role != "technician" {
			return ErrMaintenanceOperationDenied
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, changeID).Error; err != nil {
			return err
		}
		if row.Status == "queued" {
			return nil
		}
		if row.Status != "approved" || row.BindingHash != maintenanceChangeBinding(row) {
			return ErrMaintenanceOperationDenied
		}
		hash, err := HashKernelOperationConfig(row.ConfigJSON)
		if err != nil || hash != row.ConfigHash {
			return errors.New("approved configuration was modified")
		}
		var nodeIDs []uint
		if err := json.Unmarshal([]byte(row.NodeIDsJSON), &nodeIDs); err != nil {
			return err
		}
		if len(nodeIDs) != 1 || (row.Kind != "restart" && row.Kind != "rollback") {
			approvingRole, err := MaintenanceActorRole(tx, row.ApprovedBy)
			if err != nil {
				return err
			}
			if approvingRole != "owner" {
				return ErrMaintenanceOperationDenied
			}
		}
		if err := validateMaintenanceChangeTargets(tx, row, nodeIDs); err != nil {
			return err
		}
		now := time.Now().UTC()
		claimed := tx.Model(&row).Where("status = ?", "approved").Updates(map[string]any{"status": "queued", "queued_at": now})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return errors.New("change was concurrently executed")
		}
		if err := LockPluginInstallationTarget(tx, "agent"); err != nil {
			return err
		}
		operationIDs := make([]string, 0)
		for _, nodeID := range nodeIDs {
			ids, err := queueMaintenanceNodeChange(tx, row, nodeID, now)
			if err != nil {
				return err
			}
			operationIDs = append(operationIDs, ids...)
		}
		encoded, _ := json.Marshal(operationIDs)
		row.OperationIDsJSON, row.Status, row.QueuedAt = string(encoded), "queued", &now
		if err := tx.Model(&row).Update("operation_ids_json", row.OperationIDsJSON).Error; err != nil {
			return err
		}
		return maintenanceChangeAudit(tx, row, actorID, "queued")
	})
	return &row, err
}

func queueMaintenanceNodeChange(tx *gorm.DB, change model.MaintenanceChange, nodeID uint, now time.Time) ([]string, error) {
	lifecycle, err := LockNodePluginLifecycleTx(tx, nodeID, change.PluginID)
	if err != nil {
		return nil, err
	}
	var inFlight int64
	if err := tx.Model(&model.KernelOperation{}).Where("node_id = ? AND plugin_id = ? AND state IN ?", nodeID, change.PluginID, []string{"pending", "dispatching", "dispatched", "acknowledged", "running", "cancel_requested"}).Count(&inFlight).Error; err != nil {
		return nil, err
	}
	if inFlight > 0 {
		return nil, errors.New("node plugin has an operation in progress")
	}
	enabled := change.Kind != "disable"
	if change.Kind == "configure" {
		enabled = lifecycle.DesiredEnabled
	}
	if err := tx.Model(&model.NodeServiceAssignment{}).Where("node_id = ? AND plugin_id = ? AND delete_pending = ?", nodeID, change.PluginID, false).Updates(map[string]any{"desired_version": change.TargetVersion, "enabled": enabled}).Error; err != nil {
		return nil, err
	}
	lifecycle.DesiredGeneration++
	lifecycle.DesiredVersion, lifecycle.DesiredEnabled = change.TargetVersion, enabled
	lifecycle.RetryEpoch, lifecycle.RetryExhausted, lifecycle.RetryAfter = 0, false, nil
	if err := tx.Save(lifecycle).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.NodeServiceAssignment{}).Where("node_id = ? AND plugin_id = ?", nodeID, change.PluginID).Update("lifecycle_generation", lifecycle.DesiredGeneration).Error; err != nil {
		return nil, err
	}
	override := model.MaintenancePluginOverride{NodeID: nodeID, PluginID: change.PluginID, ChangeID: change.ID, Generation: lifecycle.DesiredGeneration, Version: change.TargetVersion, ConfigJSON: change.ConfigJSON, CreatedAt: now}
	if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&override).Error; err != nil {
		return nil, err
	}
	var ids []string
	queue := func(kind, config string) error {
		dependency := ""
		if len(ids) > 0 {
			dependency = ids[len(ids)-1]
		}
		deadline := now.Add(20 * time.Minute)
		operation, _, err := CreateKernelOperation(tx, model.KernelOperation{ID: uuid.NewString(), IdempotencyKey: fmt.Sprintf("maintenance:%d:%d:%s", change.ID, nodeID, kind), NodeID: &nodeID, PluginID: change.PluginID, TargetVersion: change.TargetVersion, Kind: kind, ConfigJSON: config, DependsOnOperationID: dependency, DeadlineAt: &deadline})
		if err != nil {
			return err
		}
		ids = append(ids, operation.ID)
		return nil
	}
	switch change.Kind {
	case "restart":
		if err := queue("plugin.disable", `{}`); err != nil {
			return nil, err
		}
		if err := queue("plugin.enable", `{}`); err != nil {
			return nil, err
		}
	case "install", "update", "rollback":
		installConfig, err := BuildAgentPluginInstallConfig(tx, nodeID, change.PluginID, change.TargetVersion)
		if err != nil {
			return nil, err
		}
		if err := queue("plugin.install", installConfig); err != nil {
			return nil, err
		}
		if err := queue("plugin.update", change.ConfigJSON); err != nil {
			return nil, err
		}
		if err := queue("plugin.enable", `{}`); err != nil {
			return nil, err
		}
	case "configure":
		if err := queue("plugin.configure", change.ConfigJSON); err != nil {
			return nil, err
		}
	default:
		if err := queue("plugin."+change.Kind, `{}`); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// MaintenanceOverridesLifecycle prevents an old global assignment worker from
// replacing an explicit repair. Explicit subsequent assignment edits advance
// the generation and naturally revoke this node override.
func MaintenanceOverridesLifecycle(tx *gorm.DB, lifecycle model.NodePluginLifecycle) (bool, error) {
	if !tx.Migrator().HasTable(&model.MaintenancePluginOverride{}) {
		return false, nil
	}
	var count int64
	err := tx.Model(&model.MaintenancePluginOverride{}).Where("node_id = ? AND plugin_id = ? AND generation = ? AND version = ?", lifecycle.NodeID, lifecycle.PluginID, lifecycle.DesiredGeneration, lifecycle.DesiredVersion).Count(&count).Error
	return count > 0, err
}

func MaintenanceAuthorizesAgentRelease(tx *gorm.DB, nodeID uint, pluginID, version string) (bool, error) {
	if pluginID != "machine-telemetry" || !tx.Migrator().HasTable(&model.MaintenancePluginOverride{}) {
		return false, nil
	}
	var lifecycle model.NodePluginLifecycle
	if err := tx.First(&lifecycle, "node_id = ? AND plugin_id = ? AND desired_version = ?", nodeID, pluginID, version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return MaintenanceOverridesLifecycle(tx, lifecycle)
}

// BlockLegacyMaintenanceMutation keeps a designated technician's legacy admin
// flag from bypassing maintenance approval. Owners retain explicit legacy
// administration; all other maintenance operators use the scoped workflow.
// Settings table absence is the only legacy compatibility exception. Explicit
// catalog queries preserve errors (unlike Migrator.HasTable's boolean result).
func maintenanceSettingsTableExists(db *gorm.DB) (bool, error) {
	if db == nil {
		return false, errors.New("database is not initialized")
	}
	var count int64
	var err error
	switch db.Dialector.Name() {
	case "sqlite":
		err = db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", (model.MaintenanceSettings{}).TableName()).Scan(&count).Error
	case "postgres":
		err = db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?", (model.MaintenanceSettings{}).TableName()).Scan(&count).Error
	default:
		return false, errors.New("unsupported database for maintenance authorization")
	}
	return count > 0, err
}

func BlockLegacyMaintenanceMutation(db *gorm.DB, actorID uint) (bool, error) {
	exists, err := maintenanceSettingsTableExists(db)
	if err != nil || !exists {
		return false, err
	}
	settings, err := GetMaintenanceSettings(db)
	if err != nil {
		return false, err
	}
	return settings.OwnerID != 0 && settings.OwnerID != actorID, nil
}

// ChangesPublicConfig allows owners to inspect safe canonical values, without
// exposing any executor-generated artifact addresses or raw operation results.
func MaintenanceChangePublicConfig(row model.MaintenanceChange) json.RawMessage {
	if topologyConfigContainsInlineSecret(row.ConfigJSON) || !json.Valid([]byte(row.ConfigJSON)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(strings.TrimSpace(row.ConfigJSON))
}

// MaintenanceSettingsAvailable exposes the error-preserving compatibility probe.
func MaintenanceSettingsAvailable(db *gorm.DB) (bool, error) {
	return maintenanceSettingsTableExists(db)
}
