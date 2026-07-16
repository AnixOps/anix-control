package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/AnixOps/anix-control/v3/internal/service"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const controlOperationLeaseDuration = 30 * time.Second

var errControlOperationLeaseLost = errors.New("control plugin operation lease lost")

var controlOperationKinds = []string{
	"plugin.inspect", "plugin.install", "plugin.configure", "plugin.enable",
	"plugin.disable", "plugin.update", "plugin.rollback", "plugin.health",
}

// OperationWorker executes durable Control-target operations. Node-target
// operations remain owned by KernelOperationBridge and the Agent stream.
type OperationWorker struct {
	db            *gorm.DB
	registry      *Registry
	now           func() time.Time
	workerID      string
	leaseDuration time.Duration
}

func NewOperationWorker(db *gorm.DB, registry *Registry) (*OperationWorker, error) {
	if db == nil {
		return nil, errors.New("control plugin operation worker requires a database")
	}
	if registry == nil {
		return nil, errors.New("control plugin operation worker requires an executor registry")
	}
	return &OperationWorker{
		db: db, registry: registry, now: time.Now, workerID: uuid.NewString(), leaseDuration: controlOperationLeaseDuration,
	}, nil
}

func (w *OperationWorker) RunOnce(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := w.now()
	if _, err := service.ExpireKernelOperations(w.db, now); err != nil {
		return 0, err
	}
	// A Control process may recover work only after the previous owner's lease
	// expires. Active workers renew their lease while package code is running.
	if err := w.db.Model(&model.KernelOperation{}).
		Where("node_id IS NULL AND state = ? AND kind IN ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?)", "running", controlOperationKinds, now).
		Updates(map[string]any{
			"state": "pending", "claimed_by": "", "lease_expires_at": nil,
			"last_error": "Control executor lease expired; retrying durable operation",
		}).Error; err != nil {
		return 0, err
	}

	var operations []model.KernelOperation
	if err := w.db.Where("node_id IS NULL AND state = ? AND kind IN ?", "pending", controlOperationKinds).
		Order("created_at, id").Find(&operations).Error; err != nil {
		return 0, err
	}
	processed := 0
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		claimed, err := w.executeOne(ctx, operation)
		if err != nil {
			return processed, err
		}
		if claimed {
			processed++
		}
	}
	return processed, nil
}

