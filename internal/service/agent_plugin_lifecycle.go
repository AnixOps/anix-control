package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxAgentPluginLifecycleAttempts = 3

const maxSQLiteLifecycleTransactionAttempts = 6

type nodePluginDesiredState struct {
	Version        string
	ConfigRevision int64
	Enabled        bool
}

// WithAgentLifecycleTransaction retries only SQLite writer-contention errors.
// PostgreSQL locking and all semantic errors are returned immediately.
func WithAgentLifecycleTransaction(db *gorm.DB, mutation func(*gorm.DB) error) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if mutation == nil {
		return errors.New("lifecycle transaction mutation is required")
	}
	for attempt := 0; ; attempt++ {
		err := db.Transaction(mutation)
		if err == nil || db.Name() != "sqlite" || !isSQLiteBusyError(err) || attempt+1 >= maxSQLiteLifecycleTransactionAttempts {
			return err
		}
		time.Sleep(time.Duration(5*(1<<attempt)) * time.Millisecond)
	}
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked")
}

// SyncNodePluginLifecycle serializes every role of one node/plugin pair,
// computes aggregate desired state and optionally rearms an exhausted chain
// after an explicit administrator mutation.
func SyncNodePluginLifecycle(tx *gorm.DB, nodeID uint, pluginID string, explicitRetry bool, _ time.Time) (*model.NodePluginLifecycle, bool, error) {
	if tx == nil {
		return nil, false, errors.New("database is not initialized")
	}
	if nodeID == 0 || !safePluginSegment(pluginID) {
		return nil, false, errors.New("node plugin lifecycle identity is invalid")
	}
	lifecycle, err := lockNodePluginLifecycle(tx, nodeID, pluginID)
	if err != nil {
		return nil, false, err
	}
	observedChanged, err := refreshNodePluginObservedState(tx, lifecycle)
	if err != nil {
		return nil, false, err
	}
	desired, err := resolveNodePluginDesiredState(tx, lifecycle)
	if err != nil {
		return nil, false, err
	}
	changed := lifecycle.DesiredGeneration == 0 || lifecycle.DesiredVersion != desired.Version ||
		lifecycle.DesiredConfigRevision != desired.ConfigRevision || lifecycle.DesiredEnabled != desired.Enabled
	if !changed && explicitRetry && lifecycle.RetryExhausted {
		changed = true
	}
	if !changed {
		if observedChanged {
			if err := tx.Save(lifecycle).Error; err != nil {
				return nil, false, err
			}
		}
		return lifecycle, false, nil
	}
	if !desired.Enabled && desired.Version == "" {
		desired.Version = strings.TrimSpace(lifecycle.ActiveVersion)
		if desired.Version == "" {
			desired.Version = strings.TrimSpace(lifecycle.DesiredVersion)
		}
	}
	lifecycle.DesiredGeneration++
	if lifecycle.DesiredGeneration <= 0 {
		lifecycle.DesiredGeneration = 1
	}
	lifecycle.RetryEpoch, lifecycle.RetryAfter, lifecycle.RetryExhausted = 0, nil, false
	lifecycle.DesiredVersion, lifecycle.DesiredConfigRevision, lifecycle.DesiredEnabled = desired.Version, desired.ConfigRevision, desired.Enabled
	lifecycle.LastError = ""
	if err := tx.Save(lifecycle).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Model(&model.NodeServiceAssignment{}).
		Where("node_id = ? AND plugin_id = ?", nodeID, pluginID).
		Update("lifecycle_generation", lifecycle.DesiredGeneration).Error; err != nil {
		return nil, false, err
	}
	if err := supersedeOlderNodePluginOperations(tx, *lifecycle); err != nil {
		return nil, false, err
	}
	return lifecycle, true, nil
}

func lockNodePluginLifecycle(tx *gorm.DB, nodeID uint, pluginID string) (*model.NodePluginLifecycle, error) {
	seed := model.NodePluginLifecycle{NodeID: nodeID, PluginID: pluginID}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return nil, err
	}
	var lifecycle model.NodePluginLifecycle
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lifecycle, "node_id = ? AND plugin_id = ?", nodeID, pluginID).Error; err != nil {
		return nil, err
	}
	return &lifecycle, nil
}

