package plugincontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type orderedLifecycleExecutor struct {
	id        string
	version   string
	failKind  string
	failErr   error
	mu        *sync.Mutex
	callOrder *[]string
}

func (e *orderedLifecycleExecutor) PluginID() string { return e.id }
func (e *orderedLifecycleExecutor) Version() string  { return e.version }
func (e *orderedLifecycleExecutor) HandleRoute(context.Context, RouteRequest) (RouteResponse, error) {
	return RouteResponse{Status: 200}, nil
}
func (e *orderedLifecycleExecutor) ExecuteLifecycle(_ context.Context, request LifecycleRequest) (json.RawMessage, error) {
	e.mu.Lock()
	*e.callOrder = append(*e.callOrder, e.id+":"+request.Kind)
	e.mu.Unlock()
	if request.Kind == e.failKind {
		return nil, e.failErr
	}
	return json.RawMessage(`{"ok":true}`), nil
}

type cancellablePlanExecutor struct {
	id        string
	version   string
	started   chan struct{}
	once      sync.Once
	callOrder *[]string
	mu        *sync.Mutex
}

func (e *cancellablePlanExecutor) PluginID() string { return e.id }
func (e *cancellablePlanExecutor) Version() string  { return e.version }
func (e *cancellablePlanExecutor) HandleRoute(context.Context, RouteRequest) (RouteResponse, error) {
	return RouteResponse{Status: 200}, nil
}
func (e *cancellablePlanExecutor) ExecuteLifecycle(ctx context.Context, request LifecycleRequest) (json.RawMessage, error) {
	e.mu.Lock()
	*e.callOrder = append(*e.callOrder, e.id+":"+request.Kind)
	e.mu.Unlock()
	if request.Kind == "plugin.install" {
		e.once.Do(func() { close(e.started) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return json.RawMessage(`{"ok":true}`), nil
		}
	}
	return json.RawMessage(`{"ok":true}`), nil
}

func seedDependencyLifecycleRelease(t *testing.T, db *gorm.DB, pluginID, version string, dependencies ...string) model.PluginRelease {
	t.Helper()
	artifact := []byte(pluginID + "-" + version + "-artifact")
	digest := sha256.Sum256(artifact)
	manifest := service.PluginManifest{
		ID: pluginID, Name: pluginID, Version: version, APIVersion: "v1", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: hex.EncodeToString(digest[:]), Dependencies: dependencies,
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	plugin := model.Plugin{ID: pluginID, Name: pluginID, Publisher: "AnixOps", Official: true}
	require.NoError(t, db.Where("id = ?", pluginID).FirstOrCreate(&plugin).Error)
	release := model.PluginRelease{
		PluginID: pluginID, Version: version, APIVersion: "v1", ManifestJSON: string(canonical),
		ArtifactSHA256: manifest.ArtifactSHA256, Signature: "test", PublishedAt: time.Now(),
	}
	require.NoError(t, db.Create(&release).Error)
	_, err = service.StorePluginArtifact(db, release.ID, artifact)
	require.NoError(t, err)
	return release
}

func queueDependencyLifecycleRoot(t *testing.T, db *gorm.DB, release model.PluginRelease, key string) (*model.KernelOperation, model.PluginInstallation) {
	t.Helper()
	root := model.PluginInstallation{
		PluginID: release.PluginID, Target: "control", DesiredVersion: release.Version,
		State: "pending", Enabled: true, LifecycleGeneration: 1,
	}
	require.NoError(t, db.Create(&root).Error)
	operation, err := QueueDependencyLifecyclePlan(db, DependencyLifecyclePlanRequest{
		RootInstallation: root, RootRelease: release, RootKind: "plugin.enable", IdempotencyKey: key,
		DeadlineAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	return operation, root
}

func TestDependencyLifecyclePlanExecutesDependencyClosureBeforeRootAndIsIdempotent(t *testing.T) {
	db := newOperationWorkerDB(t)
	leaf := seedDependencyLifecycleRelease(t, db, "plan-leaf", "1.0.0")
	middle := seedDependencyLifecycleRelease(t, db, "plan-middle", "1.0.0", leaf.PluginID)
	rootRelease := seedDependencyLifecycleRelease(t, db, "plan-root", "1.0.0", middle.PluginID)
	rootOperation, root := queueDependencyLifecycleRoot(t, db, rootRelease, "dependency-plan-order")

	// Repeating a request after a durable plan was created must return the
	// exact public root operation instead of duplicating its closure.
	replayed, err := QueueDependencyLifecyclePlan(db, DependencyLifecyclePlanRequest{
		RootInstallation: root, RootRelease: rootRelease, RootKind: "plugin.enable", IdempotencyKey: "dependency-plan-order",
		DeadlineAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, rootOperation.ID, replayed.ID)

	var plan model.PluginLifecyclePlan
	require.NoError(t, db.Where("idempotency_key = ?", "dependency-plan-order").First(&plan).Error)
	var steps []model.PluginLifecyclePlanStep
	require.NoError(t, db.Where("plan_id = ?", plan.ID).Order("sequence").Find(&steps).Error)
	require.Equal(t, []string{"plan-leaf", "plan-middle", "plan-root"}, []string{steps[0].PluginID, steps[1].PluginID, steps[2].PluginID})
	require.Equal(t, rootOperation.ID, plan.RootOperationID)

	var order []string
	var mu sync.Mutex
	registry, err := NewRegistry(
		&orderedLifecycleExecutor{id: leaf.PluginID, version: leaf.Version, mu: &mu, callOrder: &order},
		&orderedLifecycleExecutor{id: middle.PluginID, version: middle.Version, mu: &mu, callOrder: &order},
		&orderedLifecycleExecutor{id: rootRelease.PluginID, version: rootRelease.Version, mu: &mu, callOrder: &order},
	)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	processed, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 6, processed)
	require.Equal(t, []string{
		"plan-leaf:plugin.install", "plan-leaf:plugin.enable",
		"plan-middle:plugin.install", "plan-middle:plugin.enable",
		"plan-root:plugin.install", "plan-root:plugin.enable",
	}, order)

	require.NoError(t, db.First(&plan, "id = ?", plan.ID).Error)
	require.Equal(t, lifecyclePlanSucceeded, plan.State)
	for _, pluginID := range []string{leaf.PluginID, middle.PluginID, rootRelease.PluginID} {
		var installation model.PluginInstallation
		require.NoError(t, db.Where("plugin_id = ? AND target = ?", pluginID, "control").First(&installation).Error)
		require.True(t, installation.Enabled)
		require.Equal(t, "healthy", installation.State)
	}
}

func TestDependencyLifecyclePlanFailureRollsBackAppliedStepsInReverseOrder(t *testing.T) {
	db := newOperationWorkerDB(t)
	leaf := seedDependencyLifecycleRelease(t, db, "rollback-leaf", "1.0.0")
	rootRelease := seedDependencyLifecycleRelease(t, db, "rollback-root", "1.0.0", leaf.PluginID)
	_, _ = queueDependencyLifecycleRoot(t, db, rootRelease, "dependency-plan-rollback")

	var order []string
	var mu sync.Mutex
	registry, err := NewRegistry(
		&orderedLifecycleExecutor{id: leaf.PluginID, version: leaf.Version, mu: &mu, callOrder: &order},
		&orderedLifecycleExecutor{id: rootRelease.PluginID, version: rootRelease.Version, failKind: "plugin.install", failErr: errors.New("root install failed"), mu: &mu, callOrder: &order},
	)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	processed, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, processed)
	processed, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, processed)
	require.Equal(t, []string{
		"rollback-leaf:plugin.install", "rollback-leaf:plugin.enable", "rollback-root:plugin.install",
		"rollback-root:plugin.disable", "rollback-leaf:plugin.disable",
	}, order)

	var plan model.PluginLifecyclePlan
	require.NoError(t, db.Where("idempotency_key = ?", "dependency-plan-rollback").First(&plan).Error)
	require.Equal(t, lifecyclePlanFailed, plan.State)
	for _, pluginID := range []string{leaf.PluginID, rootRelease.PluginID} {
		var installation model.PluginInstallation
		require.NoError(t, db.Where("plugin_id = ? AND target = ?", pluginID, "control").First(&installation).Error)
		require.False(t, installation.Enabled)
		require.Equal(t, "disabled", installation.State)
	}
}

func TestDependencyLifecyclePlanCancellationAndExpiredLeaseRecoverDurably(t *testing.T) {
	db := newOperationWorkerDB(t)
	leaf := seedDependencyLifecycleRelease(t, db, "cancel-leaf", "1.0.0")
	rootRelease := seedDependencyLifecycleRelease(t, db, "cancel-root", "1.0.0", leaf.PluginID)
	rootOperation, _ := queueDependencyLifecycleRoot(t, db, rootRelease, "dependency-plan-cancel")

	var order []string
	var mu sync.Mutex
	blocking := &cancellablePlanExecutor{id: leaf.PluginID, version: leaf.Version, started: make(chan struct{}), mu: &mu, callOrder: &order}
	rootExecutor := &orderedLifecycleExecutor{id: rootRelease.PluginID, version: rootRelease.Version, mu: &mu, callOrder: &order}
	registry, err := NewRegistry(blocking, rootExecutor)
	require.NoError(t, err)
	worker, err := NewOperationWorker(db, registry)
	require.NoError(t, err)
	worker.leaseDuration = 20 * time.Millisecond
	run := make(chan error, 1)
	go func() {
		_, runErr := worker.RunOnce(context.Background())
		run <- runErr
	}()
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("dependency operation did not start")
	}
	cancelled, err := service.CancelKernelOperation(db, rootOperation.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.State)
	require.NoError(t, <-run)
	processed, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var plan model.PluginLifecyclePlan
	require.NoError(t, db.Where("idempotency_key = ?", "dependency-plan-cancel").First(&plan).Error)
	require.Equal(t, lifecyclePlanCancelled, plan.State)
	require.Equal(t, []string{"cancel-leaf:plugin.install", "cancel-leaf:plugin.disable"}, order)

	// A second plan starts with its first operation apparently owned by a dead
	// Control process. The next worker recovers the expired lease and completes
	// the dependency closure without any in-memory coordinator.
	stableLeaf := seedDependencyLifecycleRelease(t, db, "recover-leaf", "1.0.0")
	stableRoot := seedDependencyLifecycleRelease(t, db, "recover-root", "1.0.0", stableLeaf.PluginID)
	_, _ = queueDependencyLifecycleRoot(t, db, stableRoot, "dependency-plan-recover")
	var first model.KernelOperation
	require.NoError(t, db.Where("lifecycle_plan_id <> ? AND plugin_id = ?", plan.ID, stableLeaf.PluginID).Order("lifecycle_plan_sequence").First(&first).Error)
	expired := time.Now().Add(-time.Second)
	require.NoError(t, db.Model(&first).Updates(map[string]any{"state": "running", "claimed_by": "dead-worker", "lease_expires_at": expired}).Error)
	registry, err = NewRegistry(
		&orderedLifecycleExecutor{id: stableLeaf.PluginID, version: stableLeaf.Version, mu: &mu, callOrder: &order},
		&orderedLifecycleExecutor{id: stableRoot.PluginID, version: stableRoot.Version, mu: &mu, callOrder: &order},
	)
	require.NoError(t, err)
	worker, err = NewOperationWorker(db, registry)
	require.NoError(t, err)
	processed, err = worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4, processed)
	var recovered model.PluginLifecyclePlan
	require.NoError(t, db.Where("idempotency_key = ?", "dependency-plan-recover").First(&recovered).Error)
	require.Equal(t, lifecyclePlanSucceeded, recovered.State)
}
