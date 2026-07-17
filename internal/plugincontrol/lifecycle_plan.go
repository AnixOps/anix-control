package plugincontrol

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	lifecyclePlanPending         = "pending"
	lifecyclePlanRunning         = "running"
	lifecyclePlanCancelRequested = "cancel_requested"
	lifecyclePlanRollingBack     = "rolling_back"
	lifecyclePlanSucceeded       = "succeeded"
	lifecyclePlanFailed          = "failed"
	lifecyclePlanCancelled       = "cancelled"
	lifecyclePlanSuperseded      = "superseded"
	lifecyclePlanPhaseApply      = "apply"
	lifecyclePlanPhaseRollback   = "rollback"
	lifecyclePlanStepPending     = "pending"
	lifecyclePlanStepSucceeded   = "succeeded"
	lifecyclePlanStepFailed      = "failed"
	lifecyclePlanStepSkipped     = "skipped"
)

// DependencyLifecyclePlanRequest describes one Control-target root action.
// RootInstallation has already been persisted with the requested intent. The
// pre-mutation copy is retained so a failed closure can restore exact desired
// state without purging package data.
type DependencyLifecyclePlanRequest struct {
	RootInstallation         model.PluginInstallation
	PreviousRootInstallation *model.PluginInstallation
	RootRelease              model.PluginRelease
	RootKind                 string
	IdempotencyKey           string
	DeadlineAt               time.Time
}