// LockNodePluginLifecycleTx acquires the aggregate lock without recalculating
// desired state. Mutating handlers use it before locking assignment rows so all
// callers follow the same lifecycle -> assignment lock order.
func LockNodePluginLifecycleTx(tx *gorm.DB, nodeID uint, pluginID string) (*model.NodePluginLifecycle, error) {
	if tx == nil {
		return nil, errors.New("database is not initialized")
	}
	return lockNodePluginLifecycle(tx, nodeID, pluginID)
}

func refreshNodePluginObservedState(tx *gorm.DB, lifecycle *model.NodePluginLifecycle) (bool, error) {
	var operations []model.KernelOperation
	if err := tx.Where("node_id = ? AND plugin_id = ? AND state IN ? AND kind IN ?", lifecycle.NodeID, lifecycle.PluginID,
		[]string{"succeeded", "completed"}, []string{"plugin.update", "plugin.enable", "plugin.disable", "plugin.rollback"}).
		Order("revision DESC").Limit(32).Find(&operations).Error; err != nil {
		return false, err
	}
	version := strings.TrimSpace(lifecycle.ActiveVersion)
	minimumRevision := lifecycle.ActiveRevision
	activeRevision := lifecycle.ActiveRevision
	latestTransitionRevision := int64(-1)
	enabled := lifecycle.ActiveEnabled
	for _, operation := range operations {
		if operation.Revision < minimumRevision {
			continue
		}
		if operation.Revision > activeRevision {
			activeRevision = operation.Revision
			if strings.TrimSpace(operation.TargetVersion) != "" {
				version = operation.TargetVersion
			}
		}
		if operation.Revision > latestTransitionRevision {
			switch operation.Kind {
			case "plugin.enable", "plugin.rollback":
				enabled, latestTransitionRevision = true, operation.Revision
			case "plugin.disable":
				enabled, latestTransitionRevision = false, operation.Revision
			}
		}
	}
	changed := lifecycle.ActiveVersion != version || lifecycle.ActiveEnabled != enabled || lifecycle.ActiveRevision != activeRevision
	lifecycle.ActiveVersion, lifecycle.ActiveEnabled, lifecycle.ActiveRevision = version, enabled, activeRevision
	return changed, nil
}

func resolveNodePluginDesiredState(tx *gorm.DB, lifecycle *model.NodePluginLifecycle) (nodePluginDesiredState, error) {
	var installation model.PluginInstallation
	installationErr := tx.First(&installation, "plugin_id = ? AND target = ?", lifecycle.PluginID, "agent").Error
	if installationErr != nil && !errors.Is(installationErr, gorm.ErrRecordNotFound) {
		return nodePluginDesiredState{}, installationErr
	}
	var assignments []model.NodeServiceAssignment
	if err := tx.Where("node_id = ? AND plugin_id = ?", lifecycle.NodeID, lifecycle.PluginID).Order("id").Find(&assignments).Error; err != nil {
		return nodePluginDesiredState{}, err
	}
	var desired nodePluginDesiredState
	for _, assignment := range assignments {
		if assignment.DeletePending || !assignment.Enabled || strings.TrimSpace(assignment.DesiredVersion) == "" {
			continue
		}
		if !desired.Enabled {
			desired = nodePluginDesiredState{Version: assignment.DesiredVersion, ConfigRevision: assignment.DesiredConfigRevision, Enabled: true}
			continue
		}
		if desired.Version != assignment.DesiredVersion || desired.ConfigRevision != assignment.DesiredConfigRevision {
			return nodePluginDesiredState{}, fmt.Errorf("%w: enabled roles must share desired_version and desired_config_revision", ErrAgentPluginAssignmentConflict)
		}
	}
	if desired.Enabled {
		if errors.Is(installationErr, gorm.ErrRecordNotFound) {
			return nodePluginDesiredState{}, fmt.Errorf("%w: agent plugin installation is missing", ErrAgentPluginReconcileNotReady)
		}
		if !installation.Enabled {
			desired.Enabled = false
		} else if installation.DesiredVersion != desired.Version {
			return nodePluginDesiredState{}, fmt.Errorf("%w: installation version %s does not match assignment version %s", ErrAgentPluginReconcileNotReady, installation.DesiredVersion, desired.Version)
		} else {
			configuration, err := GetPluginConfiguration(tx, installation.ID)
			if err != nil {
				return nodePluginDesiredState{}, err
			}
			if configuration.Revision != desired.ConfigRevision {
				return nodePluginDesiredState{}, fmt.Errorf("%w: assignment config revision %d does not match installation revision %d", ErrAgentPluginReconcileNotReady, desired.ConfigRevision, configuration.Revision)
			}
			return desired, nil
		}
	}
	activeVersion, err := resolveNodePluginActiveVersion(tx, lifecycle, assignments)
	if err != nil {
		return nodePluginDesiredState{}, err
	}
	return nodePluginDesiredState{Version: activeVersion, Enabled: false}, nil
}