func (w *OperationWorker) Start(ctx context.Context, interval time.Duration, reportError func(error)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		run := func() {
			if _, err := w.RunOnce(ctx); err != nil && reportError != nil && !errors.Is(err, context.Canceled) {
				reportError(err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// QueueReconciliation records one enable operation per currently enabled
// Control package for this boot. Calling it again with the same boot ID is
// idempotent; a new Control process uses a new boot ID and therefore proves
// restart reconciliation with a new durable operation.
func (w *OperationWorker) QueueReconciliation(bootID string) (int, error) {
	bootID = strings.TrimSpace(bootID)
	if bootID == "" || strings.ContainsAny(bootID, "\r\n\x00") {
		return 0, errors.New("control reconciliation boot id is required")
	}
	var installations []model.PluginInstallation
	if err := w.db.Where("target = ? AND enabled = ?", "control", true).Order("plugin_id").Find(&installations).Error; err != nil {
		return 0, err
	}
	queued := 0
	var reconcileErr error
	for _, installation := range installations {
		idempotencyKey := fmt.Sprintf("control-reconcile:%s:%d:%s:%d", bootID, installation.ID, installation.DesiredVersion, installation.ConfigRevision)
		var count int64
		if err := w.db.Model(&model.KernelOperation{}).Where("idempotency_key = ?", idempotencyKey).Count(&count).Error; err != nil {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("reconcile %s: %w", installation.PluginID, err))
			continue
		}
		if count > 0 {
			continue
		}
		configuration, err := service.GetPluginConfiguration(w.db, installation.ID)
		if err != nil {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("reconcile %s configuration: %w", installation.PluginID, err))
			continue
		}
		deadline := w.now().Add(5 * time.Minute)
		_, reused, err := service.CreateKernelOperation(w.db, model.KernelOperation{
			ID: uuid.NewString(), IdempotencyKey: idempotencyKey,
			PluginID: installation.PluginID, TargetVersion: installation.DesiredVersion,
			Kind: "plugin.enable", ConfigJSON: configuration.ConfigJSON, DeadlineAt: &deadline,
		})
		if err != nil {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("reconcile %s operation: %w", installation.PluginID, err))
			continue
		}
		if !reused {
			queued++
		}
	}
	return queued, reconcileErr
}

func (w *OperationWorker) executeOne(ctx context.Context, operation model.KernelOperation) (bool, error) {
	claimedAt := w.now()
	claimed, err := w.claimOperation(&operation, claimedAt)
	if err != nil || !claimed {
		return claimed, err
	}

	var installation model.PluginInstallation
	if err := w.db.First(&installation, "plugin_id = ? AND target = ?", operation.PluginID, "control").Error; err != nil {
		return true, w.recordFailure(operation, nil, fmt.Errorf("control plugin installation is missing: %w", err), false)
	}
	if operation.DeadlineAt == nil {
		return true, w.recordFailure(operation, &installation, errors.New("operation deadline is missing"), false)
	}
	if !installationWantsOperation(installation, operation) {
		return true, w.recordSuperseded(operation, "installation desired state changed before execution")
	}
	operationCtx, cancelOperation := context.WithCancel(ctx)
	defer cancelOperation()
	leaseDone := make(chan struct{})
	leaseStopped := make(chan struct{})
	leaseErrors := make(chan error, 1)
	go w.renewLease(operationCtx, cancelOperation, operation.ID, leaseDone, leaseStopped, leaseErrors)
	leaseActive := true
	stopLease := func() {
		if !leaseActive {
			return
		}
		leaseActive = false
		close(leaseDone)
		<-leaseStopped
	}
	defer stopLease()
	executionCtx, cancelExecution := context.WithDeadline(operationCtx, *operation.DeadlineAt)
	result, executeErr := w.registry.ExecuteLifecycle(executionCtx, operation.PluginID, operation.TargetVersion, LifecycleRequest{
		OperationID: operation.ID, Kind: operation.Kind, Target: operation.TargetVersion, Config: json.RawMessage(operation.ConfigJSON),
	})
	cancelExecution()
	rollbackSucceeded := false
	if executeErr != nil && (operation.Kind == "plugin.update" || operation.Kind == "plugin.rollback") && installation.Enabled && installation.ObservedVersion != "" && installation.ObservedVersion != operation.TargetVersion {
		rollbackCtx, cancelRollback := context.WithTimeout(operationCtx, 2*time.Minute)
		_, rollbackErr := w.registry.ExecuteLifecycle(rollbackCtx, installation.PluginID, installation.ObservedVersion, LifecycleRequest{
			OperationID: operation.ID + ":automatic-rollback", Kind: "plugin.enable", Target: installation.ObservedVersion,
			Config: json.RawMessage(operation.ConfigJSON),
		})
		cancelRollback()
		rollbackSucceeded = rollbackErr == nil
		if rollbackErr != nil {
			executeErr = errors.Join(executeErr, fmt.Errorf("automatic rollback failed: %w", rollbackErr))
		}
	}
	stopLease()
	select {
	case leaseErr := <-leaseErrors:
		return true, leaseErr
	default:
	}
	if executeErr != nil {
		return true, w.recordFailure(operation, &installation, executeErr, rollbackSucceeded)
	}
	if len(result) == 0 {
		result = json.RawMessage(`{}`)
	}
	if !json.Valid(result) {
		return true, w.recordFailure(operation, &installation, errors.New("control plugin executor returned invalid JSON"), false)
	}
	return true, w.recordSuccess(operation, installation.ID, result)
}

func (w *OperationWorker) claimOperation(operation *model.KernelOperation, claimedAt time.Time) (bool, error) {
	claimed := false
	err := w.db.Transaction(func(tx *gorm.DB) error {
		var installation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&installation, "plugin_id = ? AND target = ?", operation.PluginID, "control").Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var earliest model.KernelOperation
		err := tx.Where(
			"node_id IS NULL AND plugin_id = ? AND state IN ? AND kind IN ?",
			operation.PluginID, []string{"pending", "dispatching", "running", "cancel_requested"}, controlOperationKinds,
		).Order("created_at, id").First(&earliest).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if earliest.ID != operation.ID || earliest.State != "pending" {
			return nil
		}
		leaseExpiresAt := claimedAt.Add(w.leaseDuration)
		claim := tx.Model(&model.KernelOperation{}).Where("id = ? AND state = ?", operation.ID, "pending").Updates(map[string]any{
			"state": "running", "dispatched_at": claimedAt, "acknowledged_at": claimedAt, "last_error": "",
			"claimed_by": w.workerID, "lease_expires_at": leaseExpiresAt, "attempt": gorm.Expr("attempt + 1"),
		})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			return nil
		}
		if err := tx.First(operation, "id = ?", operation.ID).Error; err != nil {
			return err
		}
		claimed = true
		return nil
	})
	return claimed, err
}

