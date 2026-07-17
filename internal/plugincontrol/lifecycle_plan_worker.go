package plugincontrol

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// reconcileLifecyclePlans makes cancellation, deadline expiry, and process
// restart recovery converge through the same durable reverse-rollback path.
// It is deliberately called before every worker sweep, so no in-memory plan
// coordinator is required for correctness.
func (w *OperationWorker) reconcileLifecyclePlans() error {
	var plans []model.PluginLifecyclePlan
	if err := w.db.Where("target = ? AND state IN ?", "control", []string{
		lifecyclePlanPending, lifecyclePlanRunning, lifecyclePlanCancelRequested, lifecyclePlanRollingBack,
	}).Order("created_at, id").Find(&plans).Error; err != nil {
		return err
	}
	for _, candidate := range plans {
		if err := w.db.Transaction(func(tx *gorm.DB) error {
			var plan model.PluginLifecyclePlan
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", candidate.ID).Error; err != nil {
				return err
			}
			return w.reconcileLifecyclePlanTx(tx, &plan, w.now())
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *OperationWorker) reconcileLifecyclePlanTx(tx *gorm.DB, plan *model.PluginLifecyclePlan, now time.Time) error {
	if plan == nil {
		return errors.New("lifecycle plan is required")
	}
	switch plan.State {
	case lifecyclePlanCancelRequested:
		active, err := lifecyclePlanHasActiveApplyOperations(tx, plan.ID)
		if err != nil || active {
			return err
		}
		return w.beginLifecyclePlanRollbackTx(tx, plan, lifecyclePlanCancelled, plan.LastError, now)
	case lifecyclePlanPending, lifecyclePlanRunning:
		var failed []model.KernelOperation
		if err := tx.Where("lifecycle_plan_id = ? AND lifecycle_plan_phase = ? AND state IN ?", plan.ID, lifecyclePlanPhaseApply,
			[]string{"failed", "timed_out", "cancelled", "superseded"}).Order("lifecycle_plan_sequence, created_at, id").Find(&failed).Error; err != nil {
			return err
		}
		if len(failed) == 0 {
			return nil
		}
		for _, operation := range failed {
			if operation.State == "superseded" {
				return supersedeLifecyclePlanTx(tx, plan, "installation desired state changed during dependency lifecycle plan", now)
			}
		}
		message := strings.TrimSpace(failed[0].LastError)
		if message == "" {
			message = "dependency lifecycle plan apply operation did not complete"
		}
		outcome := lifecyclePlanFailed
		for _, operation := range failed {
			if operation.State == "cancelled" {
				outcome = lifecyclePlanCancelled
				break
			}
		}
		return w.beginLifecyclePlanRollbackTx(tx, plan, outcome, message, now)
	case lifecyclePlanRollingBack:
		return w.finishLifecyclePlanRollbackTx(tx, plan, now)
	default:
		return nil
	}
}

func lifecyclePlanHasActiveApplyOperations(tx *gorm.DB, planID string) (bool, error) {
	var count int64
	err := tx.Model(&model.KernelOperation{}).Where(
		"lifecycle_plan_id = ? AND lifecycle_plan_phase = ? AND state IN ?", planID, lifecyclePlanPhaseApply,
		[]string{"pending", "dispatching", "running", "cancel_requested"},
	).Count(&count).Error
	return count > 0, err
}

func (w *OperationWorker) beginLifecyclePlanRollbackTx(tx *gorm.DB, plan *model.PluginLifecyclePlan, outcome, message string, now time.Time) error {
	if plan == nil {
		return errors.New("lifecycle plan is required")
	}
	if plan.State == lifecyclePlanSuperseded || plan.State == lifecyclePlanSucceeded || plan.State == lifecyclePlanFailed || plan.State == lifecyclePlanCancelled {
		return nil
	}
	// No later apply operation may run after the first failed vertex. They are
	// made terminal before rollback gating begins, which also survives a worker
	// restart between the failure and reverse actions.
	if err := tx.Model(&model.KernelOperation{}).Where(
		"lifecycle_plan_id = ? AND lifecycle_plan_phase = ? AND state = ?", plan.ID, lifecyclePlanPhaseApply, "pending",
	).Updates(map[string]any{
		"state": "cancelled", "cancel_at": now, "last_error": "dependency lifecycle plan stopped before this apply operation",
	}).Error; err != nil {
		return err
	}
	plan.State = lifecyclePlanRollingBack
	plan.Outcome = outcome
	plan.LastError = strings.TrimSpace(message)
	if err := tx.Save(plan).Error; err != nil {
		return err
	}

	var steps []model.PluginLifecyclePlanStep
	if err := tx.Where("plan_id = ?", plan.ID).Order("sequence DESC, id DESC").Find(&steps).Error; err != nil {
		return err
	}
	created := 0
	for _, step := range steps {
		if !step.Changed {
			if step.RollbackState != lifecyclePlanStepSkipped {
				step.RollbackState = lifecyclePlanStepSkipped
				if err := tx.Save(&step).Error; err != nil {
					return err
				}
			}
			continue
		}
		var applyOperations []model.KernelOperation
		if err := tx.Where("lifecycle_plan_id = ? AND lifecycle_plan_step_id = ? AND lifecycle_plan_phase = ?", plan.ID, step.ID, lifecyclePlanPhaseApply).
			Order("lifecycle_plan_sequence").Find(&applyOperations).Error; err != nil {
			return err
		}
		attempted := false
		for _, operation := range applyOperations {
			if operation.State == "succeeded" || (operation.Attempt > 0 && (operation.State == "failed" || operation.State == "timed_out" || operation.State == "cancelled")) {
				attempted = true
				break
			}
		}
		if !attempted {
			if err := restoreLifecyclePlanStep(tx, step, now); err != nil {
				return err
			}
			step.RollbackState = lifecyclePlanStepSkipped
			if err := tx.Save(&step).Error; err != nil {
				return err
			}
			continue
		}

		var existing int64
		if err := tx.Model(&model.KernelOperation{}).Where("lifecycle_plan_id = ? AND lifecycle_plan_step_id = ? AND lifecycle_plan_phase = ?", plan.ID, step.ID, lifecyclePlanPhaseRollback).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			continue
		}
		var installation model.PluginInstallation
		if err := tx.Where("plugin_id = ? AND target = ?", step.PluginID, "control").First(&installation).Error; err != nil {
			return err
		}
		configuration, err := service.GetPluginConfiguration(tx, installation.ID)
		if err != nil {
			return err
		}
		kind, version := lifecyclePlanRollbackAction(step)
		deadline := now.Add(5 * time.Minute)
		sequence := 1_000_000 + len(steps) - step.Sequence
		if _, err := createLifecyclePlanOperation(tx, *plan, step, lifecyclePlanPhaseRollback, sequence, installation, kind, version, configuration.ConfigJSON, deadline); err != nil {
			return err
		}
		created++
	}
	if created == 0 {
		return w.finishLifecyclePlanRollbackTx(tx, plan, now)
	}
	return nil
}

func lifecyclePlanRollbackAction(step model.PluginLifecyclePlanStep) (string, string) {
	if !step.HadInstallation || !step.PreviousEnabled {
		return "plugin.disable", step.TargetVersion
	}
	version := step.PreviousObservedVersion
	if version == "" {
		version = step.PreviousDesiredVersion
	}
	if version == "" || version == step.TargetVersion {
		return "plugin.enable", step.TargetVersion
	}
	return "plugin.rollback", version
}

func (w *OperationWorker) finishLifecyclePlanRollbackTx(tx *gorm.DB, plan *model.PluginLifecyclePlan, now time.Time) error {
	var operations []model.KernelOperation
	if err := tx.Where("lifecycle_plan_id = ? AND lifecycle_plan_phase = ?", plan.ID, lifecyclePlanPhaseRollback).Find(&operations).Error; err != nil {
		return err
	}
	for _, operation := range operations {
		switch operation.State {
		case "pending", "dispatching", "running", "cancel_requested":
			return nil
		case "failed", "timed_out", "cancelled", "superseded":
			plan.State = lifecyclePlanFailed
			plan.Outcome = lifecyclePlanFailed
			if strings.TrimSpace(operation.LastError) != "" {
				plan.LastError = operation.LastError
			}
			plan.CompletedAt = &now
			return tx.Save(plan).Error
		}
	}
	plan.State = plan.Outcome
	if plan.State == "" {
		plan.State = lifecyclePlanFailed
	}
	plan.CompletedAt = &now
	return tx.Save(plan).Error
}

func supersedeLifecyclePlanTx(tx *gorm.DB, plan *model.PluginLifecyclePlan, message string, now time.Time) error {
	if plan == nil {
		return errors.New("lifecycle plan is required")
	}
	if err := tx.Model(&model.KernelOperation{}).Where("lifecycle_plan_id = ? AND state = ?", plan.ID, "pending").
		Updates(map[string]any{"state": "superseded", "observed_at": now, "last_error": message}).Error; err != nil {
		return err
	}
	plan.State, plan.Outcome, plan.LastError, plan.CompletedAt = lifecyclePlanSuperseded, lifecyclePlanSuperseded, message, &now
	return tx.Save(plan).Error
}

func lifecyclePlanAllowsClaim(tx *gorm.DB, operation model.KernelOperation, now time.Time) (bool, error) {
	if strings.TrimSpace(operation.LifecyclePlanID) == "" {
		return true, nil
	}
	var plan model.PluginLifecyclePlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", operation.LifecyclePlanID).Error; err != nil {
		return false, err
	}
	switch operation.LifecyclePlanPhase {
	case lifecyclePlanPhaseApply:
		if plan.State != lifecyclePlanPending && plan.State != lifecyclePlanRunning {
			return false, nil
		}
		var previous int64
		if err := tx.Model(&model.KernelOperation{}).Where(
			"lifecycle_plan_id = ? AND lifecycle_plan_phase = ? AND lifecycle_plan_sequence < ? AND state <> ?",
			plan.ID, lifecyclePlanPhaseApply, operation.LifecyclePlanSequence, "succeeded",
		).Count(&previous).Error; err != nil {
			return false, err
		}
		if previous > 0 {
			return false, nil
		}
		if plan.State == lifecyclePlanPending {
			if err := tx.Model(&plan).Update("state", lifecyclePlanRunning).Error; err != nil {
				return false, err
			}
		}
		return true, nil
	case lifecyclePlanPhaseRollback:
		if plan.State != lifecyclePlanRollingBack {
			return false, nil
		}
		active, err := lifecyclePlanHasActiveApplyOperations(tx, plan.ID)
		if err != nil || active {
			return false, err
		}
		var step model.PluginLifecyclePlanStep
		if err := tx.First(&step, operation.LifecyclePlanStepID).Error; err != nil {
			return false, err
		}
		var earlier int64
		if err := tx.Model(&model.PluginLifecyclePlanStep{}).Where(
			"plan_id = ? AND sequence > ? AND rollback_state NOT IN ?", plan.ID, step.Sequence,
			[]string{lifecyclePlanStepSucceeded, lifecyclePlanStepSkipped},
		).Count(&earlier).Error; err != nil {
			return false, err
		}
		return earlier == 0, nil
	default:
		return false, fmt.Errorf("unsupported lifecycle plan phase %q", operation.LifecyclePlanPhase)
	}
}