func resolveNodePluginActiveVersion(tx *gorm.DB, lifecycle *model.NodePluginLifecycle, assignments []model.NodeServiceAssignment) (string, error) {
	if active := strings.TrimSpace(lifecycle.ActiveVersion); active != "" {
		return active, nil
	}
	var observed model.KernelOperation
	err := tx.Where("node_id = ? AND plugin_id = ? AND state IN ? AND kind IN ?", lifecycle.NodeID, lifecycle.PluginID,
		[]string{"succeeded", "completed"}, []string{"plugin.update", "plugin.enable", "plugin.disable", "plugin.rollback"}).
		Order("revision DESC").First(&observed).Error
	if err == nil && strings.TrimSpace(observed.TargetVersion) != "" {
		return observed.TargetVersion, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if previous := strings.TrimSpace(lifecycle.DesiredVersion); previous != "" {
		return previous, nil
	}
	uniform := ""
	for _, assignment := range assignments {
		version := strings.TrimSpace(assignment.DesiredVersion)
		if version == "" {
			continue
		}
		if uniform == "" {
			uniform = version
			continue
		}
		if uniform != version {
			return "", nil
		}
	}
	return uniform, nil
}

// QueueAgentAssignmentLifecycle remains the focused entry point used by tests
// and callers that have already mutated one assignment transactionally.
func QueueAgentAssignmentLifecycle(tx *gorm.DB, assignment model.NodeServiceAssignment, now time.Time) (*AgentAssignmentOperationChain, error) {
	if assignment.NodeID == 0 || strings.TrimSpace(assignment.PluginID) == "" {
		return &AgentAssignmentOperationChain{}, nil
	}
	lifecycle, _, err := SyncNodePluginLifecycle(tx, assignment.NodeID, assignment.PluginID, false, now)
	if err != nil {
		return nil, err
	}
	return QueueNodePluginLifecycle(tx, lifecycle, now)
}

// QueueNodePluginLifecycle reuses a valid current chain before touching signed
// package metadata. Failed chains receive bounded retry epochs with backoff;
// successful or in-flight chains are never duplicated on periodic reconcile.
func QueueNodePluginLifecycle(tx *gorm.DB, lifecycle *model.NodePluginLifecycle, now time.Time) (*AgentAssignmentOperationChain, error) {
	if lifecycle == nil || lifecycle.DesiredGeneration <= 0 {
		return &AgentAssignmentOperationChain{}, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	if lifecycle.RetryExhausted {
		return &AgentAssignmentOperationChain{}, nil
	}
	if lifecycle.RetryAfter != nil && now.Before(*lifecycle.RetryAfter) {
		return &AgentAssignmentOperationChain{}, nil
	}
	kinds := nodePluginLifecycleOperationKinds(*lifecycle)
	if !lifecycle.DesiredEnabled {
		if strings.TrimSpace(lifecycle.DesiredVersion) == "" || !lifecycle.ActiveEnabled {
			return &AgentAssignmentOperationChain{}, nil
		}
	}
	existing, complete, err := loadNodePluginOperationChain(tx, *lifecycle, kinds)
	if err != nil {
		return nil, err
	}
	if complete {
		if chainSucceeded(existing) || chainInFlight(existing) {
			return mapNodePluginChain(existing), nil
		}
		return mapNodePluginChain(existing), scheduleNodePluginRetry(tx, lifecycle, now, existing)
	}
	if len(existing) != 0 {
		return nil, errors.New("node plugin lifecycle chain is incomplete")
	}
	if lifecycle.RetryAfter != nil {
		lifecycle.RetryAfter = nil
		if err := tx.Save(lifecycle).Error; err != nil {
			return nil, err
		}
	}
	if !lifecycle.DesiredEnabled {
		disable, err := queueNodePluginOperation(tx, *lifecycle, "plugin.disable", `{}`, "", now.Add(5*time.Minute))
		if err != nil {
			return nil, err
		}
		return &AgentAssignmentOperationChain{Disable: disable}, nil
	}

	installConfig, err := BuildAgentPluginInstallConfig(tx, lifecycle.NodeID, lifecycle.PluginID, lifecycle.DesiredVersion)
	if err != nil {
		return nil, err
	}
	var installation model.PluginInstallation
	if err := tx.First(&installation, "plugin_id = ? AND target = ? AND desired_version = ? AND enabled = ?", lifecycle.PluginID, "agent", lifecycle.DesiredVersion, true).Error; err != nil {
		return nil, fmt.Errorf("%w: enabled agent plugin installation is missing", ErrAgentPluginReconcileNotReady)
	}
	configuration, err := GetPluginConfiguration(tx, installation.ID)
	if err != nil {
		return nil, err
	}
	if configuration.Revision != lifecycle.DesiredConfigRevision {
		return nil, fmt.Errorf("%w: lifecycle config revision is stale", ErrAgentPluginReconcileNotReady)
	}
	install, err := queueNodePluginOperation(tx, *lifecycle, "plugin.install", installConfig, "", now.Add(10*time.Minute))
	if err != nil {
		return nil, err
	}
	update, err := queueNodePluginOperation(tx, *lifecycle, "plugin.update", configuration.ConfigJSON, install.ID, now.Add(15*time.Minute))
	if err != nil {
		return nil, err
	}
	enable, err := queueNodePluginOperation(tx, *lifecycle, "plugin.enable", `{}`, update.ID, now.Add(20*time.Minute))
	if err != nil {
		return nil, err
	}
	return &AgentAssignmentOperationChain{Install: install, Update: update, Enable: enable}, nil
}

func loadNodePluginOperationChain(tx *gorm.DB, lifecycle model.NodePluginLifecycle, kinds []string) ([]model.KernelOperation, bool, error) {
	operations := make([]model.KernelOperation, 0, len(kinds))
	for _, kind := range kinds {
		var operation model.KernelOperation
		err := tx.First(&operation, "idempotency_key = ?", nodePluginOperationKey(lifecycle, kind)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		operations = append(operations, operation)
	}
	return operations, len(operations) == len(kinds), nil
}

func chainSucceeded(operations []model.KernelOperation) bool {
	for _, operation := range operations {
		if operation.State != "succeeded" && operation.State != "completed" {
			return false
		}
	}
	return len(operations) > 0
}

func chainInFlight(operations []model.KernelOperation) bool {
	for _, operation := range operations {
		switch operation.State {
		case "pending", "dispatching", "running", "cancel_requested":
			return true
		}
	}
	return false
}

func scheduleNodePluginRetry(tx *gorm.DB, lifecycle *model.NodePluginLifecycle, now time.Time, operations []model.KernelOperation) error {
	if lifecycle.RetryEpoch+1 >= maxAgentPluginLifecycleAttempts {
		lifecycle.RetryExhausted = true
		lifecycle.RetryAfter = nil
		lifecycle.LastError = summarizeOperationChainFailure(operations)
		if err := tx.Save(lifecycle).Error; err != nil {
			return err
		}
		return nil
	}
	lifecycle.RetryEpoch++
	delay := time.Duration(5*(1<<(lifecycle.RetryEpoch-1))) * time.Second
	if delay > 2*time.Minute {
		delay = 2 * time.Minute
	}
	retryAt := now.Add(delay)
	lifecycle.RetryAfter = &retryAt
	lifecycle.LastError = summarizeOperationChainFailure(operations)
	if err := tx.Save(lifecycle).Error; err != nil {
		return err
	}
	return nil
}

func nodePluginLifecycleOperationKinds(lifecycle model.NodePluginLifecycle) []string {
	if lifecycle.DesiredEnabled {
		return []string{"plugin.install", "plugin.update", "plugin.enable"}
	}
	return []string{"plugin.disable"}
}

func summarizeOperationChainFailure(operations []model.KernelOperation) string {
	for _, operation := range operations {
		if operation.State == "failed" || operation.State == "timed_out" || operation.State == "cancelled" || operation.State == "superseded" {
			message := strings.TrimSpace(operation.LastError)
			if message == "" {
				message = operation.State
			}
			return operation.Kind + ": " + message
		}
	}
	return "plugin lifecycle chain did not succeed"
}

func mapNodePluginChain(operations []model.KernelOperation) *AgentAssignmentOperationChain {
	chain := &AgentAssignmentOperationChain{}
	for index := range operations {
		operation := operations[index]
		switch operation.Kind {
		case "plugin.install":
			chain.Install = &operation
		case "plugin.update":
			chain.Update = &operation
		case "plugin.enable":
			chain.Enable = &operation
		case "plugin.disable":
			chain.Disable = &operation
		}
	}
	return chain
}

func queueNodePluginOperation(tx *gorm.DB, lifecycle model.NodePluginLifecycle, kind, configJSON, dependsOn string, deadline time.Time) (*model.KernelOperation, error) {
	idempotencyKey := nodePluginOperationKey(lifecycle, kind)
	canonical, err := CanonicalKernelOperationConfig(configJSON)
	if err != nil {
		return nil, err
	}
	configHash, err := HashKernelOperationConfig(canonical)
	if err != nil {
		return nil, err
	}
	var existing model.KernelOperation
	if err := tx.First(&existing, "idempotency_key = ?", idempotencyKey).Error; err == nil {
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	operation, _, err := CreateKernelOperation(tx, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: idempotencyKey, NodeID: &lifecycle.NodeID,
		PluginID: lifecycle.PluginID, TargetVersion: lifecycle.DesiredVersion, Kind: kind,
		ConfigJSON: canonical, ConfigHash: configHash, DependsOnOperationID: dependsOn, DeadlineAt: &deadline,
	})
	return operation, err
}

func nodePluginOperationKey(lifecycle model.NodePluginLifecycle, kind string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"agent-lifecycle", strconv.FormatUint(uint64(lifecycle.NodeID), 10), lifecycle.PluginID,
		strconv.FormatInt(lifecycle.DesiredGeneration, 10), strconv.Itoa(lifecycle.RetryEpoch),
		lifecycle.DesiredVersion, strconv.FormatInt(lifecycle.DesiredConfigRevision, 10), strconv.FormatBool(lifecycle.DesiredEnabled), kind,
	}, "\x00")))
	return "agent-lifecycle:" + hex.EncodeToString(digest[:])
}

