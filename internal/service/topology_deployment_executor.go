package service

import (
	"context"
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

const (
	topologyDeploymentStatePlanned           = "planned"
	topologyDeploymentStateApplying          = "applying"
	topologyDeploymentStateRollbackRequested = "rollback_requested"
	topologyDeploymentStateSucceeded         = "succeeded"
	topologyDeploymentStateFailed            = "failed"
	topologyDeploymentStateRolledBack        = "rolled_back"

	topologyStepStatePlanned             = "planned"
	topologyStepStateConfiguring         = "configuring"
	topologyStepStateEnabling            = "enabling"
	topologyStepStateDisabling           = "disabling"
	topologyStepStateSucceeded           = "succeeded"
	topologyStepStateFailed              = "failed"
	topologyStepStateSkipped             = "skipped"
	topologyStepStateRollbackConfiguring = "rollback_configuring"
	topologyStepStateRollbackEnabling    = "rollback_enabling"
	topologyStepStateRollbackDisabling   = "rollback_disabling"
	topologyStepStateRolledBack          = "rolled_back"
	topologyStepStateRollbackFailed      = "rollback_failed"

	topologyApplyActionConfigureEnable = "configure_enable"
	topologyApplyActionDisable         = "disable"
	topologyRollbackModeRestore        = "restore"
	topologyRollbackModeDisable        = "disable"
)

var (
	ErrTopologyDeploymentNotPlanned = errors.New("topology deployment is not planned")
	ErrTopologyDeploymentTerminal   = errors.New("topology deployment is already terminal")
	ErrTopologyDeploymentBusy       = errors.New("topology has an active deployment")
)

// TopologyDeploymentPlanInput identifies an immutable topology revision to
// compile into a durable node-operation plan. The plan is intentionally inert:
// callers must request apply and the separately feature-gated executor must be
// running before any Agent operation is created.
type TopologyDeploymentPlanInput struct {
	TopologyID    uint
	RevisionID    uint
	ActorID       uint
	RolloutGroup  string
	FailurePolicy string
}

// TopologyDeploymentStatus is the transport-neutral status read used by the
// HTTP handler and tests. All fields are persisted kernel records.
type TopologyDeploymentStatus struct {
	Deployment model.TopologyDeployment       `json:"deployment"`
	Steps      []model.TopologyDeploymentStep `json:"steps"`
	Observed   []model.TopologyObservedState  `json:"observed_states"`
}

// TopologyDeploymentExecutor turns a previously planned revision into durable
// Agent operations. It never opens a network/data-plane connection itself; the
// existing KernelOperationBridge remains the sole Agent transport.
//
// A failed operation fences further expansion immediately, cancels in-flight
// sibling work, then compensates successful/started steps in reverse DAG order.
// The compensation restores the previous active revision where possible, or
// disables a newly introduced plugin vertex when no predecessor exists.
type TopologyDeploymentExecutor struct {
	db               *gorm.DB
	now              func() time.Time
	operationTimeout time.Duration
}

func NewTopologyDeploymentExecutor(db *gorm.DB) (*TopologyDeploymentExecutor, error) {
	if db == nil {
		return nil, errors.New("topology deployment executor requires a database")
	}
	return &TopologyDeploymentExecutor{
		db: db, now: time.Now, operationTimeout: 5 * time.Minute,
	}, nil
}

// Start runs reconciliation until ctx is cancelled. It is deliberately
// separate from plan/apply HTTP paths so topology execution remains opt-in.
func (e *TopologyDeploymentExecutor) Start(ctx context.Context, interval time.Duration, reportError func(error)) {
	if e == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		run := func() {
			if _, err := e.RunOnce(ctx); err != nil && reportError != nil && !errors.Is(err, context.Canceled) {
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

// RunOnce reconciles every active topology deployment. Operations are created
// in database transactions and carry stable idempotency keys, so repeating a
// pass after a Control restart is safe.
func (e *TopologyDeploymentExecutor) RunOnce(ctx context.Context) (int, error) {
	if e == nil || e.db == nil {
		return 0, errors.New("topology deployment executor is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := ExpireKernelOperations(e.db, e.now()); err != nil {
		return 0, err
	}
	var ids []uint
	if err := e.db.Model(&model.TopologyDeployment{}).
		Where("state IN ?", []string{topologyDeploymentStateApplying, topologyDeploymentStateRollbackRequested}).
		Order("id").Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	changed := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return changed, err
		}
		updated, err := e.reconcileDeployment(ctx, id)
		if err != nil {
			return changed, err
		}
		if updated {
			changed++
		}
	}
	return changed, nil
}

// PlanTopologyDeployment compiles the immutable graph into deployment steps.
// It does not mutate an active revision or create any Agent work.
func PlanTopologyDeployment(db *gorm.DB, input TopologyDeploymentPlanInput) (*model.TopologyDeployment, []model.TopologyDeploymentStep, error) {
	if db == nil {
		return nil, nil, errors.New("database is not initialized")
	}
	if input.TopologyID == 0 || input.RevisionID == 0 {
		return nil, nil, errors.New("topology_id and revision_id are required")
	}
	input.RolloutGroup = strings.TrimSpace(input.RolloutGroup)
	input.FailurePolicy = strings.TrimSpace(input.FailurePolicy)
	if input.FailurePolicy == "" {
		input.FailurePolicy = "stop_and_rollback"
	}
	if input.FailurePolicy != "stop_and_rollback" {
		return nil, nil, errors.New("topology deployment failure_policy must be stop_and_rollback")
	}

	var result model.TopologyDeployment
	var resultSteps []model.TopologyDeploymentStep
	err := db.Transaction(func(tx *gorm.DB) error {
		var topology model.Topology
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&topology, input.TopologyID).Error; err != nil {
			return err
		}
		var revision model.TopologyRevision
		if err := tx.First(&revision, "id = ? AND topology_id = ?", input.RevisionID, input.TopologyID).Error; err != nil {
			return errors.New("topology revision does not belong to topology")
		}
		var activeCount int64
		if err := tx.Model(&model.TopologyDeployment{}).
			Where("topology_id = ? AND state IN ?", topology.ID, []string{topologyDeploymentStateApplying, topologyDeploymentStateRollbackRequested}).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount > 0 {
			return ErrTopologyDeploymentBusy
		}

		var vertices []model.TopologyVertex
		var edges []model.TopologyEdge
		if err := tx.Where("revision_id = ?", revision.ID).Order("key").Find(&vertices).Error; err != nil {
			return err
		}
		if err := tx.Where("revision_id = ?", revision.ID).Order("id").Find(&edges).Error; err != nil {
			return err
		}
		steps, err := compileTopologyDeploymentSteps(tx, topology, revision, vertices, edges, input.RolloutGroup)
		if err != nil {
			return err
		}
		result = model.TopologyDeployment{
			TopologyID: topology.ID, RevisionID: revision.ID, PreviousRevisionID: topology.ActiveRevisionID,
			RolloutGroup: input.RolloutGroup, State: topologyDeploymentStatePlanned,
			FailurePolicy: input.FailurePolicy, CreatedBy: input.ActorID,
		}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		for i := range steps {
			steps[i].DeploymentID = result.ID
		}
		if err := tx.Create(&steps).Error; err != nil {
			return err
		}
		resultSteps = append([]model.TopologyDeploymentStep(nil), steps...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &result, resultSteps, nil
}

// RequestTopologyDeploymentApply makes a plan eligible for the feature-gated
// worker. It is idempotent while already applying but never reopens terminal
// deployments, preserving an audit trail for every revision attempt.
func RequestTopologyDeploymentApply(db *gorm.DB, deploymentID uint, _ time.Time) (*model.TopologyDeployment, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if deploymentID == 0 {
		return nil, errors.New("deployment_id is required")
	}
	var deployment model.TopologyDeployment
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deployment, deploymentID).Error; err != nil {
			return err
		}
		switch deployment.State {
		case topologyDeploymentStatePlanned:
			if err := tx.Model(&deployment).Updates(map[string]any{
				"state": topologyDeploymentStateApplying, "completed_at": nil, "last_error": "",
				"rollback_started_at": nil, "rollback_completed_at": nil,
			}).Error; err != nil {
				return err
			}
			deployment.State, deployment.CompletedAt, deployment.LastError = topologyDeploymentStateApplying, nil, ""
		case topologyDeploymentStateApplying:
			return nil
		case topologyDeploymentStateRollbackRequested:
			return ErrTopologyDeploymentNotPlanned
		default:
			return ErrTopologyDeploymentTerminal
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// RequestTopologyDeploymentRollback stops expansion and schedules durable
// reverse-order compensation. It is safe to call before any operation has
// dispatched; such a deployment simply reaches rolled_back with skipped steps.
func RequestTopologyDeploymentRollback(db *gorm.DB, deploymentID uint, at time.Time) (*model.TopologyDeployment, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if deploymentID == 0 {
		return nil, errors.New("deployment_id is required")
	}
	if at.IsZero() {
		at = time.Now()
	}
	var deployment model.TopologyDeployment
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deployment, deploymentID).Error; err != nil {
			return err
		}
		switch deployment.State {
		case topologyDeploymentStatePlanned, topologyDeploymentStateApplying, topologyDeploymentStateSucceeded, topologyDeploymentStateFailed:
			if err := tx.Model(&deployment).Updates(map[string]any{
				"state": topologyDeploymentStateRollbackRequested, "completed_at": nil,
				"rollback_started_at": at, "rollback_completed_at": nil,
			}).Error; err != nil {
				return err
			}
			deployment.State, deployment.CompletedAt, deployment.RollbackStartedAt = topologyDeploymentStateRollbackRequested, nil, &at
		case topologyDeploymentStateRollbackRequested:
			return nil
		default:
			return ErrTopologyDeploymentTerminal
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

func GetTopologyDeploymentStatus(db *gorm.DB, deploymentID uint) (*TopologyDeploymentStatus, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if deploymentID == 0 {
		return nil, errors.New("deployment_id is required")
	}
	status := &TopologyDeploymentStatus{}
	if err := db.First(&status.Deployment, deploymentID).Error; err != nil {
		return nil, err
	}
	if err := db.Where("deployment_id = ?", deploymentID).Order("apply_order, id").Find(&status.Steps).Error; err != nil {
		return nil, err
	}
	if err := db.Where("deployment_id = ?", deploymentID).Order("node_id").Find(&status.Observed).Error; err != nil {
		return nil, err
	}
	return status, nil
}

func compileTopologyDeploymentSteps(tx *gorm.DB, topology model.Topology, revision model.TopologyRevision, vertices []model.TopologyVertex, edges []model.TopologyEdge, rolloutGroup string) ([]model.TopologyDeploymentStep, error) {
	issues := ValidateTopology(TopologyRevisionInput{Vertices: vertices, Edges: edges})
	if len(issues) > 0 {
		return nil, fmt.Errorf("topology validation failed: %s", issues[0].Message)
	}
	order, err := topologyVertexApplyOrder(vertices, edges)
	if err != nil {
		return nil, err
	}
	verticesByKey := make(map[string]model.TopologyVertex, len(vertices))
	for _, vertex := range vertices {
		verticesByKey[vertex.Key] = vertex
	}

	assignments := make(map[string]model.NodeServiceAssignment)
	loadAssignment := func(vertex model.TopologyVertex) (model.NodeServiceAssignment, error) {
		if vertex.NodeID == nil || strings.TrimSpace(vertex.PluginID) == "" {
			return model.NodeServiceAssignment{}, errors.New("topology vertex is not deployable")
		}
		key := topologyDeploymentPluginKey(*vertex.NodeID, vertex.PluginID, vertex.Role)
		if assignment, ok := assignments[key]; ok {
			return assignment, nil
		}
		var assignment model.NodeServiceAssignment
		err := tx.Where("node_id = ? AND service_scope = ? AND plugin_id = ? AND role = ? AND enabled = ?",
			*vertex.NodeID, topology.ServiceScope, vertex.PluginID, vertex.Role, true).First(&assignment).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.NodeServiceAssignment{}, fmt.Errorf("topology vertex %q has no enabled node assignment", vertex.Key)
		}
		if err != nil {
			return model.NodeServiceAssignment{}, err
		}
		if strings.TrimSpace(assignment.DesiredVersion) == "" {
			return model.NodeServiceAssignment{}, fmt.Errorf("topology vertex %q assignment has no desired plugin version", vertex.Key)
		}
		var release model.PluginRelease
		if err := tx.First(&release, "plugin_id = ? AND version = ?", assignment.PluginID, assignment.DesiredVersion).Error; err != nil {
			return model.NodeServiceAssignment{}, fmt.Errorf("topology vertex %q desired plugin release is not registered", vertex.Key)
		}
		if err := ValidatePluginReleaseTarget(release, "agent"); err != nil {
			return model.NodeServiceAssignment{}, fmt.Errorf("topology vertex %q plugin release is not Agent-compatible: %w", vertex.Key, err)
		}
		assignments[key] = assignment
		return assignment, nil
	}

	selected := make(map[string]bool, len(vertices))
	seenNodePlugin := make(map[string]string)
	for _, vertex := range vertices {
		if vertex.NodeID == nil || strings.TrimSpace(vertex.PluginID) == "" {
			continue
		}
		assignment, err := loadAssignment(vertex)
		if err != nil {
			return nil, err
		}
		if rolloutGroup != "" && assignment.RolloutGroup != rolloutGroup {
			continue
		}
		nodePlugin := fmt.Sprintf("%d\x00%s", *vertex.NodeID, vertex.PluginID)
		if existing, duplicate := seenNodePlugin[nodePlugin]; duplicate {
			return nil, fmt.Errorf("topology vertices %q and %q share Agent plugin %q on node %d; plugin instances must be unique", existing, vertex.Key, vertex.PluginID, *vertex.NodeID)
		}
		seenNodePlugin[nodePlugin] = vertex.Key
		selected[vertex.Key] = true
	}
	if len(selected) == 0 {
		return nil, errors.New("topology revision has no deployable plugin vertices for rollout group")
	}

	previous := make(map[string]model.TopologyVertex)
	if topology.ActiveRevisionID != nil {
		var previousVertices []model.TopologyVertex
		if err := tx.Where("revision_id = ?", *topology.ActiveRevisionID).Order("key").Find(&previousVertices).Error; err != nil {
			return nil, err
		}
		for _, vertex := range previousVertices {
			if vertex.NodeID == nil || strings.TrimSpace(vertex.PluginID) == "" {
				continue
			}
			assignment, err := loadAssignment(vertex)
			if err != nil {
				return nil, err
			}
			if rolloutGroup != "" && assignment.RolloutGroup != rolloutGroup {
				continue
			}
			key := topologyDeploymentPluginKey(*vertex.NodeID, vertex.PluginID, vertex.Role)
			if _, duplicate := previous[key]; duplicate {
				return nil, fmt.Errorf("previous active topology has duplicate plugin assignment %q", key)
			}
			previous[key] = vertex
		}
	}

	steps := make([]model.TopologyDeploymentStep, 0, len(selected)+len(previous))
	selectedPluginKeys := make(map[string]struct{}, len(selected))
	for index, key := range order {
		if !selected[key] {
			continue
		}
		vertex := verticesByKey[key]
		assignment, err := loadAssignment(vertex)
		if err != nil {
			return nil, err
		}
		config, err := CanonicalKernelOperationConfig(vertex.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("canonicalize topology vertex %q config: %w", vertex.Key, err)
		}
		pluginKey := topologyDeploymentPluginKey(*vertex.NodeID, vertex.PluginID, vertex.Role)
		selectedPluginKeys[pluginKey] = struct{}{}
		step := model.TopologyDeploymentStep{
			VertexID: vertex.ID, VertexKey: vertex.Key, NodeID: *vertex.NodeID,
			PluginID: vertex.PluginID, Role: vertex.Role, TargetVersion: assignment.DesiredVersion,
			ApplyOrder: index + 1, ApplyAction: topologyApplyActionConfigureEnable,
			ConfigJSON: config, RollbackMode: topologyRollbackModeDisable, RollbackConfigJSON: `{}`,
			State: topologyStepStatePlanned,
		}
		if old, found := previous[pluginKey]; found {
			rollbackConfig, err := CanonicalKernelOperationConfig(old.ConfigJSON)
			if err != nil {
				return nil, fmt.Errorf("canonicalize previous topology vertex %q config: %w", old.Key, err)
			}
			step.RollbackMode, step.RollbackConfigJSON = topologyRollbackModeRestore, rollbackConfig
		}
		steps = append(steps, step)
	}

	removedKeys := make([]string, 0, len(previous))
	for key := range previous {
		if _, retained := selectedPluginKeys[key]; !retained {
			removedKeys = append(removedKeys, key)
		}
	}
	sort.Strings(removedKeys)
	baseOrder := len(order) + 1
	for index, key := range removedKeys {
		vertex := previous[key]
		assignment, err := loadAssignment(vertex)
		if err != nil {
			return nil, err
		}
		rollbackConfig, err := CanonicalKernelOperationConfig(vertex.ConfigJSON)
		if err != nil {
			return nil, fmt.Errorf("canonicalize previous topology vertex %q config: %w", vertex.Key, err)
		}
		steps = append(steps, model.TopologyDeploymentStep{
			VertexID: vertex.ID, VertexKey: vertex.Key, NodeID: *vertex.NodeID,
			PluginID: vertex.PluginID, Role: vertex.Role, TargetVersion: assignment.DesiredVersion,
			ApplyOrder: baseOrder + index, Removal: true, ApplyAction: topologyApplyActionDisable,
			ConfigJSON: `{}`, RollbackMode: topologyRollbackModeRestore, RollbackConfigJSON: rollbackConfig,
			State: topologyStepStatePlanned,
		})
	}
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].ApplyOrder != steps[j].ApplyOrder {
			return steps[i].ApplyOrder < steps[j].ApplyOrder
		}
		return steps[i].VertexKey < steps[j].VertexKey
	})
	return steps, nil
}

func topologyVertexApplyOrder(vertices []model.TopologyVertex, edges []model.TopologyEdge) ([]string, error) {
	byKey := make(map[string]struct{}, len(vertices))
	indegree := make(map[string]int, len(vertices))
	outgoing := make(map[string][]string, len(vertices))
	for _, vertex := range vertices {
		if _, duplicate := byKey[vertex.Key]; duplicate {
			return nil, fmt.Errorf("topology has duplicate vertex %q", vertex.Key)
		}
		byKey[vertex.Key] = struct{}{}
		indegree[vertex.Key] = 0
	}
	for _, edge := range edges {
		if _, source := byKey[edge.SourceKey]; !source {
			return nil, fmt.Errorf("topology edge source %q does not exist", edge.SourceKey)
		}
		if _, target := byKey[edge.TargetKey]; !target {
			return nil, fmt.Errorf("topology edge target %q does not exist", edge.TargetKey)
		}
		outgoing[edge.SourceKey] = append(outgoing[edge.SourceKey], edge.TargetKey)
		indegree[edge.TargetKey]++
	}
	ready := make([]string, 0, len(vertices))
	for key, degree := range indegree {
		if degree == 0 {
			ready = append(ready, key)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(vertices))
	for len(ready) > 0 {
		key := ready[0]
		ready = ready[1:]
		order = append(order, key)
		targets := append([]string(nil), outgoing[key]...)
		sort.Strings(targets)
		for _, target := range targets {
			indegree[target]--
			if indegree[target] == 0 {
				ready = append(ready, target)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(vertices) {
		return nil, errors.New("topology must be a directed acyclic graph")
	}
	return order, nil
}

func (e *TopologyDeploymentExecutor) reconcileDeployment(ctx context.Context, deploymentID uint) (bool, error) {
	changed := false
	err := e.db.Transaction(func(tx *gorm.DB) error {
		var deployment model.TopologyDeployment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deployment, deploymentID).Error; err != nil {
			return err
		}
		if deployment.State != topologyDeploymentStateApplying && deployment.State != topologyDeploymentStateRollbackRequested {
			return nil
		}
		var topology model.Topology
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&topology, deployment.TopologyID).Error; err != nil {
			return err
		}
		var revision model.TopologyRevision
		if err := tx.First(&revision, deployment.RevisionID).Error; err != nil {
			return err
		}
		var steps []model.TopologyDeploymentStep
		if err := tx.Where("deployment_id = ?", deployment.ID).Order("apply_order, id").Find(&steps).Error; err != nil {
			return err
		}
		if len(steps) == 0 {
			return errors.New("topology deployment has no steps")
		}
		stepChanged, applyFailed, err := refreshTopologyStepStates(tx, &steps)
		if err != nil {
			return err
		}
		changed = changed || stepChanged

		if deployment.State == topologyDeploymentStateApplying && applyFailed {
			if err := beginTopologyRollback(tx, &deployment, steps, e.now(), "an apply operation failed"); err != nil {
				return err
			}
			changed = true
		}

		if deployment.State == topologyDeploymentStateApplying {
			queued, err := e.queueReadyApplySteps(tx, deployment, revision, steps)
			if err != nil {
				return err
			}
			changed = changed || queued
			if topologyAllStepsState(steps, topologyStepStateSucceeded) {
				if err := completeTopologyDeploymentApply(tx, &topology, &deployment, revision, steps, e.now()); err != nil {
					return err
				}
				changed = true
			}
			return nil
		}

		queued, terminal, err := e.reconcileRollback(tx, &topology, &deployment, revision, steps)
		if err != nil {
			return err
		}
		changed = changed || queued || terminal
		return nil
	})
	return changed, err
}

func refreshTopologyStepStates(tx *gorm.DB, steps *[]model.TopologyDeploymentStep) (bool, bool, error) {
	changed, applyFailed := false, false
	for index := range *steps {
		step := &(*steps)[index]
		nextState, failure, err := topologyStepNextState(tx, *step)
		if err != nil {
			return changed, applyFailed, err
		}
		if failure {
			applyFailed = true
		}
		if nextState == "" || nextState == step.State {
			continue
		}
		updates := map[string]any{"state": nextState}
		if topologyStepFailureState(nextState) {
			updates["last_error"] = topologyStepOperationError(tx, *step)
		}
		if err := tx.Model(step).Updates(updates).Error; err != nil {
			return changed, applyFailed, err
		}
		step.State = nextState
		if value, ok := updates["last_error"].(string); ok {
			step.LastError = value
		}
		changed = true
	}
	return changed, applyFailed, nil
}

func topologyStepNextState(tx *gorm.DB, step model.TopologyDeploymentStep) (string, bool, error) {
	switch step.State {
	case topologyStepStateConfiguring:
		return topologyNextStateForOperation(tx, step.ConfigureOperationID, topologyStepStateEnabling, topologyStepStateFailed)
	case topologyStepStateEnabling:
		return topologyNextStateForOperation(tx, step.EnableOperationID, topologyStepStateSucceeded, topologyStepStateFailed)
	case topologyStepStateDisabling:
		return topologyNextStateForOperation(tx, step.DisableOperationID, topologyStepStateSucceeded, topologyStepStateFailed)
	case topologyStepStateRollbackConfiguring:
		next, _, err := topologyNextStateForOperation(tx, step.RollbackConfigureOperationID, topologyStepStateRollbackEnabling, topologyStepStateRollbackFailed)
		return next, false, err
	case topologyStepStateRollbackEnabling:
		next, _, err := topologyNextStateForOperation(tx, step.RollbackEnableOperationID, topologyStepStateRolledBack, topologyStepStateRollbackFailed)
		return next, false, err
	case topologyStepStateRollbackDisabling:
		next, _, err := topologyNextStateForOperation(tx, step.RollbackDisableOperationID, topologyStepStateRolledBack, topologyStepStateRollbackFailed)
		return next, false, err
	default:
		return "", false, nil
	}
}

func topologyNextStateForOperation(tx *gorm.DB, operationID, successState, failureState string) (string, bool, error) {
	if operationID == "" {
		return failureState, true, nil
	}
	var operation model.KernelOperation
	if err := tx.First(&operation, "id = ?", operationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failureState, true, nil
		}
		return "", false, err
	}
	switch operation.State {
	case "succeeded":
		return successState, false, nil
	case "failed", "cancelled", "superseded", "timed_out":
		return failureState, true, nil
	default:
		return "", false, nil
	}
}

func topologyStepOperationError(tx *gorm.DB, step model.TopologyDeploymentStep) string {
	operationID := topologyCurrentStepOperationID(step)
	if operationID == "" {
		return "topology operation was not created"
	}
	var operation model.KernelOperation
	if err := tx.First(&operation, "id = ?", operationID).Error; err != nil || strings.TrimSpace(operation.LastError) == "" {
		return "topology operation failed"
	}
	return operation.LastError
}

func topologyCurrentStepOperationID(step model.TopologyDeploymentStep) string {
	switch step.State {
	case topologyStepStateConfiguring:
		return step.ConfigureOperationID
	case topologyStepStateEnabling:
		return step.EnableOperationID
	case topologyStepStateDisabling:
		return step.DisableOperationID
	case topologyStepStateRollbackConfiguring:
		return step.RollbackConfigureOperationID
	case topologyStepStateRollbackEnabling:
		return step.RollbackEnableOperationID
	case topologyStepStateRollbackDisabling:
		return step.RollbackDisableOperationID
	default:
		return ""
	}
}

func topologyStepFailureState(state string) bool {
	return state == topologyStepStateFailed || state == topologyStepStateRollbackFailed
}

func (e *TopologyDeploymentExecutor) queueReadyApplySteps(tx *gorm.DB, deployment model.TopologyDeployment, revision model.TopologyRevision, steps []model.TopologyDeploymentStep) (bool, error) {
	dependencies, err := topologyStepDependencies(tx, deployment.RevisionID, steps)
	if err != nil {
		return false, err
	}
	queued := false
	for index := range steps {
		step := steps[index]
		if step.State == topologyStepStateEnabling && step.EnableOperationID == "" {
			if err := e.queueStepEnable(tx, deployment, revision, &step, false); err != nil {
				return queued, err
			}
			queued = true
			continue
		}
		if step.State != topologyStepStatePlanned || !topologyStepDependenciesSucceeded(step.ID, dependencies, steps) {
			continue
		}
		if step.Removal && !topologyAllNonRemovalStepsSucceeded(steps) {
			continue
		}
		if err := e.queueApplyStep(tx, deployment, revision, &step); err != nil {
			return queued, err
		}
		queued = true
	}
	return queued, nil
}

func topologyStepDependencies(tx *gorm.DB, revisionID uint, steps []model.TopologyDeploymentStep) (map[uint][]uint, error) {
	byVertexKey := make(map[string]uint, len(steps))
	for _, step := range steps {
		if !step.Removal {
			byVertexKey[step.VertexKey] = step.ID
		}
	}
	var edges []model.TopologyEdge
	if err := tx.Where("revision_id = ?", revisionID).Find(&edges).Error; err != nil {
		return nil, err
	}
	dependencies := make(map[uint][]uint, len(steps))
	for _, edge := range edges {
		source, sourceSelected := byVertexKey[edge.SourceKey]
		target, targetSelected := byVertexKey[edge.TargetKey]
		if sourceSelected && targetSelected {
			dependencies[target] = append(dependencies[target], source)
		}
	}
	return dependencies, nil
}

func topologyStepDependenciesSucceeded(stepID uint, dependencies map[uint][]uint, steps []model.TopologyDeploymentStep) bool {
	byID := make(map[uint]model.TopologyDeploymentStep, len(steps))
	for _, step := range steps {
		byID[step.ID] = step
	}
	for _, dependencyID := range dependencies[stepID] {
		if byID[dependencyID].State != topologyStepStateSucceeded {
			return false
		}
	}
	return true
}

func topologyAllNonRemovalStepsSucceeded(steps []model.TopologyDeploymentStep) bool {
	for _, step := range steps {
		if !step.Removal && step.State != topologyStepStateSucceeded {
			return false
		}
	}
	return true
}

func (e *TopologyDeploymentExecutor) queueApplyStep(tx *gorm.DB, deployment model.TopologyDeployment, revision model.TopologyRevision, step *model.TopologyDeploymentStep) error {
	var kind, config, field, state, action string
	switch step.ApplyAction {
	case topologyApplyActionConfigureEnable:
		kind, config, field, state, action = "plugin.configure", step.ConfigJSON, "configure_operation_id", topologyStepStateConfiguring, "configure"
	case topologyApplyActionDisable:
		kind, config, field, state, action = "plugin.disable", `{}`, "disable_operation_id", topologyStepStateDisabling, "disable"
	default:
		return fmt.Errorf("unsupported topology apply action %q", step.ApplyAction)
	}
	op, err := e.ensureTopologyStepOperation(tx, deployment, revision, *step, kind, config, action)
	if err != nil {
		return err
	}
	if err := tx.Model(step).Updates(map[string]any{field: op.ID, "state": state, "last_error": ""}).Error; err != nil {
		return err
	}
	step.State = state
	return nil
}

func (e *TopologyDeploymentExecutor) queueStepEnable(tx *gorm.DB, deployment model.TopologyDeployment, revision model.TopologyRevision, step *model.TopologyDeploymentStep, rollback bool) error {
	config := step.ConfigJSON
	field := "enable_operation_id"
	action := "enable"
	if rollback {
		config, field, action = step.RollbackConfigJSON, "rollback_enable_operation_id", "rollback-enable"
	}
	op, err := e.ensureTopologyStepOperation(tx, deployment, revision, *step, "plugin.enable", config, action)
	if err != nil {
		return err
	}
	if err := tx.Model(step).Update(field, op.ID).Error; err != nil {
		return err
	}
	if rollback {
		step.RollbackEnableOperationID = op.ID
	} else {
		step.EnableOperationID = op.ID
	}
	return nil
}

func (e *TopologyDeploymentExecutor) reconcileRollback(tx *gorm.DB, topology *model.Topology, deployment *model.TopologyDeployment, revision model.TopologyRevision, steps []model.TopologyDeploymentStep) (bool, bool, error) {
	changed := cancelTopologyApplyOperations(tx, steps, e.now())
	if topologyHasActiveApplyStep(steps) {
		return changed, false, nil
	}
	for index := range steps {
		if steps[index].State != topologyStepStateRollbackEnabling || steps[index].RollbackEnableOperationID != "" {
			continue
		}
		if err := e.queueStepEnable(tx, *deployment, revision, &steps[index], true); err != nil {
			return changed, false, err
		}
		return true, false, nil
	}
	if step := topologyNextRollbackStep(steps); step != nil {
		if err := e.queueRollbackStep(tx, *deployment, revision, step); err != nil {
			return changed, false, err
		}
		return true, false, nil
	}
	if topologyHasActiveRollbackStep(steps) {
		return changed, false, nil
	}
	now := e.now()
	if topologyAnyStepState(steps, topologyStepStateRollbackFailed) {
		nodes, err := topologyTerminalObservedNodes(tx, deployment.RevisionID)
		if err != nil {
			return changed, false, err
		}
		if err := writeTopologyTerminalObservedStates(tx, *deployment, revision, nodes, "failed", deployment.LastError, now); err != nil {
			return changed, false, err
		}
		if err := tx.Model(deployment).Updates(map[string]any{
			"state": topologyDeploymentStateFailed, "completed_at": now, "rollback_completed_at": now,
		}).Error; err != nil {
			return changed, false, err
		}
		return true, true, nil
	}
	if err := completeTopologyDeploymentRollback(tx, topology, deployment, revision, steps, now); err != nil {
		return changed, false, err
	}
	return true, true, nil
}

func beginTopologyRollback(tx *gorm.DB, deployment *model.TopologyDeployment, steps []model.TopologyDeploymentStep, at time.Time, reason string) error {
	if deployment.RollbackStartedAt == nil {
		deployment.RollbackStartedAt = &at
	}
	lastError := strings.TrimSpace(deployment.LastError)
	if lastError == "" {
		for _, step := range steps {
			if step.State == topologyStepStateFailed && strings.TrimSpace(step.LastError) != "" {
				lastError = step.LastError
				break
			}
		}
	}
	if lastError == "" {
		lastError = reason
	}
	if err := tx.Model(deployment).Updates(map[string]any{
		"state": topologyDeploymentStateRollbackRequested, "last_error": lastError,
		"completed_at": nil, "rollback_started_at": deployment.RollbackStartedAt,
	}).Error; err != nil {
		return err
	}
	deployment.State, deployment.LastError, deployment.CompletedAt = topologyDeploymentStateRollbackRequested, lastError, nil
	for index := range steps {
		if steps[index].State != topologyStepStatePlanned {
			continue
		}
		if err := tx.Model(&steps[index]).Update("state", topologyStepStateSkipped).Error; err != nil {
			return err
		}
		steps[index].State = topologyStepStateSkipped
	}
	return nil
}

func cancelTopologyApplyOperations(tx *gorm.DB, steps []model.TopologyDeploymentStep, at time.Time) bool {
	changed := false
	for _, step := range steps {
		if !topologyApplyStepInFlight(step.State) {
			continue
		}
		operationID := topologyCurrentStepOperationID(step)
		if operationID == "" {
			continue
		}
		result := tx.Model(&model.KernelOperation{}).Where("id = ? AND state = ?", operationID, "pending").Updates(map[string]any{
			"state": "cancelled", "cancel_at": at, "last_error": "topology rollback requested before dispatch",
		})
		if result.Error == nil && result.RowsAffected > 0 {
			changed = true
			continue
		}
		result = tx.Model(&model.KernelOperation{}).Where("id = ? AND state IN ?", operationID, []string{"dispatching", "running"}).Updates(map[string]any{
			"state": "cancel_requested", "cancel_at": at, "last_error": "topology rollback requested",
		})
		if result.Error == nil && result.RowsAffected > 0 {
			changed = true
		}
	}
	return changed
}

func topologyApplyStepInFlight(state string) bool {
	return state == topologyStepStateConfiguring || state == topologyStepStateEnabling || state == topologyStepStateDisabling
}

func topologyHasActiveApplyStep(steps []model.TopologyDeploymentStep) bool {
	for _, step := range steps {
		if topologyApplyStepInFlight(step.State) {
			return true
		}
	}
	return false
}

func topologyNextRollbackStep(steps []model.TopologyDeploymentStep) *model.TopologyDeploymentStep {
	ordered := append([]model.TopologyDeploymentStep(nil), steps...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].ApplyOrder != ordered[j].ApplyOrder {
			return ordered[i].ApplyOrder > ordered[j].ApplyOrder
		}
		return ordered[i].ID > ordered[j].ID
	})
	for index := range ordered {
		state := ordered[index].State
		if state == topologyStepStateSucceeded || state == topologyStepStateFailed {
			step := ordered[index]
			return &step
		}
	}
	return nil
}

func topologyHasActiveRollbackStep(steps []model.TopologyDeploymentStep) bool {
	for _, step := range steps {
		switch step.State {
		case topologyStepStateRollbackConfiguring, topologyStepStateRollbackEnabling, topologyStepStateRollbackDisabling:
			return true
		}
	}
	return false
}

func topologyAnyStepState(steps []model.TopologyDeploymentStep, target string) bool {
	for _, step := range steps {
		if step.State == target {
			return true
		}
	}
	return false
}

func (e *TopologyDeploymentExecutor) queueRollbackStep(tx *gorm.DB, deployment model.TopologyDeployment, revision model.TopologyRevision, step *model.TopologyDeploymentStep) error {
	var kind, config, field, state, action string
	switch step.RollbackMode {
	case topologyRollbackModeRestore:
		kind, config, field, state, action = "plugin.configure", step.RollbackConfigJSON, "rollback_configure_operation_id", topologyStepStateRollbackConfiguring, "rollback-configure"
	case topologyRollbackModeDisable:
		kind, config, field, state, action = "plugin.disable", `{}`, "rollback_disable_operation_id", topologyStepStateRollbackDisabling, "rollback-disable"
	default:
		return fmt.Errorf("unsupported topology rollback mode %q", step.RollbackMode)
	}
	op, err := e.ensureTopologyStepOperation(tx, deployment, revision, *step, kind, config, action)
	if err != nil {
		return err
	}
	if err := tx.Model(step).Updates(map[string]any{field: op.ID, "state": state}).Error; err != nil {
		return err
	}
	step.State = state
	return nil
}

func (e *TopologyDeploymentExecutor) ensureTopologyStepOperation(tx *gorm.DB, deployment model.TopologyDeployment, revision model.TopologyRevision, step model.TopologyDeploymentStep, kind, configJSON, action string) (*model.KernelOperation, error) {
	idempotencyKey := fmt.Sprintf("topology:%d:step:%d:%s", deployment.ID, step.ID, action)
	var existing model.KernelOperation
	if err := tx.First(&existing, "idempotency_key = ?", idempotencyKey).Error; err == nil {
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	deadline := e.now().Add(e.operationTimeout)
	deploymentID, stepID := deployment.ID, step.ID
	op, _, err := CreateKernelOperation(tx, model.KernelOperation{
		ID: uuid.NewString(), IdempotencyKey: idempotencyKey, NodeID: &step.NodeID,
		PluginID: step.PluginID, TargetVersion: step.TargetVersion, Kind: kind,
		ConfigJSON: configJSON, DeadlineAt: &deadline,
		TopologyDeploymentID: &deploymentID, TopologyStepID: &stepID, TopologyRevision: revision.Revision,
	})
	return op, err
}

func completeTopologyDeploymentApply(tx *gorm.DB, topology *model.Topology, deployment *model.TopologyDeployment, revision model.TopologyRevision, steps []model.TopologyDeploymentStep, at time.Time) error {
	nodes, err := topologyTerminalObservedNodes(tx, deployment.RevisionID)
	if err != nil {
		return err
	}
	if err := writeTopologyTerminalObservedStates(tx, *deployment, revision, nodes, "succeeded", "", at); err != nil {
		return err
	}
	if err := tx.Model(topology).Update("active_revision_id", revision.ID).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.TopologyRevision{}).Where("id = ?", revision.ID).Update("state", "active").Error; err != nil {
		return err
	}
	if err := tx.Model(deployment).Updates(map[string]any{
		"state": topologyDeploymentStateSucceeded, "completed_at": at, "last_error": "",
	}).Error; err != nil {
		return err
	}
	deployment.State, deployment.CompletedAt, deployment.LastError = topologyDeploymentStateSucceeded, &at, ""
	return nil
}

func completeTopologyDeploymentRollback(tx *gorm.DB, topology *model.Topology, deployment *model.TopologyDeployment, revision model.TopologyRevision, _ []model.TopologyDeploymentStep, at time.Time) error {
	nodes, err := topologyTerminalObservedNodes(tx, deployment.RevisionID)
	if err != nil {
		return err
	}
	if err := writeTopologyTerminalObservedStates(tx, *deployment, revision, nodes, "rolled_back", deployment.LastError, at); err != nil {
		return err
	}
	if err := tx.Model(topology).Update("active_revision_id", deployment.PreviousRevisionID).Error; err != nil {
		return err
	}
	if err := tx.Model(deployment).Updates(map[string]any{
		"state": topologyDeploymentStateRolledBack, "completed_at": at, "rollback_completed_at": at,
	}).Error; err != nil {
		return err
	}
	deployment.State, deployment.CompletedAt, deployment.RollbackCompletedAt = topologyDeploymentStateRolledBack, &at, &at
	return nil
}

func topologyTerminalObservedNodes(tx *gorm.DB, revisionID uint) ([]uint, error) {
	var nodes []uint
	if err := tx.Model(&model.TopologyVertex{}).Where("revision_id = ? AND node_id IS NOT NULL", revisionID).
		Distinct("node_id").Order("node_id").Pluck("node_id", &nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func writeTopologyTerminalObservedStates(tx *gorm.DB, deployment model.TopologyDeployment, revision model.TopologyRevision, nodes []uint, state, lastError string, at time.Time) error {
	for _, nodeID := range nodes {
		if _, _, err := ApplyTopologyObservedStateTx(tx, TopologyObservedStateUpdate{
			DeploymentID: deployment.ID, NodeID: nodeID, DesiredRevision: revision.Revision,
			ObservedRevision: revision.Revision, State: state, HealthJSON: `{}`,
			LastError: lastError, ObservedAt: at,
		}); err != nil {
			return err
		}
	}
	return nil
}

func topologyAllStepsState(steps []model.TopologyDeploymentStep, state string) bool {
	if len(steps) == 0 {
		return false
	}
	for _, step := range steps {
		if step.State != state {
			return false
		}
	}
	return true
}

func topologyDeploymentPluginKey(nodeID uint, pluginID, role string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", nodeID, pluginID, role)
}