func (w *OperationWorker) recordLifecyclePlanSuccess(tx *gorm.DB, operation model.KernelOperation, now time.Time) error {
	if strings.TrimSpace(operation.LifecyclePlanID) == "" {
		return nil
	}
	var plan model.PluginLifecyclePlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", operation.LifecyclePlanID).Error; err != nil {
		return err
	}
	var step model.PluginLifecyclePlanStep
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&step, operation.LifecyclePlanStepID).Error; err != nil {
		return err
	}
	switch operation.LifecyclePlanPhase {
	case lifecyclePlanPhaseApply:
		var incomplete int64
		if err := tx.Model(&model.KernelOperation{}).Where(
			"lifecycle_plan_id = ? AND lifecycle_plan_step_id = ? AND lifecycle_plan_phase = ? AND state <> ?",
			plan.ID, step.ID, lifecyclePlanPhaseApply, "succeeded",
		).Count(&incomplete).Error; err != nil {
			return err
		}
		if incomplete == 0 {
			step.ApplyState = lifecyclePlanStepSucceeded
			if err := tx.Save(&step).Error; err != nil {
				return err
			}
		}
		var pending int64
		if err := tx.Model(&model.KernelOperation{}).Where(
			"lifecycle_plan_id = ? AND lifecycle_plan_phase = ? AND state <> ?", plan.ID, lifecyclePlanPhaseApply, "succeeded",
		).Count(&pending).Error; err != nil {
			return err
		}
		if pending == 0 {
			plan.State, plan.Outcome, plan.CompletedAt, plan.LastError = lifecyclePlanSucceeded, lifecyclePlanSucceeded, &now, ""
			return tx.Save(&plan).Error
		}
	case lifecyclePlanPhaseRollback:
		if err := restoreLifecyclePlanStep(tx, step, now); err != nil {
			return err
		}
		step.RollbackState = lifecyclePlanStepSucceeded
		if err := tx.Save(&step).Error; err != nil {
			return err
		}
		return w.finishLifecyclePlanRollbackTx(tx, &plan, now)
	}
	return nil
}