func supersedeOlderNodePluginOperations(tx *gorm.DB, lifecycle model.NodePluginLifecycle) error {
	currentKeys := make([]string, 0, 5)
	for _, kind := range []string{"plugin.install", "plugin.configure", "plugin.update", "plugin.enable", "plugin.disable"} {
		currentKeys = append(currentKeys, nodePluginOperationKey(lifecycle, kind))
	}
	return tx.Model(&model.KernelOperation{}).
		Where("node_id = ? AND plugin_id = ? AND state = ? AND (idempotency_key LIKE ? OR idempotency_key LIKE ?) AND idempotency_key NOT IN ?",
			lifecycle.NodeID, lifecycle.PluginID, "pending", "agent-lifecycle:%", "agent-assignment:%", currentKeys).
		Updates(map[string]any{"state": "superseded", "last_error": "superseded by newer node plugin lifecycle generation"}).Error
}

// ReconcileAgentAssignments resumes enrolled lifecycle rows. It does not scan
// legacy generation-zero assignments, preserving the opt-in migration gate.
func ReconcileAgentAssignments(db *gorm.DB, now time.Time) (int, error) {
	if db == nil {
		return 0, errors.New("database is not initialized")
	}
	var rows []model.NodePluginLifecycle
	if err := db.Where("desired_generation > ?", 0).Order("node_id, plugin_id").Find(&rows).Error; err != nil {
		return 0, err
	}
	reconciled := 0
	var reconcileErrors []error
	for _, row := range rows {
		err := WithAgentLifecycleTransaction(db, func(tx *gorm.DB) error {
			lifecycle, _, err := SyncNodePluginLifecycle(tx, row.NodeID, row.PluginID, false, now)
			if err != nil {
				return err
			}
			chain, err := QueueNodePluginLifecycle(tx, lifecycle, now)
			if err != nil {
				return err
			}
			return purgeCompletedAssignmentDeletes(tx, lifecycle, chain)
		})
		if err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("reconcile node %d plugin %s: %w", row.NodeID, row.PluginID, err))
			continue
		}
		if err := RefreshAgentPluginInstallationObservedState(db, row.PluginID); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("project plugin %s installation state: %w", row.PluginID, err))
		}
		reconciled++
	}
	return reconciled, errors.Join(reconcileErrors...)
}