func (w *OperationWorker) renewLease(ctx context.Context, cancel context.CancelFunc, operationID string, done <-chan struct{}, stopped chan<- struct{}, leaseErrors chan<- error) {
	defer close(stopped)
	interval := w.leaseDuration / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			expiresAt := w.now().Add(w.leaseDuration)
			result := w.db.Model(&model.KernelOperation{}).
				Where("id = ? AND state = ? AND claimed_by = ?", operationID, "running", w.workerID).
				Update("lease_expires_at", expiresAt)
			if result.Error == nil && result.RowsAffected == 1 {
				continue
			}
			err := result.Error
			if err == nil {
				err = errControlOperationLeaseLost
			} else {
				err = fmt.Errorf("%w: %v", errControlOperationLeaseLost, err)
			}
			select {
			case leaseErrors <- err:
			default:
			}
			cancel()
			return
		}
	}
}

func (w *OperationWorker) recordSuccess(operation model.KernelOperation, installationID uint, result json.RawMessage) error {
	now := w.now()
	return w.db.Transaction(func(tx *gorm.DB) error {
		var currentOperation model.KernelOperation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentOperation, "id = ?", operation.ID).Error; err != nil {
			return err
		}
		if currentOperation.State == "cancel_requested" {
			return tx.Model(&currentOperation).Updates(map[string]any{
				"state": "cancelled", "result_json": string(result), "observed_at": now,
				"last_error": "operation completed after cancellation was requested", "claimed_by": "", "lease_expires_at": nil,
			}).Error
		}
		if currentOperation.State != "running" || currentOperation.ClaimedBy != w.workerID {
			return nil
		}
		var installation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&installation, installationID).Error; err != nil {
			return err
		}
		if !installationWantsOperation(installation, operation) {
			return tx.Model(&currentOperation).Updates(map[string]any{
				"state": "superseded", "result_json": string(result), "observed_at": now,
				"last_error": "installation desired state changed during execution", "claimed_by": "", "lease_expires_at": nil,
			}).Error
		}
		if err := tx.Model(&currentOperation).Updates(map[string]any{
			"state": "succeeded", "result_json": string(result), "observed_at": now,
			"last_error": "", "claimed_by": "", "lease_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		return applyInstallationSuccess(tx, &installation, operation, now)
	})
}