// QueueDependencyLifecyclePlan converts a recursive dependency closure into
// durable Control operations. It is intentionally called only while holding
// the target lock in the HTTP handler. The returned operation is the root's
// final lifecycle action, so existing operation APIs continue to represent the
// user-visible request while the plan retains its dependent work.
func QueueDependencyLifecyclePlan(tx *gorm.DB, request DependencyLifecyclePlanRequest) (*model.KernelOperation, error) {
	if tx == nil {
		return nil, errors.New("database is not initialized")
	}
	if request.RootInstallation.ID == 0 || request.RootInstallation.Target != "control" {
		return nil, errors.New("control dependency lifecycle plan requires a persisted Control installation")
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 160 {
		return nil, errors.New("dependency lifecycle plan idempotency key is required")
	}
	if request.DeadlineAt.IsZero() || !request.DeadlineAt.After(time.Now()) {
		return nil, errors.New("dependency lifecycle plan deadline must be in the future")
	}
	switch request.RootKind {
	case "plugin.enable", "plugin.update", "plugin.rollback":
	default:
		return nil, fmt.Errorf("unsupported dependency lifecycle root action %q", request.RootKind)
	}

	var existingPlan model.PluginLifecyclePlan
	if err := tx.Where("idempotency_key = ?", request.IdempotencyKey).First(&existingPlan).Error; err == nil {
		var existingOperation model.KernelOperation
		if err := tx.First(&existingOperation, "id = ?", existingPlan.RootOperationID).Error; err != nil {
			return nil, fmt.Errorf("load idempotent lifecycle root operation: %w", err)
		}
		return &existingOperation, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	planDefinition, err := service.ResolvePluginDependencyExecutionPlan(tx, request.RootRelease, "control")
	if err != nil {
		return nil, err
	}
	if err := service.ValidatePluginDependencyExecutionPlan(tx, planDefinition); err != nil {
		return nil, err
	}

	plan := model.PluginLifecyclePlan{
		ID: uuid.NewString(), IdempotencyKey: request.IdempotencyKey, Target: "control",
		RootPluginID: request.RootInstallation.PluginID, RootOperationID: uuid.NewString(),
		State: lifecyclePlanPending,
	}
	if err := tx.Create(&plan).Error; err != nil {
		return nil, err
	}

	operationSequence := 0
	var rootOperation *model.KernelOperation
	for graphSequence, selected := range planDefinition.Steps {
		current, previous, changed, err := preparePlanInstallation(tx, selected, request)
		if err != nil {
			return nil, err
		}
		step := snapshotLifecyclePlanStep(plan.ID, graphSequence, selected.PluginID, selected.Release.Version, previous, changed)
		if err := tx.Create(&step).Error; err != nil {
			return nil, err
		}

		actions := lifecyclePlanApplyActions(selected.PluginID == request.RootInstallation.PluginID, request.RootKind, previous, current, selected.Release.Version, changed)
		if len(actions) == 0 {
			step.ApplyState = lifecyclePlanStepSkipped
			step.RollbackState = lifecyclePlanStepSkipped
			if err := tx.Save(&step).Error; err != nil {
				return nil, err
			}
			continue
		}
		configuration, err := service.GetPluginConfiguration(tx, current.ID)
		if err != nil {
			return nil, fmt.Errorf("load lifecycle configuration for %s: %w", current.PluginID, err)
		}
		for _, action := range actions {
			operationSequence++
			operation, err := createLifecyclePlanOperation(tx, plan, step, lifecyclePlanPhaseApply, operationSequence, current, action, selected.Release.Version, configuration.ConfigJSON, request.DeadlineAt)
			if err != nil {
				return nil, err
			}
			if selected.PluginID == request.RootInstallation.PluginID {
				rootOperation = operation
			}
		}
	}
	if rootOperation == nil {
		return nil, errors.New("dependency lifecycle plan did not create a root operation")
	}
	if err := tx.Model(&plan).Update("root_operation_id", rootOperation.ID).Error; err != nil {
		return nil, err
	}
	return rootOperation, nil
}

func preparePlanInstallation(tx *gorm.DB, selected service.PluginDependencyExecutionStep, request DependencyLifecyclePlanRequest) (model.PluginInstallation, *model.PluginInstallation, bool, error) {
	if selected.PluginID == request.RootInstallation.PluginID {
		current := request.RootInstallation
		if current.DesiredVersion != selected.Release.Version || !current.Enabled {
			return model.PluginInstallation{}, nil, false, errors.New("root installation no longer matches its dependency execution plan")
		}
		return current, cloneInstallation(request.PreviousRootInstallation), true, nil
	}

	var current model.PluginInstallation
	previous := cloneInstallation(selected.Installation)
	if previous == nil {
		current = model.PluginInstallation{
			PluginID: selected.PluginID, Target: "control", DesiredVersion: selected.Release.Version,
			State: "pending", Enabled: true, LifecycleGeneration: 1,
		}
		if err := tx.Create(&current).Error; err != nil {
			return model.PluginInstallation{}, nil, false, err
		}
		return current, nil, true, nil
	}
	current = *previous
	changed := !current.Enabled || current.DesiredVersion != selected.Release.Version || current.ObservedVersion != selected.Release.Version || current.State == "disabled"
	if !changed {
		return current, previous, false, nil
	}
	oldDesired, oldObserved := current.DesiredVersion, current.ObservedVersion
	current.DesiredVersion, current.Enabled, current.State, current.DisabledAt = selected.Release.Version, true, "pending", nil
	current.LifecycleGeneration++
	if oldDesired != selected.Release.Version {
		prior := oldObserved
		if prior == "" {
			prior = oldDesired
		}
		if prior != "" && prior != selected.Release.Version {
			current.PreviousVersion = prior
		}
	}
	if err := tx.Save(&current).Error; err != nil {
		return model.PluginInstallation{}, nil, false, err
	}
	return current, previous, true, nil
}

func lifecyclePlanApplyActions(isRoot bool, rootKind string, previous *model.PluginInstallation, current model.PluginInstallation, targetVersion string, changed bool) []string {
	if !changed {
		return nil
	}
	if isRoot {
		switch rootKind {
		case "plugin.update", "plugin.rollback":
			if previous != nil && previous.ObservedVersion == "" {
				return []string{"plugin.install", "plugin.enable"}
			}
			return []string{rootKind}
		case "plugin.enable":
			// Continue below through the generic enable/install state machine.
		}
	}
	if previous == nil {
		return []string{"plugin.install", "plugin.enable"}
	}
	if !previous.Enabled {
		switch previous.ObservedVersion {
		case targetVersion:
			return []string{"plugin.enable"}
		case "":
			return []string{"plugin.install", "plugin.enable"}
		default:
			return []string{"plugin.update", "plugin.enable"}
		}
	}
	if previous.ObservedVersion != targetVersion {
		return []string{"plugin.update"}
	}
	if current.State != "healthy" {
		return []string{"plugin.enable"}
	}
	return nil
}

func snapshotLifecyclePlanStep(planID string, sequence int, pluginID, targetVersion string, previous *model.PluginInstallation, changed bool) model.PluginLifecyclePlanStep {
	step := model.PluginLifecyclePlanStep{
		PlanID: planID, Sequence: sequence, PluginID: pluginID, TargetVersion: targetVersion,
		Changed: changed, ApplyState: lifecyclePlanStepPending, RollbackState: lifecyclePlanStepPending,
	}
	if previous == nil {
		return step
	}
	step.HadInstallation = true
	step.PreviousDesiredVersion = previous.DesiredVersion
	step.PreviousObservedVersion = previous.ObservedVersion
	step.PreviousPreviousVersion = previous.PreviousVersion
	step.PreviousState = previous.State
	step.PreviousEnabled = previous.Enabled
	step.PreviousDisabledAt = previous.DisabledAt
	step.PreviousLastError = previous.LastError
	return step
}

func cloneInstallation(source *model.PluginInstallation) *model.PluginInstallation {
	if source == nil {
		return nil
	}
	copy := *source
	if source.DisabledAt != nil {
		value := *source.DisabledAt
		copy.DisabledAt = &value
	}
	return &copy
}

func createLifecyclePlanOperation(tx *gorm.DB, plan model.PluginLifecyclePlan, step model.PluginLifecyclePlanStep, phase string, sequence int, installation model.PluginInstallation, kind, targetVersion, configuration string, deadline time.Time) (*model.KernelOperation, error) {
	key := fmt.Sprintf("control-plan:%s:%s:%d", plan.ID, phase, sequence)
	operation, _, err := service.CreateKernelOperation(tx, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: key, PluginID: installation.PluginID, TargetVersion: targetVersion,
		Kind: kind, ConfigJSON: configuration, DeadlineAt: &deadline,
		LifecyclePlanID: plan.ID, LifecyclePlanStepID: step.ID,
		LifecyclePlanPhase: phase, LifecyclePlanSequence: sequence,
	})
	return operation, err
}

func restoreLifecyclePlanStep(tx *gorm.DB, step model.PluginLifecyclePlanStep, now time.Time) error {
	var installation model.PluginInstallation
	if err := tx.Where("plugin_id = ? AND target = ?", step.PluginID, "control").First(&installation).Error; err != nil {
		return err
	}
	if !step.HadInstallation {
		installation.Enabled = false
		installation.State = "disabled"
		installation.ObservedVersion = ""
		installation.PreviousVersion = ""
		installation.DisabledAt = &now
		return tx.Save(&installation).Error
	}
	installation.DesiredVersion = step.PreviousDesiredVersion
	installation.ObservedVersion = step.PreviousObservedVersion
	installation.PreviousVersion = step.PreviousPreviousVersion
	installation.State = step.PreviousState
	installation.Enabled = step.PreviousEnabled
	installation.DisabledAt = step.PreviousDisabledAt
	installation.LastError = step.PreviousLastError
	return tx.Save(&installation).Error
}