func purgeCompletedAssignmentDeletes(tx *gorm.DB, lifecycle *model.NodePluginLifecycle, chain *AgentAssignmentOperationChain) error {
	var pending int64
	if err := tx.Model(&model.NodeServiceAssignment{}).
		Where("node_id = ? AND plugin_id = ? AND delete_pending = ?", lifecycle.NodeID, lifecycle.PluginID, true).Count(&pending).Error; err != nil || pending == 0 {
		return err
	}
	ready := false
	if lifecycle.DesiredEnabled {
		ready = chain != nil && chain.Enable != nil && (chain.Enable.State == "succeeded" || chain.Enable.State == "completed")
	} else if strings.TrimSpace(lifecycle.DesiredVersion) == "" || !lifecycle.ActiveEnabled {
		ready = true
	} else {
		ready = chain != nil && chain.Disable != nil && (chain.Disable.State == "succeeded" || chain.Disable.State == "completed")
	}
	if !ready {
		return nil
	}
	return tx.Where("node_id = ? AND plugin_id = ? AND delete_pending = ?", lifecycle.NodeID, lifecycle.PluginID, true).
		Delete(&model.NodeServiceAssignment{}).Error
}

// PurgeCompletedAssignmentDeletes is used by handlers when dispatch is
// explicitly disabled and the observed plugin is already inactive.
func PurgeCompletedAssignmentDeletes(tx *gorm.DB, lifecycle *model.NodePluginLifecycle, chain *AgentAssignmentOperationChain) error {
	return purgeCompletedAssignmentDeletes(tx, lifecycle, chain)
}