func (w *OperationWorker) recordLifecyclePlanFailure(tx *gorm.DB, operation model.KernelOperation, installation *model.PluginInstallation, operationErr error, now time.Time) error {
	message := strings.TrimSpace(operationErr.Error())
	if message == "" {
		message = "dependency lifecycle operation failed"
	}
	var currentOperation model.KernelOperation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentOperation, "id = ?", operation.ID).Error; err != nil {
		return err
	}
	var plan model.PluginLifecyclePlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", operation.LifecyclePlanID).Error; err != nil {
		return err
	}
	var step model.PluginLifecyclePlanStep
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&step, operation.LifecyclePlanStepID).Error; err != nil {
		return err
	}
	if operation.LifecyclePlanPhase == lifecyclePlanPhaseApply && installation != nil && currentOperation.State != "cancel_requested" {
		var currentInstallation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentInstallation, installation.ID).Error; err != nil {
			return err
		}
		if !installationWantsOperation(currentInstallation, operation) {
			if err := tx.Model(&currentOperation).Where("state = ? AND claimed_by = ?", "running", w.workerID).Updates(map[string]any{
				"state": "superseded", "observed_at": now, "last_error": "installation desired state changed during failed execution",
				"claimed_by": "", "lease_expires_at": nil,
			}).Error; err != nil {
				return err
			}
			return supersedeLifecyclePlanTx(tx, &plan, "installation desired state changed during dependency lifecycle plan", now)
		}
	}

	if operation.LifecyclePlanPhase == lifecyclePlanPhaseRollback {
		if currentOperation.State == "cancel_requested" {
			currentOperation.State = "cancelled"
		} else if currentOperation.State == "running" && currentOperation.ClaimedBy == w.workerID {
			currentOperation.State = "failed"
		} else {
			return nil
		}
		currentOperation.ObservedAt, currentOperation.LastError, currentOperation.ClaimedBy, currentOperation.LeaseExpiresAt = &now, message, "", nil
		if err := tx.Save(&currentOperation).Error; err != nil {
			return err
		}
		step.RollbackState = lifecyclePlanStepFailed
		if err := tx.Save(&step).Error; err != nil {
			return err
		}
		plan.State, plan.Outcome, plan.LastError, plan.CompletedAt = lifecyclePlanFailed, lifecyclePlanFailed, message, &now
		if installation != nil {
			var currentInstallation model.PluginInstallation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentInstallation, installation.ID).Error; err != nil {
				return err
			}
			currentInstallation.State, currentInstallation.LastError = "failed", message
			if err := tx.Save(&currentInstallation).Error; err != nil {
				return err
			}
		}
		return tx.Save(&plan).Error
	}

	outcome := lifecyclePlanFailed
	if currentOperation.State == "cancel_requested" || plan.State == lifecyclePlanCancelRequested {
		currentOperation.State = "cancelled"
		outcome = lifecyclePlanCancelled
	} else if currentOperation.State == "running" && currentOperation.ClaimedBy == w.workerID {
		currentOperation.State = "failed"
	} else {
		return nil
	}
	currentOperation.ObservedAt, currentOperation.LastError, currentOperation.ClaimedBy, currentOperation.LeaseExpiresAt = &now, message, "", nil
	if err := tx.Save(&currentOperation).Error; err != nil {
		return err
	}
	step.ApplyState = lifecyclePlanStepFailed
	if err := tx.Save(&step).Error; err != nil {
		return err
	}
	if installation != nil {
		var currentInstallation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentInstallation, installation.ID).Error; err != nil {
			return err
		}
		currentInstallation.State, currentInstallation.LastError = "failed", message
		if err := tx.Save(&currentInstallation).Error; err != nil {
			return err
		}
	}
	return w.beginLifecyclePlanRollbackTx(tx, &plan, outcome, message, now)
}

func (w *OperationWorker) recordLifecyclePlanSuperseded(tx *gorm.DB, operation model.KernelOperation, now time.Time, message string) error {
	var plan model.PluginLifecyclePlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&plan, "id = ?", operation.LifecyclePlanID).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.KernelOperation{}).Where("id = ? AND state = ? AND claimed_by = ?", operation.ID, "running", w.workerID).
		Updates(map[string]any{"state": "superseded", "observed_at": now, "last_error": message, "claimed_by": "", "lease_expires_at": nil}).Error; err != nil {
		return err
	}
	return supersedeLifecyclePlanTx(tx, &plan, message, now)
}