func applyInstallationSuccess(tx *gorm.DB, installation *model.PluginInstallation, operation model.KernelOperation, now time.Time) error {
	installation.LastError = ""
	switch operation.Kind {
	case "plugin.install":
		installation.ObservedVersion, installation.State = operation.TargetVersion, "installed"
	case "plugin.enable":
		installation.DesiredVersion, installation.ObservedVersion = operation.TargetVersion, operation.TargetVersion
		installation.Enabled, installation.State, installation.DisabledAt = true, "healthy", nil
	case "plugin.disable":
		installation.Enabled, installation.State, installation.DisabledAt = false, "disabled", &now
	case "plugin.update", "plugin.rollback":
		previous := installation.ObservedVersion
		if previous != "" && previous != operation.TargetVersion {
			installation.PreviousVersion = previous
		}
		installation.DesiredVersion, installation.ObservedVersion = operation.TargetVersion, operation.TargetVersion
		installation.DisabledAt = nil
		if installation.Enabled {
			installation.State = "healthy"
		} else {
			installation.State = "installed"
		}
	case "plugin.configure", "plugin.health":
		if installation.Enabled {
			installation.State = "healthy"
		}
	case "plugin.inspect":
	}
	return tx.Save(installation).Error
}

func (w *OperationWorker) recordFailure(operation model.KernelOperation, installation *model.PluginInstallation, operationErr error, rollbackSucceeded bool) error {
	message := strings.TrimSpace(operationErr.Error())
	if message == "" {
		message = "control plugin operation failed"
	}
	now := w.now()
	return w.db.Transaction(func(tx *gorm.DB) error {
		var currentOperation model.KernelOperation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentOperation, "id = ?", operation.ID).Error; err != nil {
			return err
		}
		if currentOperation.State == "cancel_requested" {
			return tx.Model(&currentOperation).Updates(map[string]any{
				"state": "cancelled", "observed_at": now, "last_error": message,
				"claimed_by": "", "lease_expires_at": nil,
			}).Error
		}
		if currentOperation.State != "running" || currentOperation.ClaimedBy != w.workerID {
			return nil
		}
		if installation == nil {
			return tx.Model(&currentOperation).Updates(map[string]any{
				"state": "failed", "observed_at": now, "last_error": message,
				"claimed_by": "", "lease_expires_at": nil,
			}).Error
		}
		var currentInstallation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentInstallation, installation.ID).Error; err != nil {
			return err
		}
		if !installationWantsOperation(currentInstallation, operation) {
			return tx.Model(&currentOperation).Updates(map[string]any{
				"state": "superseded", "observed_at": now,
				"last_error": "installation desired state changed during failed execution", "claimed_by": "", "lease_expires_at": nil,
			}).Error
		}
		if err := tx.Model(&currentOperation).Updates(map[string]any{
			"state": "failed", "observed_at": now, "last_error": message,
			"claimed_by": "", "lease_expires_at": nil,
		}).Error; err != nil {
			return err
		}
		currentInstallation.LastError = message
		if rollbackSucceeded {
			currentInstallation.PreviousVersion = operation.TargetVersion
			currentInstallation.DesiredVersion = currentInstallation.ObservedVersion
			currentInstallation.State = "healthy"
		} else {
			currentInstallation.State = "failed"
		}
		return tx.Save(&currentInstallation).Error
	})
}

func (w *OperationWorker) recordSuperseded(operation model.KernelOperation, message string) error {
	now := w.now()
	return w.db.Model(&model.KernelOperation{}).
		Where("id = ? AND state = ? AND claimed_by = ?", operation.ID, "running", w.workerID).
		Updates(map[string]any{
			"state": "superseded", "observed_at": now, "last_error": message,
			"claimed_by": "", "lease_expires_at": nil,
		}).Error
}

func installationWantsOperation(installation model.PluginInstallation, operation model.KernelOperation) bool {
	switch operation.Kind {
	case "plugin.enable", "plugin.update", "plugin.rollback":
		return installation.Enabled && installation.DesiredVersion == operation.TargetVersion
	case "plugin.disable":
		return !installation.Enabled && installation.DesiredVersion == operation.TargetVersion
	case "plugin.install", "plugin.configure", "plugin.health":
		return installation.DesiredVersion == operation.TargetVersion
	case "plugin.inspect":
		return true
	default:
		return false
	}
}