// SyncAgentInstallationAssignments updates every matching assignment revision
// and aggregate generation in the caller's configuration transaction.
func SyncAgentInstallationAssignments(tx *gorm.DB, installation model.PluginInstallation, configRevision int64, queue bool, now time.Time) ([]*model.KernelOperation, error) {
	if installation.Target != "agent" {
		return nil, nil
	}
	if err := LockPluginInstallationTarget(tx, "agent"); err != nil {
		return nil, err
	}
	var nodeIDs []uint
	if err := tx.Model(&model.NodeServiceAssignment{}).
		Where("plugin_id = ?", installation.PluginID).Distinct("node_id").Order("node_id").Pluck("node_id", &nodeIDs).Error; err != nil {
		return nil, err
	}
	// Lock every aggregate in a deterministic order before taking assignment
	// row locks. This is the same order used by per-node assignment mutations.
	sort.Slice(nodeIDs, func(i, j int) bool { return nodeIDs[i] < nodeIDs[j] })
	for _, nodeID := range nodeIDs {
		if _, err := LockNodePluginLifecycleTx(tx, nodeID, installation.PluginID); err != nil {
			return nil, err
		}
	}
	var assignments []model.NodeServiceAssignment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("plugin_id = ?", installation.PluginID).Order("node_id, id").Find(&assignments).Error; err != nil {
		return nil, err
	}
	for _, assignment := range assignments {
		updates := map[string]any{}
		// The Agent installation is the package-version authority for existing
		// assignments. Preserve an explicitly cleared version and a soft-delete
		// intent, but advance every other role atomically with the installation.
		// SyncNodePluginLifecycle below then creates one new aggregate generation
		// for the node/plugin pair instead of reconciling stale role versions.
		if !assignment.DeletePending && strings.TrimSpace(assignment.DesiredVersion) != "" && assignment.DesiredVersion != installation.DesiredVersion {
			updates["desired_version"] = installation.DesiredVersion
			assignment.DesiredVersion = installation.DesiredVersion
		}
		if !assignment.DeletePending && strings.TrimSpace(assignment.DesiredVersion) == installation.DesiredVersion && assignment.DesiredConfigRevision != configRevision {
			updates["desired_config_revision"] = configRevision
			assignment.DesiredConfigRevision = configRevision
		}
		if len(updates) > 0 {
			if err := tx.Model(&model.NodeServiceAssignment{}).Where("id = ?", assignment.ID).Updates(updates).Error; err != nil {
				return nil, err
			}
		}
	}
	var operations []*model.KernelOperation
	for _, nodeID := range nodeIDs {
		lifecycle, _, err := SyncNodePluginLifecycle(tx, nodeID, installation.PluginID, false, now)
		if err != nil {
			return nil, err
		}
		if !queue {
			continue
		}
		chain, err := QueueNodePluginLifecycle(tx, lifecycle, now)
		if err != nil {
			return nil, err
		}
		for _, operation := range []*model.KernelOperation{chain.Install, chain.Update, chain.Enable, chain.Disable} {
			if operation != nil {
				operations = append(operations, operation)
			}
		}
	}
	return operations, nil
}

// RecordNodePluginOperationSuccessTx captures the version actually selected by
// the Agent, so later disable/delete never guesses from a stale role row.
func RecordNodePluginOperationSuccessTx(tx *gorm.DB, operation model.KernelOperation) error {
	if tx == nil || operation.NodeID == nil {
		return nil
	}
	var lifecycle model.NodePluginLifecycle
	if err := tx.First(&lifecycle, "node_id = ? AND plugin_id = ?", *operation.NodeID, operation.PluginID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if operation.Revision < lifecycle.ActiveRevision {
		return nil
	}
	updates := map[string]any{}
	switch operation.Kind {
	case "plugin.update", "plugin.enable", "plugin.rollback":
		updates["active_version"] = operation.TargetVersion
		updates["active_revision"] = operation.Revision
		if operation.Kind == "plugin.enable" || operation.Kind == "plugin.rollback" {
			updates["active_enabled"] = true
		}
	case "plugin.disable":
		updates["active_version"] = operation.TargetVersion
		updates["active_enabled"] = false
		updates["active_revision"] = operation.Revision
	}
	if len(updates) == 0 {
		return nil
	}
	if err := tx.Model(&lifecycle).Updates(updates).Error; err != nil {
		return err
	}
	finalCurrentOperation := operation.IdempotencyKey == nodePluginOperationKey(lifecycle, operation.Kind) &&
		((lifecycle.DesiredEnabled && operation.Kind == "plugin.enable") || (!lifecycle.DesiredEnabled && operation.Kind == "plugin.disable"))
	if finalCurrentOperation {
		operations, complete, err := loadNodePluginOperationChain(tx, lifecycle, nodePluginLifecycleOperationKinds(lifecycle))
		if err != nil {
			return err
		}
		if complete && chainSucceeded(operations) {
			if err := tx.Model(&lifecycle).Updates(map[string]any{
				"retry_after": nil, "retry_exhausted": false, "last_error": "",
			}).Error; err != nil {
				return err
			}
			lifecycle.RetryAfter, lifecycle.RetryExhausted, lifecycle.LastError = nil, false, ""
		}
	}
	return nil
}

// RefreshAgentPluginInstallationObservedState projects node lifecycle state in
// a separate, consistently ordered transaction. Lifecycle mutations must not
// call this while holding an aggregate lock because global installation writes
// use target -> installation -> lifecycle ordering.
func RefreshAgentPluginInstallationObservedState(db *gorm.DB, pluginID string) error {
	if db == nil || strings.TrimSpace(pluginID) == "" {
		return nil
	}
	return WithAgentLifecycleTransaction(db, func(tx *gorm.DB) error {
		if err := LockPluginInstallationTarget(tx, "agent"); err != nil {
			return err
		}
		var installation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&installation, "plugin_id = ? AND target = ?", pluginID, "agent").Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		return refreshAgentPluginInstallationObservedStateTx(tx, pluginID)
	})
}

// refreshAgentPluginInstallationObservedStateTx assumes the caller already
// holds the Agent target and installation row locks.
func refreshAgentPluginInstallationObservedStateTx(tx *gorm.DB, pluginID string) error {
	if tx == nil || strings.TrimSpace(pluginID) == "" {
		return nil
	}
	var installation model.PluginInstallation
	if err := tx.First(&installation, "plugin_id = ? AND target = ?", pluginID, "agent").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !installation.Enabled {
		return tx.Model(&installation).Updates(map[string]any{"state": "disabled", "last_error": ""}).Error
	}
	var lifecycles []model.NodePluginLifecycle
	if err := tx.Where("plugin_id = ? AND desired_generation > ?", pluginID, 0).Order("node_id").Find(&lifecycles).Error; err != nil {
		return err
	}
	if len(lifecycles) == 0 {
		return tx.Model(&installation).Updates(map[string]any{"state": "pending", "last_error": ""}).Error
	}
	allConverged := true
	failed := false
	degraded := false
	lastError := ""
	for _, lifecycle := range lifecycles {
		if lifecycle.RetryExhausted {
			failed = true
		}
		if strings.TrimSpace(lifecycle.LastError) != "" {
			degraded = true
			if lastError == "" {
				lastError = lifecycle.LastError
			}
		}
		kinds := nodePluginLifecycleOperationKinds(lifecycle)
		operations, complete, err := loadNodePluginOperationChain(tx, lifecycle, kinds)
		if err != nil {
			return err
		}
		for _, operation := range operations {
			if operation.State == "failed" || operation.State == "superseded" || operation.State == "cancelled" || operation.State == "timed_out" {
				degraded = true
				if lastError == "" {
					lastError = summarizeOperationChainFailure(operations)
				}
			}
		}
		converged := false
		if lifecycle.DesiredEnabled {
			converged = complete && chainSucceeded(operations) && lifecycle.ActiveEnabled && lifecycle.ActiveVersion == lifecycle.DesiredVersion
		} else {
			converged = !lifecycle.ActiveEnabled
		}
		allConverged = allConverged && converged
	}
	updates := map[string]any{"last_error": lastError}
	switch {
	case failed:
		updates["state"] = "failed"
	case degraded:
		updates["state"] = "degraded"
	case allConverged:
		updates["state"] = "healthy"
		updates["observed_version"] = installation.DesiredVersion
		updates["last_error"] = ""
	default:
		updates["state"] = "pending"
		updates["last_error"] = ""
	}
	return tx.Model(&installation).Updates(updates).Error
}
